## Why

Metrics that run long SQL statements are killed by the global `collector.query_timeout` (default 3s) and never collect, while "heavy" metrics are allowed to run too long under the same global value, causing contention and locks on system resources. The global timeout is a single compromise that cannot serve both kinds of metrics at once; operators need per-metric control, exactly like the existing per-metric `interval` override.

## What Changes

- Add an optional per-metric `query_timeout` field to metric definitions (`internal/config.Metric`), parsed with the existing `Duration` type (string like `"30s"` or integer seconds) under strict known-fields YAML parsing.
- Add `Metric.EffectiveQueryTimeout(global)` mirroring `Metric.EffectiveInterval(global)`: the metric's own `query_timeout` when set, the global `collector.query_timeout` otherwise. A metric with no override behaves exactly as before.
- Enforce a strict invariant at config load: every metric's **effective** `query_timeout` MUST NOT exceed its **effective** `interval`. Violations are rejected with a non-zero exit and an error naming the metric and both conflicting values (including their source: metric override vs global). This covers three cases:
  - explicit per-metric `query_timeout` > effective interval;
  - global `query_timeout` > a metric's custom (per-metric) interval;
  - global `query_timeout` > global `interval` (the global pair itself).
  - **BREAKING**: configs that previously loaded with an effective timeout greater than the effective interval (including a global pair like `query_timeout: 15s` / `interval: 10s`) are now rejected at startup. `timeout == interval` is allowed; only strictly greater is rejected. Defaults (3s/10s) pass unchanged.
- Wire the collector to use the per-metric effective timeout instead of only the global value: each query execution is bounded by its metric's effective `query_timeout`.
- Keep the pool-level startup connectivity ping (`postgres.Open`) bounded by the **global** `collector.query_timeout`: it is a connection check, not a metric query, and has no per-metric equivalent.
- Document `query_timeout` in `configs/metrics.yaml.example` and `configs/config.yaml.example`.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `agent-config`: metric definitions gain an optional `query_timeout` field with global fallback (`EffectiveQueryTimeout`), and config loading gains a strict cross-file validation rule — effective `query_timeout` must never exceed effective `interval` for the global pair or any metric; violations fail startup with an error naming the metric and the conflicting values.
- `metric-collection`: query execution is bounded per metric — each metric's query runs under its effective `query_timeout` (own value, else global), so one metric may run longer or shorter than the global timeout without affecting others.

## Impact

- `internal/config/config.go`: new `Metric.QueryTimeout` field, `EffectiveQueryTimeout()` method, validation of the global pair in `LoadAgent`, and a cross-config validation entry point applied after both files load in `cmd/yama/main.go`.
- `internal/collect/collector.go`: `Collector` applies the metric's effective timeout in `CollectOnce` (constructor semantics change from "fixed timeout" to "global fallback timeout").
- `cmd/yama/main.go`: pass the global timeout as collector fallback; pool ping stays on the global timeout (unchanged).
- Tests: `internal/config/config_test.go` (fallback, both override directions, three rejection cases, example-config load), `internal/collect/collect_test.go` (per-metric timeout applied in `CollectOnce`).
- `configs/config.yaml.example`, `configs/metrics.yaml.example`: document the new field.
- No dependency, API, or storage changes.
