# Tasks — add-multi-metrics

## 1. Agent config: multi type and columns validation

- [ ] 1.1 Add `MetricTypeMulti` (`"multi"`) to `internal/config/config.go`; add a `Columns map[string]MetricType` field (`yaml:"columns"`) to `Metric`. Verify `make build`.
- [ ] 1.2 Extend `LoadMetrics` validation in `internal/config/config.go`: `multi` requires a non-empty `columns` map; each column type must be `counter` or `gauge`; `columns` on non-multi metrics is rejected; errors name the file, metric, and field. Verify with a table-driven test in `internal/config/config_test.go` covering: valid multi accepted, missing/empty columns rejected, columns on gauge rejected, unknown column type rejected.
- [ ] 1.3 Extend duplicate-name validation in `internal/config/config.go` to reject collisions between declared metric names and derivable sub-series names (`<parent>.<column>`) across all multi metrics. Verify with a table-driven test in `internal/config/config_test.go` covering: declared-vs-sub-series collision, sub-series-vs-sub-series collision, distinct parents with same column name accepted.

## 2. Postgres row query

- [ ] 2.1 Add `QueryRow` to `internal/postgres/postgres.go`: executes a query that MUST return exactly one row with one or more columns, all numeric; returns `map[string]float64` keyed by column name; errors name the violation (row count, non-numeric column). Verify `make build`.
- [ ] 2.2 Add a table-driven test in `internal/postgres/postgres_test.go` (or extend the existing test setup) covering: one row N numeric columns, zero rows rejected, multiple rows rejected, non-numeric column rejected.

## 3. Collector: multi-metric explosion

- [ ] 3.1 Extend `collect.Querier` in `internal/collect/collector.go` with the row-query method (or a second interface checked at dispatch); implement the multi path in `CollectOnce`: query, exact-match the returned column set against the metric's declared `columns` (both directions), write one `store.Point` per column named `<metric>.<column>` with the column's declared type and the shared cycle timestamp. Column-set mismatch returns an error naming the metric and both column sets. Verify `make build`.
- [ ] 3.2 Add a table-driven test in `internal/collect/collect_test.go` with a fake Querier and the in-memory store, covering: successful explosion (names, types, values, shared timestamp), extra returned column fails, missing declared column fails, multi-row fails, no points written on failure.

## 4. Scheduler integration

- [ ] 4.1 Confirm (and adjust if needed) `internal/collect/scheduler.go` treats a multi metric as one job with no per-column dispatch; no code change expected. Verify with a scheduler test in `internal/collect/collect_test.go` asserting one query per due cycle for a multi metric.

## 5. Frontend config: scalar-or-list metric

- [ ] 5.1 In `internal/web/config/config.go`, change `Graph.Metric` to a custom unmarshaler type accepting a scalar string or a list of strings, normalized to `[]string`; validation: at least one entry, no duplicates within a graph, other YAML types rejected; errors name the file and graph. Verify `make build`.
- [ ] 5.2 Add a table-driven test in `internal/web/config/config_test.go` covering: scalar accepted, list accepted, single-entry list accepted, empty list rejected, duplicate entries rejected, mapping rejected, unknown fields still rejected.

## 6. Chart: multi-series rendering

- [ ] 6.1 Refactor `internal/web/chart/chart.go` to render N series: fixed ~6-color palette cycled per series, one path per series, shared absolute y-axis computed across all series, per-series legend (name + current value) below the chart for multi-series, existing min/max/current legend preserved for single-series, tooltips unchanged. Verify `make build`.
- [ ] 6.2 Update `Render`/`Clip` signatures in `internal/web/chart/chart.go` as needed for named series; add a table-driven test in `internal/web/chart/chart_test.go` covering: single series keeps existing legend, multi series produces one path per series with distinct colors, y-domain spans all series, all-series-empty returns "".

## 7. Frontend server: per-series fetch

- [ ] 7.1 Update `fetchPanel` in `internal/web/server/server.go` to fetch each of the graph's series (concurrently), aggregate into the multi-series chart input, and keep panel state semantics: chart when any series has data, no-data when none does, error when the agent call fails. Update `templates/*.tmpl` if the panel view shape changes. Verify `make build`.
- [ ] 7.2 Add a table-driven test in `internal/web/server/server_test.go` with a stub agent client, covering: multi-series graph renders one panel with all series, one empty series among populated ones still renders, agent failure yields error state and banner, scalar graphs behave as before.

## 8. Docs, examples, final verification

- [ ] 8.1 Update `README.md` and `configs/metrics.yaml.example` documenting the `multi` type, the `columns` map, sub-series naming, and the runtime exact-match rule.
- [ ] 8.2 Update `configs/graphs.yaml.example` (or the web config example) demonstrating the list form of `metric`.
- [ ] 8.3 Run `make lint` and `make test`; verify all pass.
