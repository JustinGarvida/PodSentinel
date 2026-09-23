# Go Agent

The Go agent is PodSentinel's core ingestion path: it polls the Kubernetes Metrics API, writes raw pod metrics to Postgres/TimescaleDB, publishes each pod's sample to RabbitMQ for the anomaly detector, and serves a REST API for the dashboard to read from. It's a single binary — one process handles polling, persistence, publishing, and HTTP, deliberately kept as one service rather than several (see [`../CLAUDE.md`](../CLAUDE.md#tech-stack--why)).

Publishing to RabbitMQ (`metrics.raw`) is wired up; consuming anomaly events (`anomalies.detected`) and the Python anomaly detector are not built yet. The `anomalies` table and its API endpoint exist and return an empty list until that lands — see [`../docs/architecture.md`](../docs/architecture.md) for the full system design and build order.

## Architecture

```
                ┌─────────────────────────────────────────────┐
                │                 cmd/agent                    │
                │   loads config, builds dependencies, runs    │
                │   the poll loop + HTTP server, handles       │
                │   SIGINT/SIGTERM for graceful shutdown       │
                └───────────────┬───────────────┬──────────────┘
                                │                │
                     poll loop  │                │  serves
                     (ticker)   ▼                ▼
                        ┌───────────────┐  ┌──────────────┐
                        │    ingest     │  │     api      │
                        │  Run(): poll  │  │ chi router + │
                        │  → persist,   │  │  handlers    │
                        │  one cycle    │  └──────┬───────┘
                        └───┬───────┬───┘         │
                            │       │             │
                  Poll()    │       │ InsertPodMetric
                            ▼       ▼             ▼
                     ┌──────────┐ ┌──────────────────┐
                     │   k8s    │ │       store       │
                     │ Poller:  │ │ pgx pool + queries│
                     │ lists    │ │ (ListPods,        │
                     │ pods +   │ │ GetPodMetrics,     │
                     │ metrics, │ │ ListAnomalies);    │
                     │ joins    │ │ runs embedded      │
                     │ them     │ │ SQL migrations on  │
                     │ (join.go)│ │ Open()             │
                     └────┬─────┘ └─────────┬──────────┘
                          │                 │
                          ▼                 ▼
              Kubernetes core +      Postgres / TimescaleDB
              metrics.k8s.io APIs    (pod_metrics, anomalies)
              (client-go)
```

One poll cycle (`ingest.Run`, called once at startup and then on every `POLL_INTERVAL` tick):

1. **`k8s.Poller.Poll`** lists pods (core API) and pod metrics (`metrics.k8s.io`) per watched namespace. A failed namespace is logged and skipped — it doesn't block the rest of the cycle.
2. **`k8s.joinPodsAndMetrics`** (`internal/k8s/join.go`) joins the two lists by `namespace/name` into a `PodSample` (identity, status, restart count, CPU, memory, timestamp). A pod with no matching metrics entry yet gets a zeroed sample rather than being dropped. The timestamp prefers metrics-server's own scrape time over wall-clock "now", since metrics-server scrapes on its own ~60s cadence independent of the poll interval.
3. **`ingest.Run`** converts each sample to a `store.PodMetricRow` and calls `store.InsertPodMetric`. A failed insert is logged and skipped, not fatal — one bad row doesn't stop the rest of the batch.
4. **`mq.Publisher.Publish`** (same loop, independent of step 3) sends the sample as one JSON message to the `metrics.raw` topic exchange with routing key `<namespace>.<pod>`. A failed publish is logged and skipped; the insert happens either way, so a RabbitMQ outage never loses metrics.

### RabbitMQ publishing

`internal/mq` declares this topology on connect (idempotent — the Python detector declares the same, so either can start first):

| Name | Type | Purpose |
|---|---|---|
| `metrics.raw` | topic exchange | Raw per-pod samples, routing key `<namespace>.<pod>` |
| `metrics.raw.detector` | durable queue, bound with `#` | What the detector consumes; dead-letters to `metrics.raw.dlx` |
| `metrics.raw.dlx` | fanout exchange | Receives messages the detector nacks without requeue |
| `metrics.raw.dlq` | durable queue | Holds dead-lettered messages for inspection |

Message body (`mq.MetricMessage`, `content_type: application/json`, persistent):

```json
{"schema_version": 1, "timestamp": "2026-09-23T12:00:00Z", "namespace": "default",
 "pod": "web-7d9f8c6b5d-x8k2p", "pod_uid": "…", "owner_kind": "Deployment", "owner_name": "web",
 "cpu": 0.25, "memory": 150000000, "status": "Running", "restart_count": 2}
```

The publisher connects lazily on the first publish, so the agent starts fine with RabbitMQ down. After a failed connect it waits before trying again, doubling the wait each time (1s → 30s max). While it waits, publishes fail fast with `mq.ErrUnavailable` instead of stalling the poll cycle. If a publish fails on an open connection (e.g. the broker restarted), the publisher drops that connection and retries once on a new one. Leave `RABBITMQ_URL` empty to turn publishing off. To test publishing, see [`docs/testing-rabbitmq-publishing.md`](docs/testing-rabbitmq-publishing.md).

