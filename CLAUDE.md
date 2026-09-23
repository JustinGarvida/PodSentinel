# CLAUDE.md

Guidance for Claude Code sessions working in this repository.

## Project Summary

PodSentinel monitors Kubernetes pod CPU/memory usage, detects anomalies with a statistical model, and displays them in a React/TypeScript dashboard. It's an experimental, single-cluster project intentionally combining Go, Python, RabbitMQ, and React/TS.

**Current stage**: Go agent's core ingestion path implemented (Kubernetes polling → Postgres/TimescaleDB → REST API); the React dashboard's pod list view is built (polls the REST API); Python anomaly detection, RabbitMQ pub/sub, the anomaly timeline, and the pod detail view are not yet built — see the suggested build order in `docs/architecture.md`.

## Tech Stack & Why

- **Go** — chosen for the metrics ingestion agent because it pairs naturally with `client-go` for talking to the Kubernetes API, and is fast enough to run as a lightweight in-cluster Deployment. The same Go binary handles polling, Postgres writes, RabbitMQ pub/sub, and the REST API — deliberately one service, not several, to keep this experimental project's ops surface small.
- **Python** — chosen for anomaly detection to make use of its statistics/ML ecosystem, and because the user specifically wanted to experiment with Python for the ML side of this project.
- **RabbitMQ** — the event bus between Go (ingestion) and Python (detection), and for publishing anomaly events. Chosen specifically as an experiment with message-queue-driven architecture, not because scale requires it.
- **PostgreSQL / TimescaleDB** — source of truth for raw metrics and anomaly history; TimescaleDB extension for efficient time-series storage/queries.
- **React + TypeScript** — dashboard frontend, reading from the Go REST API.

## Planned Repo Structure

```
go/         # Go agent: metrics ingestion, RabbitMQ pub/sub, REST API
python/     # Python anomaly detection service
frontend/   # React + TypeScript dashboard
docs/       # Design and architecture documentation
```

## Key Architectural Decisions (preserve this context)

- **Per-pod RabbitMQ messages, not batched.** Deliberate choice: one malformed payload should only affect one pod, not a whole batch. Malformed messages go to a dead-letter queue rather than crashing a consumer.
- **Go is the sole Postgres writer.** Python never writes to Postgres directly — it only publishes anomaly events to RabbitMQ, which Go consumes and persists. This keeps all database access centralized in one service.
- **Statistical baseline before ML.** Anomaly detection starts with z-score/EWMA-based deviation from a pod's rolling recent baseline — explainable, no training data needed. An ML model (e.g. Isolation Forest) is a documented future upgrade, not the starting point.
- **Postgres writes don't depend on RabbitMQ.** Go writes raw metrics to Postgres directly during polling, independent of whether the RabbitMQ publish succeeds — core recording survives queue outages.
- **RabbitMQ's `anomalies.detected` exchange has no built-in alert consumer yet.** Only Go consumes it today (to persist anomalies for the dashboard). The schema is designed so a future alerting consumer (Slack/email/webhook) could bind its own queue without changing the publisher.
- **Pod history tracking keys on UID + owner, not just pod name.** A replaced pod gets a new name and UID, so `pod_metrics` also stores `pod_uid` and the resolved `owner_kind`/`owner_name` (from `ownerReferences`, not Helm labels) to correlate history across respins.

## Commit Conventions

This repo follows [Conventional Commits](https://gist.github.com/qoomon/5dfcdf8eec66a051ecd85625518cfd13):

```
<type>(<optional scope>)<!>: <description>

<optional body>

<optional footer>
```

- **Types**: `feat`, `fix`, `refactor`, `perf`, `style`, `test`, `docs`, `build`, `ops`, `chore`
- **Description**: imperative present tense ("add" not "added"), lowercase first letter, no trailing period.
- **Breaking changes**: append `!` before the colon (e.g. `feat(api)!: remove status endpoint`) and explain in a `BREAKING CHANGE:` footer.
- **Scope**: optional, project-specific context (e.g. `feat(go-agent): ...`). Don't use issue IDs as scope.

A `commit-msg` hook in `.githooks/commit-msg` enforces the header format. It isn't active by default — run `git config core.hooksPath .githooks` once per clone to enable it.

## PR Conventions

- **Title**: same format as a commit header — `<type>(<optional scope>)<!>: <description>` — using the same types, imperative present tense, and lowercase-first-letter rules as [Commit Conventions](#commit-conventions). If a branch's commits span multiple types, pick the type of the overall change (e.g. a refactor with an accompanying test counts as `refactor`, not `test`).
- **Body**: always use this template (also checked in as the default GitHub PR template at [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md)) —

  ```markdown
  ## What
  <1-3 bullet points on what changed>

  ## Why
  <the motivation — bug, feature request, architectural decision, etc.>

  ## Additional Notes
  <anything else reviewers should know: breaking changes, follow-ups, tradeoffs; omit if none>

  ## Testing Plans
  <bulleted checklist of how this was/should be verified>
  ```

- **Breaking changes**: note them under `## Additional Notes`, same content as the commit's `BREAKING CHANGE:` footer.
- Keep the title under 70 characters; put detail in the body, not the title.

## Reference

Full architecture, component responsibilities, error handling, testing strategy, and the suggested build order live in [`docs/architecture.md`](docs/architecture.md) — treat it as the source of truth when implementing any component.
