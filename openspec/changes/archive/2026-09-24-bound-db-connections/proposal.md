## Why

The agent currently opens its PostgreSQL connections through a pgxpool with default settings, whose size is `max(4, NumCPU)` — not operator-configurable. (Observation from `internal/postgres/postgres.go:30`: `pgxpool.New` is called with no pool configuration; the scheduler runs one goroutine per metric, all contending on this pool. So the count is bounded-but-unconfigurable rather than one-per-metric as originally stated — but the production problem stands: with large metric sets, the agent can hold several connections to a monitored database and operators have no way to cap it. On instances with tight `max_connections` budgets this is a scalability blocker.) This change makes the connection ceiling an explicit, safe-by-default configuration value.

## What Changes

- Add `postgres.max_db_connections` to the agent config: the maximum number of concurrent PostgreSQL connections the agent may hold. Unset → **1** (conservative default, never unlimited). An explicit value below 1 (including 0) fails startup with a clear error naming the field.
- Enforce the ceiling in the connection layer: the pool is configured with `MaxConns = max_db_connections`, so concurrent connections never exceed the configured value at any point in time.
- Rework the scheduler from "one goroutine per metric hitting the pool" to cycle-driven collection: a next-due timer wakes the agent at the earliest instant any metric is due; each cycle at time X collects every metric due at X (in config order, bounded by the pool); all points in a cycle share the timestamp X. Shorter per-metric intervals are collected more frequently than the global cadence; longer ones only when due. A metric whose job is still in flight when next due skips that cycle rather than piling up.
- **BREAKING (behavioral)**: the default concurrent connection count drops from `max(4, NumCPU)` to **1**, serializing collection by default. Accepted trade-off in favor of database safety; operators who want parallelism set `max_db_connections` explicitly.
- **Spec-level change**: `metric-collection`'s "Scheduled collection" requirement currently guarantees each metric is "scheduled independently so one metric's execution does not delay another's". A bounded pool necessarily bounds that independence — the guarantee becomes "ticks fire independently; query execution is bounded by the pool and may be delayed under contention". This requirement is modified accordingly.

Non-goals: dynamic/adaptive pool sizing, per-metric connection isolation, changes to storage or the API, new external dependencies.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `agent-config`: The "Agent configuration file" requirement gains the `max_db_connections` field: optional, defaults to 1, explicit values below 1 rejected at startup with an error naming the field.
- `metric-collection`: The "Scheduled collection" requirement changes — collection tasks are dispatched to a bounded worker pool of `max_db_connections` workers; concurrent database connections never exceed that value; ticking stays per-metric and non-blocking, but query execution may be delayed by pool contention instead of being fully independent.

## Impact

- **Code**: `internal/config` (new field + validation), `internal/postgres` (pool MaxConns), `internal/collect` (scheduler worker-pool rework), `cmd/yama` (wiring the new value through), `configs/config.yaml.example` and README (document the field).
- **Compatibility**: config/metrics YAML schemas remain backward compatible (new optional field only). Existing stored data and the REST API are unaffected. Default deployment behavior changes: at most 1 connection instead of up to `max(4, NumCPU)`.
- **Dependencies**: none added — pgxpool already supports `MaxConns`.
- **Risk accepted**: throughput reduction under the default of 1 (documented above).
