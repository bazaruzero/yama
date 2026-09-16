## Why

PostgreSQL deployments need lightweight, self-hosted metric collection without the operational weight of full monitoring stacks (Prometheus + exporters + Grafana). YAMA (Yet Another Monitoring Agent) fills this gap as a single-binary Go agent with an embedded store. This change bootstraps the backend core (Phase 1): the foundation every later phase — richer metrics, retention, the web dashboard — builds on.

## What Changes

- Introduce the `yama` backend binary in Go: pure Go, no external runtime dependencies, deployable as "copy binary → run".
- Add two-file YAML configuration: an agent config (PostgreSQL connection fields — host, port, database, user, password — global collection interval, storage and API settings) and a metrics config (named metrics with SQL query, type counter/gauge, per-metric interval override).
- Add a collection engine that connects to PostgreSQL and runs each enabled metric's SQL query on its own schedule; Phase 1 ships with a single built-in test metric (`SELECT 1`).
- Add embedded metric persistence using BadgerDB (key-value).
- Add a REST API endpoint exposing collected metric data points for a metric over a time range (default: the last 3 hours up to now), so a separate frontend binary can visualize them later.

Non-goals for this change: the frontend/dashboard binary, metric retention/cleanup policies, authentication on the API, and any metric beyond the `SELECT 1` connectivity test.

## Capabilities

### New Capabilities

- `agent-config`: Loading and validating the two YAML configuration files (agent config + metrics config), including per-metric overrides (interval, type, enabled) and fail-fast validation with clear errors.
- `metric-collection`: Connecting to PostgreSQL via the configured connection fields and executing configured metric queries on independent schedules (global interval with per-metric override), producing typed data points (counter/gauge) with timestamps; graceful start/stop of the collection loop.
- `metric-storage`: Persisting collected metric data points in an embedded BadgerDB database, keyed for efficient per-metric time-range scans, with clean open/close lifecycle.
- `metric-api`: An HTTP REST API that serves stored metric data points (per metric, over a time range) in JSON for consumption by the future frontend.

### Modified Capabilities

(none — greenfield project, no existing specs)

## Impact

- **New code**: entire Go backend (`cmd/yama`, `internal/...`) in this currently empty repository.
- **Dependencies (Go modules)**: YAML parser, PostgreSQL driver (pure Go, e.g. `pgx`), BadgerDB. Standard library `net/http` for the REST API. No cgo, no SQLite.
- **External systems**: one monitored PostgreSQL instance (read-only queries only).
- **Deployment**: single static binary plus two YAML files; no runtime services beyond PostgreSQL.
