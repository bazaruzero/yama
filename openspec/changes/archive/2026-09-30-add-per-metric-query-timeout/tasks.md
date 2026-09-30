## 1. Config: per-metric query_timeout field

- [x] 1.1 Add `QueryTimeout Duration` (`yaml:"query_timeout"`) to `config.Metric` and `EffectiveQueryTimeout(global time.Duration) time.Duration` mirroring `EffectiveInterval` (own value when `> 0`, else global). Verify with a unit test in `internal/config/config_test.go` covering fallback (unset → global), longer override, and shorter override (mirrors `TestEffectiveInterval`).
- [x] 1.2 Verify parsing: a metrics YAML using `query_timeout: 30s` (string) and `query_timeout: 2` (integer seconds) loads into the field; a metric with an unknown field is still rejected by strict known-fields decoding (regression test).

## 2. Config: strict validation

- [x] 2.1 In `LoadMetrics`, reject an explicitly negative `query_timeout` with an error naming the metric and field. Verify with a unit test (`query_timeout: -5s` → load error naming the metric).
- [x] 2.2 In `LoadAgent`, after defaults are applied, reject `collector.query_timeout > collector.interval` with an error naming both settings and values (design D2 global-pair message). Verify with unit tests: violation rejected and error mentions both field names; equality (`15s`/`15s`) accepted; defaults (no collector section) still load.
- [x] 2.3 Add exported `config.ValidateQueryTimeouts(agent *AgentConfig, mcfg *MetricsConfig) error` checking each metric's effective query timeout vs effective interval, with source-tagged values in the error (design D2 metric message). Verify with unit tests: explicit per-metric violation rejected; global timeout (5s) vs metric custom interval (2s) rejected with metric named; effective equality accepted; config with only defaults passes; first offending metric is reported.

## 3. Collector wiring

- [x] 3.1 Change `Collector.timeout` semantics to global fallback: in `CollectOnce` use `m.EffectiveQueryTimeout(c.timeout)` for the per-run `context.WithTimeout`. Verify with tests in `internal/collect/collect_test.go` using the sleep-based `fakeQuerier` (150ms sleep): (a) raised override (500ms timeout) succeeds and stores a point; (b) tightened override (50ms timeout) fails with no point stored; (c) no override falls back to the constructor timeout (50ms constructor timeout → fails, 1s → succeeds).

## 4. Main wiring and examples

- [x] 4.1 In `cmd/yama/main.go`, call `config.ValidateQueryTimeouts(cfg, mcfg)` immediately after both configs load (returning the error → non-zero exit); leave `postgres.Open` on the global `collector.query_timeout` (design D5). Verify `make build` succeeds and the invalid-config rejection is observable by running the binary with a violating pair (manual or build-verified).
- [x] 4.2 Update `configs/metrics.yaml.example` with a commented `# query_timeout: 30s` line and `configs/config.yaml.example`'s `query_timeout` comment to mention the per-metric override. Verify `TestExampleConfigLoads` and `make test` still pass.

## 5. Final verification

- [x] 5.1 Run `make test`, `make lint`, and `make build`; all pass with no formatting issues. Confirm the full validation matrix from the spec is covered: fallback, both override directions, three rejection cases (per-metric, global+custom-interval, global pair), equality accepted, defaults accepted.
