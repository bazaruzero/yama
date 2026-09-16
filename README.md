# YAMA — Yet Another Monitoring Agent

A minimal PostgreSQL metrics collector. Single static binary, zero runtime dependencies.

## Features

- Collects PostgreSQL metrics via configurable SQL queries
- Per-metric enable/disable and collection interval overrides
- Embedded BadgerDB storage — no external database server
- REST API for querying collected data points
- Graceful shutdown on SIGTERM/SIGINT
- Static binary, pure Go: copy and run

## Prerequisites

- A running PostgreSQL instance
- Writable filesystem for the data directory

## Quick Start

### 1. Build

```
make build
```

Produces `./bin/yama`.

### 2. Configure

```
cp configs/config.yaml.example ./config.yaml
cp configs/metrics.yaml.example ./metrics.yaml
```

Edit `config.yaml` — set at minimum `postgres.host` and `postgres.user`.
The password can be left empty and supplied via the `PG_PASSWORD`
environment variable.

### 3. Run

```
export PG_PASSWORD="your-password"
./bin/yama --config ./config.yaml --metrics ./metrics.yaml
```

### 4. Query collected data

```
curl "http://localhost:8080/api/v1/metrics/select_1/data"
```

Returns a JSON array of `{"timestamp":"...","value":...}` points for the
last 3 hours. Optional `from`/`to` RFC3339 query parameters adjust the
window (`from` defaults to now − 3h, `to` defaults to now).

### 5. Stop

`Ctrl+C` or `kill -TERM <pid>` — the agent drains and exits cleanly.

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

## Development

```
make build    # build binary
make test     # run tests
make lint     # go vet + gofmt check
make clean    # remove build artifacts
```

## Project Structure

```
cmd/yama/              Entry point
internal/config/       Configuration loading and validation
internal/postgres/     PostgreSQL connection management
internal/collect/      Metric collector and scheduler
internal/store/        BadgerDB storage layer
internal/api/          REST API handlers
configs/               Example configuration files
```
