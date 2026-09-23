# Local RabbitMQ + TimescaleDB (docker-compose)

Guide for the local RabbitMQ and TimescaleDB instances used for development, per the integration testing strategy described in [`architecture.md`](../architecture.md#testing--verification-strategy).

Defined in [`infra/docker-compose.yml`](../../infra/docker-compose.yml): a `rabbitmq` service (with the management UI plugin) and a `timescaledb` service (Postgres + the TimescaleDB extension).

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (with the `docker compose` CLI plugin)

## Configure credentials

```bash
cp infra/.env.example infra/.env
```

Adjust `infra/.env` if you want non-default credentials. `infra/.env` is gitignored — never commit it.

Set credentials *before* the first `up` — both Postgres and RabbitMQ only apply their credential env vars when their (named, persistent) volume is first initialized. Changing `infra/.env` after that point won't change the running containers' credentials; you'll need to reset the volumes first with `docker compose -f infra/docker-compose.yml down -v`.

## Bring the stack up

```bash
docker compose -f infra/docker-compose.yml up -d
```

## Verify

RabbitMQ management UI: http://localhost:15672 (default `guest`/`guest`, or whatever you set in `infra/.env`).

```bash
docker compose -f infra/docker-compose.yml ps
docker compose -f infra/docker-compose.yml exec timescaledb pg_isready -U postgres
```

Both services should show as running, and `pg_isready` should report `accepting connections`. (The `-U postgres` flag above assumes the default `POSTGRES_USER` — adjust it if you changed that in `infra/.env`; `pg_isready` only checks server responsiveness, so it won't fail on a wrong username.)

## Tear down

```bash
# Stop containers, keep data
docker compose -f infra/docker-compose.yml down

# Stop containers and delete volumes (full reset)
docker compose -f infra/docker-compose.yml down -v
```

## Connection details for app config

| Service | Host (from host machine) | Port | Credentials |
|---|---|---|---|
| RabbitMQ (AMQP) | `localhost` | `5672` | `infra/.env`: `RABBITMQ_DEFAULT_USER` / `RABBITMQ_DEFAULT_PASS` |
| RabbitMQ (management UI) | `localhost` | `15672` | same as above |
| TimescaleDB | `localhost` | `5432` | `infra/.env`: `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` |

If the Go agent later runs inside the local KIND cluster instead of on the host, the intended approach is to use `host.docker.internal` in place of `localhost`. This is confirmed to work for a container running directly under Docker Desktop, but has *not* been verified for a pod running inside a KIND node — pod DNS goes through CoreDNS, and KIND rewrites the node's resolv.conf, so `host.docker.internal` will likely need an explicit `hostAliases` entry (or the bridge gateway IP) to resolve from inside a pod. Note that the loopback port bindings above are a tradeoff here — a bridge-gateway-IP route won't reach these services without widening them.

## Next steps

Schema (`pod_metrics`, `anomalies`) is defined by the Go agent's migrations. The `metrics.raw` RabbitMQ topology is declared by the Go agent on connect — see [`go/README.md`](../../go/README.md#rabbitmq-publishing). `anomalies.detected` isn't defined yet; it'll be added alongside the Python detector's anomaly publishing.
