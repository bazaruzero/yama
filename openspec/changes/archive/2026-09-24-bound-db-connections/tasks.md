## 1. Configuration (capability: agent-config)

- [x] 1.1 Add `MaxDBConnections *int` (`yaml:"max_db_connections"`) to `PostgresConfig` in `internal/config`, with validation in `LoadAgent`: nil → default 1; explicit value < 1 (including 0) → error naming `postgres.max_db_connections` and the minimum of 1; verify unit tests cover unset→1, explicit 1 and 5 pass through, 0 and -3 are rejected with the field named
- [x] 1.2 Document `postgres.max_db_connections` in `configs/config.yaml.example` (default 1, minimum 1) and add a sizing note to README; verify the example file still loads via `LoadAgent` in a unit test

## 2. Connection pool ceiling (capability: metric-collection)

- [x] 2.1 Add `buildPoolConfig(connStr string, maxConns int) (*pgxpool.Config, error)` in `internal/postgres`: parse with `pgxpool.ParseConfig`, set `MaxConns = int32(maxConns)`, leave `MinConns` at 0; verify unit tests assert `MaxConns` maps 1→1 and 5→5 and that invalid connection strings error
- [x] 2.2 Switch `Open` to `buildPoolConfig` + `pgxpool.NewWithConfig`, threading the value from `PostgresConfig`; verify manually against the local instance (localhost:5415, user admin) that startup still pings and collects with `max_db_connections: 2` set

## 3. Scheduler worker pool (capability: metric-collection)

- [x] 3.1 Rework `internal/collect/scheduler.go`: per-metric ticker goroutines enqueue into a mutex-guarded FIFO queue with a per-metric pending set (skip duplicate enqueue); `min(max_db_connections, enabled count)` worker goroutines dequeue and run `CollectOnce`; `Run` keeps its contract (blocks until ctx cancel + drain); verify existing package tests still compile/pass after signature changes (`NewScheduler` gains a max-conns argument)
- [x] 3.2 Unit test bounded concurrency: a fake `Querier` that sleeps 50ms and tracks concurrent executions via atomic counter, ~10 metrics at 50ms intervals, max 2 workers → observed peak concurrency ≤ 2 and every metric produces stored points
- [x] 3.3 Unit test dedup under contention: one slow metric (querier sleeps 300ms) ticking every 30ms with 1 worker → executions ≤ ceil(elapsed/300ms)+1, proving ticks while pending do not enqueue duplicates
- [x] 3.4 Update existing scheduler tests (`TestSchedulerPerMetricIntervals`, `TestSchedulerDisabledMetricNeverRuns`, `TestSchedulerQueryErrorDoesNotStopCollection`) for the new signature; verify all pass

## 4. Wiring and end-to-end verification (needs running PostgreSQL)

- [x] 4.1 Wire `cfg.Postgres.MaxDBConnections` through `cmd/yama/main.go` into pool and scheduler; add a startup log line with the effective max connections; verify agent starts cleanly against localhost:5415
- [x] 4.2 Generate a test config with 100 metrics (10s interval) and `max_db_connections: 2`; run the agent 60s while polling `SELECT count(*) FROM pg_stat_activity WHERE usename = 'admin'` (excluding the poller's own connection) every second; verify the agent's connection count never exceeds 2 and all 100 metrics have stored points
- [x] 4.3 Same 100-metric run with `max_db_connections` unset; verify pg_stat_activity shows at most 1 agent connection and all metrics still collect
- [x] 4.4 Verify startup rejection: configs with `max_db_connections: 0` and `max_db_connections: -3` each exit non-zero with an error naming `postgres.max_db_connections`
- [x] 4.5 Verify no data loss across the window: for the 60s run at 10s interval, each metric has ≥ 5 stored points (allows bounded startup skew), all value-correct

## 5. Final sweep

- [x] 5.1 Run `go vet ./...`, `gofmt -l .` (empty), full `go test ./...`, and `make build`; verify clean and the binary remains statically linked (`ldd bin/yama` → "not a dynamic executable")

## 6. Cycle-driven scheduling (revised semantics)

- [x] 6.1 Rework `internal/collect/scheduler.go` to global-interval cycles: wake ticker at `GCD(effective intervals)` (1ms clamp), cycle wake X collects all due metrics (`X - last >= interval`) in config order, jobs carry shared timestamp X; verify unit test: 3 metrics with 20ms queries and `maxConns: 1` produce an initial cycle whose stored points all share one identical timestamp
- [x] 6.2 Change `Collector.CollectOnce` to accept the cycle timestamp; verify unit test asserting the stored point's timestamp equals the passed value exactly
- [x] 6.3 Unit test shorter-than-global interval: global 500ms, one metric overridden to 100ms, ~600ms run → the overridden metric collects ≥ 5 times while the global metric collects ≤ 2 times (neither skipped nor delayed)
- [x] 6.4 Unit test longer-than-global interval: global 50ms, one metric overridden to 200ms, ~460ms run → the long metric collects 2–4 times total (not on every 50ms wake)
- [x] 6.5 Update and re-verify existing scheduler tests (per-metric intervals, disabled never runs, query error continues, bounded concurrency, dedup under contention) against the cycle model; verify all pass
- [x] 6.6 Document cycle semantics and shared timestamps in README; verify live against localhost:5415: 3 metrics, default `max_db_connections`, API shows all three metrics sharing identical cycle timestamps, sequential within each cycle

## 7. Next-due timer (replaces GCD wake grid)

- [x] 7.1 Replace the GCD-grid ticker in `internal/collect/scheduler.go` with a next-due timer loop: after each cycle, sleep until `min(last[name] + effective interval)`; remove `wakeInterval`/`gcdDuration`; verify package builds and existing tests pass
- [x] 7.2 Add regression test for exact cadence: a 200ms-interval metric over ~1050ms collects 5-6 times with every consecutive gap <= 1.3x the interval (a full grid-step slip would show a ~2x gap and fail); verify it passes
- [x] 7.3 Update artifacts (spec delta, design, proposal) and README from GCD wording to next-due timer; verify `openspec validate --strict`
- [x] 7.4 Live-verify cadence against localhost:5415 with 5s global + one 7s metric: consecutive `sleep_query` collection timestamps differ by ~7.00s each (no 8s gaps), and 5s metrics hold ~5.00s gaps; verify via API timestamps and logs
