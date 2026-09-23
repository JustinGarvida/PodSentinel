# PodSentinel Architecture

## Overview

PodSentinel monitors CPU and memory usage for Kubernetes pods, detects anomalies using a statistical model, and surfaces them through a live dashboard. It's an experiment in combining Go, Python, RabbitMQ, and React/TypeScript into a single event-driven system.

At a high level:

```
Kubernetes Metrics API
        │
        ▼
   Go Agent ──────────────► Postgres/TimescaleDB (raw metrics)
        │                          ▲
        │ publish (per pod)        │ write (anomalies)
        ▼                          │
   RabbitMQ: metrics.raw           │
        │                          │
        ▼                          │
Python Anomaly Detector            │
        │                          │
        │ publish (per anomaly)    │
        ▼                          │
   RabbitMQ: anomalies.detected ───┘
        │
        ▼
   Go Agent (consumer) ─────► REST API ─────► React/TS Dashboard
```

## Components

### Go Agent

A single Go service that does four things:

1. **Poller** — polls the Kubernetes Metrics API (`metrics.k8s.io`, via `client-go`) every 15-30 seconds (configurable) for pod-level CPU and memory usage.
2. **Postgres writer** — writes every raw metric sample directly to Postgres. This happens independently of RabbitMQ, so core recording never depends on the queue being up.
3. **RabbitMQ publisher** — publishes **one message per pod per polling cycle** to the `metrics.raw` topic exchange, with routing key `<namespace>.<pod>`. Messages are per-pod (not batched) so a malformed payload for one pod can't affect any other pod's data.
4. **RabbitMQ consumer + REST API** — consumes anomaly events from `anomalies.detected`, persists them to Postgres, and exposes a REST API that the dashboard reads from (pod list, live/historical metrics, anomaly history). No separate API service — the same Go binary serves all of this.

### Python Anomaly Detector

A stateless-ish Python service that:

- Consumes raw metric messages from `metrics.raw`.
- Maintains a rolling per-pod window (in-memory) of recent CPU/memory samples.
- Runs statistical anomaly detection (z-score / EWMA-based deviation from the pod's recent baseline) — no ML model, no training data required. This is deliberately the simplest approach that gives a real signal; see "Future work" for an ML upgrade path.
- On detecting an anomaly, publishes a structured event to `anomalies.detected` (pod, namespace, metric, value, baseline, severity, timestamp).
- Malformed or unparseable messages are nack'd to a dead-letter queue instead of crashing the consumer loop — fault isolation matches the per-pod message granularity upstream.

### RabbitMQ

Two topic exchanges, each with a durable queue and a dead-letter queue:

- `metrics.raw` — raw per-pod metric samples, Go → Python.
- `anomalies.detected` — anomaly events, Python → Go. This exchange is intentionally general-purpose: only Go consumes from it today, but the schema is designed so future consumers (e.g. an alerting service) can bind their own queue without changes to the publisher.

RabbitMQ sits in the critical path between ingestion and detection (not just as an outbound notification bus), which is part of why per-pod messages and DLQs matter here — a single bad message shouldn't stall the pipeline.

### Postgres / TimescaleDB

- `pod_metrics` — a TimescaleDB hypertable: timestamp, namespace, pod, pod_uid, owner_kind, owner_name, cpu, memory, status, restart_count. `pod_uid` and `owner_kind`/`owner_name` (resolved from the pod's `ownerReferences`, following a ReplicaSet up to its own Deployment where present) let a workload's pod-respin history be tracked across pod churn — a crashed pod under a Deployment is deleted and replaced by a new Pod object with a new name and UID, which plain `(namespace, pod)` can't correlate. This is separate from `restart_count`, which only counts in-place container restarts within one pod's lifetime. Owner resolution is Kubernetes-native (`ownerReferences`), not Helm-release-aware — it works the same regardless of how the workload was deployed, but doesn't group by Helm release (`app.kubernetes.io/instance`), which depends on chart convention rather than a guaranteed API relationship.
- `anomalies` — timestamp, namespace, pod, metric, value, baseline/threshold, severity.

Postgres is the source of truth for both raw metrics and anomaly history, and is what the dashboard's REST API reads from.

### React / TypeScript Dashboard

- **Pod list view** — namespace, pod name, status, restart count, live CPU/memory.
- **Pod detail view** — CPU/memory time-series chart plus an anomaly timeline for that specific pod.

The dashboard talks only to the Go Agent's REST API (polling/refresh interval to start; streaming via WebSockets/SSE is a possible future enhancement, not part of the initial build).

## Error Handling & Fault Isolation

- A failed metrics fetch for one pod doesn't block others in the same polling cycle — it's logged and skipped.
- RabbitMQ publish failures are retried with backoff and logged; since Postgres writes happen independently, metrics are never lost even if publishing fails.
- Malformed messages on either exchange go to a DLQ rather than crashing a consumer.
- This is an experimental/single-cluster project, not a production system — durability guarantees are kept simple (log-and-continue) rather than building enterprise-grade retry/backpressure infrastructure.

## Testing & Verification Strategy

- **Unit tests**: Go poller/publisher logic; Python detection math run against synthetic time series with known, injected anomalies.
- **Integration test**: [docker-compose](infra/docker-compose.md) spinning up RabbitMQ + Postgres + both services, using a mocked/fake metrics source, verifying an injected anomaly flows end-to-end and lands in Postgres.
- **Manual verification**: deploy to a local kind/minikube cluster with metrics-server installed, generate CPU load in a test pod (e.g. `stress-ng`), confirm the anomaly appears in the dashboard.

## Suggested Build Order

1. Go agent: metrics polling + Postgres writes + REST API + dashboard pod list. No anomaly detection yet — this proves the core ingestion path end-to-end.
2. Add RabbitMQ: Go publishes raw metrics; a stub Python consumer just logs what it receives.
3. Implement real statistical anomaly detection in Python, publishing anomaly events.
4. Go consumes anomaly events and persists them; dashboard adds the anomaly timeline.
5. Polish: pod detail view, configurable thresholds.

## Open Questions / Future Work

Explicitly out of scope for the initial build:

- Multi-cluster support (currently: single in-cluster agent via RBAC, not an external service watching multiple kubeconfig contexts).
- Upgrading detection from statistical baseline to an ML model (e.g. Isolation Forest) once enough historical data exists.
- Building an actual alerting consumer (Slack/email/webhook) off `anomalies.detected` — the queue and schema are ready for this, but no consumer is built yet.
- Dashboard authentication/multi-user support.
