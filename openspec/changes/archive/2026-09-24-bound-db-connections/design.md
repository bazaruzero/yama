## Context

See proposal.md - Why for motivation. Current implementation (observed): `internal/postgres.Open` builds the pool via `pgxpool.New(ctx, connStr)` with no pool configuration, so the ceiling is pgxpool's default `max(4, NumCPU)`; `internal/collect.Scheduler` runs one goroutine per metric, all acquiring from that shared pool. Constraints: no new external dependencies (pgxpool already exposes `MaxConns`), backward-compatible YAML schemas, safe-by-default.

## Goals / Non-Goals

**Goals:**
- Hard, operator-configurable ceiling on concurrent PostgreSQL connections: `postgres.max_db_connections`, default 1, reject explicit values < 1.
- Scheduler dispatches collection across a bounded worker pool so concurrency is bounded structurally (workers) and defensively (pool MaxConns).
- No duplicate collection pile-up when collection falls behind schedule.

**Non-Goals:**
- Dynamic/adaptive pool sizing; per-metric connection pinning; changing storage, API, or metrics config schema; restoring parallel-by-default behavior.

## Decisions

### Config field: `postgres.max_db_connections` as `*int`
Placed under `postgres:` because it caps database connections (alternative: `collector:` — rejected, it is a database property). Modeled as `*int` (same tri-state pattern as `Metric.Enabled *bool`): absent → nil → default **1**; explicitly set → validated ≥ 1, else startup error naming the field. Alternative: plain `int` with 0-as-unset — rejected because the success criteria require an explicit `0` to fail startup, and plain int cannot distinguish unset from explicit 0.

### Pool ceiling: `pgxpool.ParseConfig` + `MaxConns`
Replace `pgxpool.New(ctx, connStr)` with `ParseConfig(connStr)` → set `MaxConns = int32(n)` → `NewWithConfig`. `MinConns` stays 0 (connections open lazily; MaxConns is a ceiling, not a preallocation — observed connection count may be below the cap, which satisfies "never exceeds"). Ping and `QueryValue` unchanged. Factored into a pure `buildPoolConfig(connStr, maxConns)` helper so the mapping is unit-testable without a database.

### Scheduler: interval-driven cycles with a next-due timer, shared cycle timestamp, bounded workers
Collection is organized into cycles. After each cycle (and at startup, immediately), the scheduler computes `next = min over enabled metrics of (lastEnqueued[name] + effective interval)` and sets a one-shot timer to exactly that instant. At each wake, `X := time.Now().UTC()` is captured once; every enabled metric with `X - lastEnqueued[name] >= effective interval` is due and its job — carrying the shared timestamp X — is pushed onto the dispatch queue in configuration order. Waking at the earliest due instant (rather than on a fixed grid) means each metric fires at its own cadence plus only timer/scheduling jitter — never delayed a full wake step — and no idle wakes occur at all.
Jobs are executed by `N = min(max_db_connections, enabled metrics)` workers, so concurrency is ≤ N ≤ `max_db_connections` (pgxpool's `MaxConns` enforces the same ceiling defensively); with the default 1 the cycle is strictly sequential in config order.
Queue discipline: a per-metric "pending" set (mutex-guarded) skips enqueue when that metric's job is still queued or running. `lastEnqueued` is updated at enqueue time even when a dedup drop occurs, keeping each metric phase-aligned and preventing unbounded catch-up bursts; a dropped cycle is a lost sample for that interval (the backlog case is a sustained-overload condition, where sampling sparsely is preferable to queueing unboundedly).

Alternative considered: GCD wake grid (previous revision). Waking at `GCD(effective intervals)` quantizes due instants to the grid, so a wake landing a hair before a metric's full interval elapsed (ticker drift is sub-millisecond, the due check compares exact nanoseconds) pushes that metric a full grid step late — e.g. a 7s metric on a 1s grid firing at 8s. Replaced by the next-due timer.
Alternative considered: busy loop polling due-ness. Equivalent precision but burns CPU continuously; the one-shot timer achieves the same wake instants with zero idle cost.

### Timestamps: cycle wake time, not completion time
`CollectOnce` takes the cycle timestamp as a parameter; the stored `Point.Timestamp` is exactly X for every metric in the cycle. This keeps series aligned across metrics regardless of query duration or queue position (the previous completion-time stamping drifted by total cycle execution time).

### Shutdown
Unchanged contract: context cancel stops the cycle goroutine and workers; workers finish their current job (`CollectOnce`'s context handles cancellation); `Run` returns after the WaitGroup drains. Queued-but-unstarted jobs are discarded; those metrics resume on their next due cycle after restart.

## Risks / Trade-offs

- [Default 1 serializes collection — throughput regression for existing users] → Accepted (proposal's stated trade-off). Mitigation: documented one-line opt-up; README sizing note: choose N so Σ(query time per cycle) comfortably fits within the global interval.
- [One slow query delays all queued collections when N=1] → Bounded by per-run `query_timeout`; dedup prevents queue pile-up; collection resumes next tick. Spurious-failure semantics avoided by design (timeout applies to execution, not queue wait).
- [Worker pool and pgxpool could drift if misconfigured independently] → Both derive from the same config value; workers are derived as `min(max_db_connections, metric count)` and can never exceed the pool ceiling.
- [pg_stat_activity verification noise from other clients] → Verification queries filter by the agent's `usename` (see tasks).

## Open Questions

(none — field placement and tri-state semantics are recorded above as decisions)
