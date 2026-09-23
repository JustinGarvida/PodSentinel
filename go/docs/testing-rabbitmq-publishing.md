# Testing RabbitMQ Publishing

How to check that the Go agent publishes per-pod samples to `metrics.raw`. For the topology and message format, see [RabbitMQ publishing](../README.md#rabbitmq-publishing).

Run commands from the repo root unless a step says otherwise.

## 1. Unit tests (no infrastructure)

```bash
cd go && go test -v ./internal/mq/ ./internal/ingest/
```

These use fakes to cover the message body and routing key, reconnecting after a failed publish, the growing wait between connect attempts, and that a failed Postgres insert and a failed publish don't block each other.

## 2. Broker tests (local RabbitMQ)

```bash
docker compose -f infra/docker-compose.yml up -d rabbitmq
cd go
set -a && . ../infra/.env && set +a
export TEST_RABBITMQ_URL="amqp://$RABBITMQ_DEFAULT_USER:$RABBITMQ_DEFAULT_PASS@localhost:5672/"
go test -count=1 -v -run 'RealBroker|DeadLetter' ./internal/mq/
```

- Without `TEST_RABBITMQ_URL`, the tests try `guest`/`guest` and **skip** (they don't fail) if they can't connect, so check for `--- PASS`, not `--- SKIP`.
- If your password has special characters (`@`, `:`, `/`), URL-encode it.
- These tests purge `metrics.raw.detector` and `metrics.raw.dlq`, so only run them against a local broker.

## 3. End to end with the real agent

Start the infrastructure and a cluster:

```bash
docker compose -f infra/docker-compose.yml up -d
make -C go kind-up
```

Run the agent with publishing turned on:

```bash
cd go
export $(grep -v '^#' .env | xargs)
export RABBITMQ_URL="amqp://<user>:<pass>@localhost:5672/"   # your infra/.env credentials
go run ./cmd/agent
```

Watch messages arrive. The Python consumer doesn't exist yet, so messages pile up in the queue:

- **Management UI:** http://localhost:15672 → **Queues** → `metrics.raw.detector`. "Ready" should go up by the number of pods every `POLL_INTERVAL` (15s by default). Click **Get messages** to see a body.
- **CLI:** `docker exec podsentinel-rabbitmq rabbitmqctl list_queues name messages`

Each message should have routing key `<namespace>.<pod>` and a JSON body with `schema_version: 1`.

## 4. Failure scenarios

| Scenario | How | Expected |
|---|---|---|
| Broker goes down | `docker compose -f infra/docker-compose.yml stop rabbitmq` while the agent runs | Logs `connecting to rabbitmq failed` with `retry_in` growing up to 30s; `curl localhost:8080/api/v1/pods` still shows fresh data |
| Broker comes back | `docker compose -f infra/docker-compose.yml start rabbitmq` | `reconnected to rabbitmq` within about 30s; the queue count grows again |
| Agent starts with the broker down | Stop `rabbitmq`, then `go run ./cmd/agent` | The agent starts, serves the API and writes to Postgres; it connects once the broker is up |
| Publishing turned off | `RABBITMQ_URL= go run ./cmd/agent` | One `publishing to rabbitmq disabled` warning and no connect attempts |

## Cleanup

```bash
docker exec podsentinel-rabbitmq rabbitmqctl purge_queue metrics.raw.detector
```
