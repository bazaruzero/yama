## Context

Greenfield repository (currently only `openspec/` exists). See proposal.md for motivation and scope. Hard constraints: pure Go, no cgo, no SQLite, minimal module dependencies, single static binary.

## Goals / Non-Goals

**Goals:**
- Runnable `yama` binary that loads two YAML files, connects to PostgreSQL, collects the `SELECT 1` test metric on a schedule, persists points to BadgerDB, and serves them over a JSON REST API.
- Package boundaries that later phases (more metrics, retention, frontend) extend without rework.
- CGO-free static build; runtime footprint of exactly one binary + two YAML files.

**Non-Goals:**
- Retention/cleanup of stored data, BadgerDB value-log GC tuning.
- Counter rate/delta math (Phase 1 stores raw values for both types).
- API authentication, TLS, pagination, and metric-list endpoints.
- The frontend binary and any deployment packaging (deb/rpm/docker).

## Decisions

### Module layout
`cmd/yama/main.go` (wiring only) + `internal/` packages: `config` (load/validate both YAML files), `postgres` (connection pool lifecycle), `collect` (metric registry + scheduler), `store` (persistence behind a narrow interface), `api` (HTTP handlers). Rationale: standard Go project layout with one package per concern keeps dependencies one-directional (collect → store, api → store) and each package independently testable; `store` behind an interface keeps BadgerDB details from leaking into `collect`/`api`. Alternative considered: flat single package — rejected, it couples config/storage/http together and complicates testing.

### PostgreSQL driver: `github.com/jackc/pgx/v5` (stdlib `database/sql` mode)
Pure Go, actively maintained; the pool connection is built internally from the config's structured fields (host/port/database/user/password/sslmode), so no DSN parsing is needed. Alternative: `lib/pq` — effectively in maintenance mode; rejected. Connectivity check at startup is a single `PingContext` with timeout.

### YAML: `gopkg.in/yaml.v3`
De facto standard, pure Go, supports strict field checking (`KnownFields`) so typos in config fail validation. Alternative: `sigs.k8s.io/yaml` — JSON round-trip indirection with no benefit here.

### Storage: BadgerDB v4 with time-ordered keys
- Key: `<metric-name>` + `0x00` + 8-byte big-endian Unix-nano timestamp. Big-endian makes lexicographic byte order == chronological order, so a BadgerDB iterator over prefix `<metric-name>\x00` with optional seek to the range start yields the spec's ascending time-range scan directly. Timestamp-only keys risk collision for same-nanosecond writes; acceptable at second-scale intervals, and a nanosecond collision would simply overwrite an identical point.
- Value: 1-byte type tag (`c`/`g`) + 8-byte big-endian float64. The key/name alone cannot recover the metric type, so it must be persisted with the point.
- Rationale: user-mandated, embedded, LSM-tree write throughput suits append-heavy metric workloads. Alternative considered: bbolt — single-writer lock and mmap growth pauses make it weaker for append-heavy time series; rejected.

### Collection scheduling: per-metric ticker goroutines
Each enabled metric gets a goroutine with `time.Ticker` at its effective interval; all share a `context.Context` from `signal.NotifyContext` (SIGINT/SIGTERM) for cancellation, with a `sync.WaitGroup` for drain-on-shutdown. Query execution uses a per-run context timeout (from config, default ~3s) so a hung query cancels instead of leaking. Rationale: stdlib-only, and per-metric isolation satisfies the spec requirement that one metric never delays another. Alternative: single loop with a min-heap next-run queue — fewer goroutines but more code and shared-delay risk; rejected.

### API: stdlib `net/http` with Go 1.22+ method patterns
Single endpoint: `GET /api/v1/metrics/{name}/data?from=<rfc3339>&to=<rfc3339>` → JSON `[{"timestamp":"...","value":1}]`. Time window: `from` defaults to now − 3h, `to` defaults to now. Non-GET → 405 via method-pattern routing. Alternative: chi/gorilla — unnecessary dependency for one route.

### Config shapes
Agent config: `postgres.host` (required), `postgres.user` (required), `postgres.port` (default 5432), `postgres.database` (default `postgres`), `postgres.password` (optional; falls back to the `PG_PASSWORD` env var), `postgres.sslmode` (default `prefer`), `collector.interval` (duration, default 10s), `collector.query_timeout` (default 3s), `storage.data_dir` (default `./data`), `api.listen` (default `:8080`). Metrics config: list of `{name, query, type, enabled, interval}`. Ships with a default `metrics.yaml` containing the `select_1` test metric.

### Shutdown ordering
Signal → cancel scheduler context → stop accepting API requests (`http.Server.Shutdown`) → wait for in-flight collectors → close BadgerDB. Store closes last so in-flight writes land.

## Risks / Trade-offs

- [BadgerDB data grows unbounded without retention] → Accepted for Phase 1; retention cleanup is an explicit follow-up change. Default intervals keep growth trivial (~8 KB/day for the test metric).
- [Ticker-based scheduling drifts under long queries] → Per-run timeout caps execution; minor interval skew is acceptable for monitoring data and documented in the test-metric expectations.
- [BadgerDB adds ~15–20 MB to the binary] → Accepted trade-off for an embedded store; still a single portable static binary.
- [Connection loss mid-run produces error logs but no auto-reconnect backoff] → pgxpool retries per query; persistent outages surface as logged query failures per the spec's "collection continues" requirement. Exponential backoff alerting is deferred.

## Open Questions

- Exact HTTP error body shape (`{"error": "..."}` assumed) — cosmetic, safe to settle during implementation.
