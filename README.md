# YAMA — Yet Another Monitoring Agent

A minimal PostgreSQL metrics collector with a companion web dashboard. Single static binaries, zero runtime dependencies.

## Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Web Dashboard (yama-web)](#web-dashboard-yama-web)
- [How collection is scheduled](#how-collection-is-scheduled)
- [Adding Metrics](#adding-metrics)
- [Development](#development)
- [Project Structure](#project-structure)

## Features

- Collects PostgreSQL metrics via configurable SQL queries
- Per-metric enable/disable and collection interval overrides
- Embedded BadgerDB storage — no external database server
- REST API for querying collected data points
- Web dashboard (`yama-web`): live auto-refreshing panels served by a standalone binary
- Graceful shutdown on SIGTERM/SIGINT
- Static binaries, pure Go: copy and run

## Prerequisites

- A running PostgreSQL instance
- Writable filesystem for the data directory

## Quick Start

### 1. Build

```
make build
```

Produces `./bin/yama` (the agent) and `./bin/yama-web` (the dashboard
frontend).

### 2. Configure

```
cp configs/config.yaml.example ./config.yaml
cp configs/metrics.yaml.example ./metrics.yaml
```

Edit `config.yaml` — set at minimum `postgres.host` and `postgres.user`.
The password can be left empty and supplied via the `PG_PASSWORD`
environment variable.

By default the agent uses at most **1** concurrent PostgreSQL connection
(`postgres.max_db_connections`, minimum 1): all metric collections run
serially over it. Raise it only if the monitored instance can afford more
connections — as a rule of thumb, pick `N` so that the total query time of
one collection cycle comfortably fits within your shortest interval.

### 3. Run

```
export PG_PASSWORD="your-password"
./bin/yama --config ./config.yaml --metrics ./metrics.yaml
```

### 4. Query collected data

All requests return a JSON array of `{"timestamp":"...","value":...}` points
ordered by timestamp ascending; a metric with no points in the window
returns `[]`.

Last 3 hours up to now (the default window):

```
curl "http://localhost:8080/api/v1/metrics/select_1/data"
```

A specific time period via `from`/`to` (RFC3339 timestamps, both inclusive;
`to` defaults to now when omitted):

```
curl "http://localhost:8080/api/v1/metrics/select_1/data?from=2026-09-24T10:00:00Z&to=2026-09-24T11:00:00Z"
```

From a fixed point until now:

```
curl "http://localhost:8080/api/v1/metrics/select_1/data?from=2026-09-24T10:30:00Z"
```

Example response:

```json
[
  {"timestamp":"2026-09-24T10:00:05.123456789Z","value":1},
  {"timestamp":"2026-09-24T10:00:15.123501002Z","value":1}
]
```

Unparseable `from`/`to` values return `400 Bad Request` with a JSON error
naming the invalid parameter; non-GET methods return `405 Method Not Allowed`.

### 5. Stop

`Ctrl+C` or `kill -TERM <pid>` — the agent drains and exits cleanly.

## Web Dashboard (yama-web)

`yama-web` is an independent dashboard binary: point it at any running,
**unmodified** agent and it serves a Grafana-style page of that agent's
metrics — no SSH, no `curl`, no Grafana server to install. It talks to the
agent's read-only API server-side (the browser only ever talks to the
frontend), so it can run on your laptop against a remote agent with just
the agent's HTTP port reachable.

### 1. Configure

```
cp configs/webui.yaml.example ./webui.yaml
cp configs/graphs.yaml.example ./graphs.yaml
```

`webui.yaml` (system settings):

| Field | Default | Meaning |
|---|---|---|
| `agent.host` | `localhost` | Host of the YAMA agent to visualize |
| `agent.port` | `8080` | The agent's `api.listen` port |
| `agent.timeout` | `5s` | Per-request timeout for calls to the agent |
| `ui.listen` | `:8081` | Dashboard listen address |
| `refresh.interval` | `10s` | Automatic panel refresh interval |

`graphs.yaml` defines the panels — one per graph, in file order (at most
two per row):

```yaml
graphs:
  - name: "Active sessions"
    metric: pg_active_sessions
    description: "count(*) from pg_stat_activity"
```

`name` is the panel title, `metric` is the metric name from the agent's
`metrics.yaml`, `description` is optional. Graphs are defined exclusively
in this file — there is no UI-side editing.

### 2. Run

```
./bin/yama-web --config ./webui.yaml --graphs ./graphs.yaml
```

Then open `http://localhost:8081`.

### Behavior

- Each panel shows the **last hour** of its metric as a smooth line chart
  (min/max/current legend under the chart, tooltips on every point) and
  refreshes automatically every `refresh.interval` — no page reload, no
  user action.
- A metric with no data in the window renders an explicit "No data in the
  last hour" panel; the rest of the page is unaffected.
- If the agent becomes unreachable, a visible banner appears and panels
  switch to an error state; when the agent comes back, everything recovers
  automatically — no frontend restart needed. The frontend also starts
  fine while the agent is down.
- The page uses a light warm theme; all assets (including the vendored
  htmx) are embedded in the binary and served by the frontend itself — no
  CDN, no external requests, no Node/npm toolchain.

## How collection is scheduled

Collection runs in cycles driven by `collector.interval`: the agent wakes,
determines every metric whose time since last collection has reached its
effective interval (its own `interval` override, else the global one), and
collects them all within that cycle — sequentially when
`postgres.max_db_connections` is 1 (the default), or up to that many at a
time otherwise. Every data point in a cycle carries the **same timestamp**:
the cycle wake time, so series stay aligned even when a cycle takes long to
drain.

Per-metric intervals interact with cycles as follows:

- The scheduler wakes exactly when the earliest metric is next due (a timer
  set to `last collection + effective interval`), so each metric is
  collected at approximately its own cadence. A **shorter** per-metric
  interval (e.g. 1s inside a 5s global) is collected every ~1s while global
  metrics stay on the 5s cadence.
- A **longer** per-metric interval (e.g. 30s inside a 5s global) collects
  only when due — every 30s — with no redundant work.
- If a metric's previous collection is still running when it comes due
  again, that cycle is skipped (no queue pile-up) and it resumes on its
  next due cycle.

## Adding Metrics

Edit `metrics.yaml` — no code changes required:

```yaml
metrics:
  - name: pg_active_sessions
    query: "SELECT count(*) FROM pg_stat_activity WHERE state = 'active'"
    type: gauge
    interval: 5s
```

Restart YAMA to apply changes.

### Metric types

Each metric declares one of two types via the `type` field (anything else
is rejected at startup):

| Type | Meaning | Typical use |
|---|---|---|
| `gauge` | A point-in-time value that can go up or down | session counts, table sizes, replication lag |
| `counter` | A cumulative, monotonically increasing value | `pg_stat_*` totals such as transactions or tuples read |

Notes:

- The query MUST return exactly **one row with one numeric column** — that
  value is stored as the data point (raw, for both types; rate/delta
  computation over counters is not performed by the agent).
- Every point of one collection cycle shares the cycle timestamp (see
  [How collection is scheduled](#how-collection-is-scheduled)), regardless
  of metric type.

## Development

```
make build    # build both binaries (yama, yama-web)
make test     # run tests
make lint     # go vet + gofmt check
make css      # optional: recompile the dashboard CSS (Tailwind standalone CLI)
make clean    # remove build artifacts
```

## Project Structure

```
cmd/yama/              Agent entry point
cmd/yama-web/          Dashboard frontend entry point
internal/config/       Agent configuration loading and validation
internal/postgres/     PostgreSQL connection management
internal/collect/      Metric collector and scheduler
internal/store/        BadgerDB storage layer
internal/api/          REST API handlers
internal/web/          Dashboard frontend (config, agent client, chart, server)
configs/               Example configuration files
```