Independently, **`api.NewRouter`** wires a [chi](https://github.com/go-chi/chi) router with request-ID, panic-recovery, and structured-request-logging middleware, backed by the same `*store.Store`. Postgres writes from the poll loop and API reads happen concurrently against the same connection pool.

### Package layout

| Package | Responsibility |
|---|---|
| `cmd/agent` | Wires everything together; owns the poll loop, HTTP server lifecycle, and shutdown |
| `internal/config` | Loads settings from environment variables (`internal/config/config.go`) |
| `internal/k8s` | Builds Kubernetes clientsets (in-cluster or kubeconfig fallback) and polls/joins pod + metrics data |
| `internal/ingest` | Wires one poller to one store and one publisher for a single poll-persist-publish cycle |
| `internal/mq` | RabbitMQ `metrics.raw` topology, message schema, and the reconnecting publisher |
| `internal/store` | Postgres/TimescaleDB access: connection pool, embedded SQL migrations, queries |
| `internal/api` | chi router and HTTP handlers for the REST API |
| `internal/models` | JSON response shapes returned by the API |
| `internal/logging` | Builds the shared `slog` JSON logger |
| `internal/integration` | End-to-end test against a real KIND cluster + real Postgres (build-tagged `integration`) |

### REST API

| Method & path | Returns |
|---|---|
| `GET /health` | `{"status": "ok"}` liveness check |
| `GET /api/v1/pods` | Latest known summary for every pod with a metric in the last hour |
| `GET /api/v1/pods/{namespace}/{pod}` | That pod's identity/status plus its last hour of CPU/memory samples |
| `GET /api/v1/pods/{namespace}/{pod}/anomalies` | That pod's anomaly history, most recent first (empty until the Python detector exists) |

## Requirements

- Go **1.26+** (matches `go.mod`; check your version with `go version` — this repo targets a newer toolchain than the `golang:1.23` base image currently pinned in the `Dockerfile`)
- [Docker](https://docs.docker.com/get-docker/), for the local TimescaleDB instance
- [`kind`](https://kind.sigs.k8s.io/docs/user-guide/quick-start/#installation) and [`kubectl`](https://kubernetes.io/docs/tasks/tools/#kubectl), for a local cluster to poll against
- A reachable Kubernetes cluster with [metrics-server](https://github.com/kubernetes-sigs/metrics-server) installed (the KIND setup below covers this)

## Local setup

A `Makefile` in this directory wraps the commands below (`make infra-up`, `make kind-up`, `make run`, `make test`, `make test-integration`, `make build`) — either approach works; both are shown here.

### 1. Start Postgres/TimescaleDB

```bash
cp infra/.env.example infra/.env   # first time only; adjust credentials if you want
docker compose -f infra/docker-compose.yml up -d timescaledb
# or: make -C go infra-up (starts rabbitmq too)
```

Verify it's up:

```bash
docker compose -f infra/docker-compose.yml exec timescaledb pg_isready -U postgres
```

Full details (credentials, RabbitMQ service, teardown): [`../docs/infra/docker-compose.md`](../docs/infra/docker-compose.md).

### 2. Start a local Kubernetes cluster with metrics-server

```bash
kind create cluster --config infra/kind/kind-config.yaml
kubectl --context kind-podsentinel apply -f infra/kind/metrics-server.yaml
kubectl --context kind-podsentinel -n kube-system rollout status deployment/metrics-server
# or: make -C go kind-up
```

Metrics-server needs roughly a minute after rollout to complete its first scrape — `kubectl --context kind-podsentinel top pods -A` should start printing numbers once it has. Full details: [`../docs/infra/kind.md`](../docs/infra/kind.md).

### 3. Configure the agent

```bash
cd go
cp .env.example .env   # first time only; defaults match infra/.env.example
export $(grep -v '^#' .env | xargs)   # or use direnv/dotenv — .env isn't auto-loaded
```

`POSTGRES_DSN` defaults to `postgres://postgres:postgres@localhost:5432/podsentinel?sslmode=disable`, matching `infra/.env.example`. See `.env.example` for every variable (`PORT`, `LOG_LEVEL`, `WATCH_NAMESPACES`, `POLL_INTERVAL`, `CORS_ORIGIN`).

### 4. Run the agent

```bash
go run ./cmd/agent
# or: make run
```

It applies pending Postgres migrations on startup (safe to re-run — already-applied migrations are a no-op), then starts polling and serving. Since `KUBECONFIG`/`~/.kube/config` already points at `kind-podsentinel` after `kind create cluster`, no extra Kubernetes config is needed locally; in-cluster credentials are used automatically when deployed inside the cluster instead.

Confirm it's working:

```bash
curl localhost:8080/health
curl localhost:8080/api/v1/pods
```

## Testing

From the `go/` directory:

```bash
go test ./...
# or: make test
```

This runs all unit tests, including `internal/store`'s tests against a real Postgres instance. Those tests call `t.Skip` (not fail) if `infra/docker-compose.yml`'s `timescaledb` service isn't reachable at the default DSN — start it first (step 1 above) to have them actually run. Point at a different instance with `TEST_POSTGRES_DSN`. Likewise, `internal/mq`'s broker tests skip unless `rabbitmq` is reachable at `amqp://guest:guest@localhost:5672/`; override with `TEST_RABBITMQ_URL` if `infra/.env` uses other credentials.

### Integration test

`internal/integration` exercises the real ingestion path end-to-end: creates a namespace/pod on a live cluster, runs `ingest.Run` on a loop, and asserts the pod shows up via `store.ListPods` within 90 seconds. It's build-tagged out of the default `go test ./...` run:

```bash
go test -tags=integration ./internal/integration/...
# or: make test-integration
```

Requires the `kind-podsentinel` cluster (step 2 above) as the current `kubectl` context, and the `timescaledb` service running (step 1).
