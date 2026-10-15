# Multi-Metric Type Implementation Plan

> **For agentic workers:** Use this plan task-by-task; steps use checkbox
> (`- [ ]`) syntax for tracking. It expands the change's tasks.md into
> micro-steps with exact signatures and tests.

**Goal:** Add a `multi` metric type (one query, one row, N numeric columns, per-column types) whose results are stored as ordinary `parent.column` series, and render graphs referencing multiple series as one panel with multiple lines.

**Architecture:** The agent explodes a multi-metric row into N ordinary `store.Point`s inside `Collector.CollectOnce`; storage, API, and scheduler stay untouched. The frontend stays name-driven: `metric:` accepts a scalar or list, `fetchPanel` fetches each series concurrently, and `chart.Render` draws one colored line per series on a shared absolute y-axis.

**Tech Stack:** Go 1.27, pgx/v5, yaml.v3, embedded SVG/htmx dashboard. No new dependencies.

**Spec:** `openspec/changes/add-multi-metrics/` (proposal.md, design.md, specs/, tasks.md).

## Global Constraints

- No new third-party dependencies; CGO_ENABLED=0 static binaries.
- Strict YAML (KnownFields), fail-fast startup validation naming file + field.
- Errors wrap with `%w` and carry metric/graph context; slog key-value logging.
- Table-driven tests in the same package; verify per task with `go build`/`go test` of the affected package; final `make lint` + `make test`.
- No commits unless the user explicitly asks.
- Invariants preserved: cycle-shared timestamps, max_db_connections ceiling, effective query_timeout <= effective interval, read-only API, embedded assets.

## Review Focus

- Multi query returning columns in a different ORDER than declared — must succeed (match by name, not position).
- Column names containing dots (multi `a` column `b.c` vs multi `a.b` column `c`) — sub-series name collisions must be rejected at startup.
- A `multi` metric whose query returns zero rows or two rows — failed run, no partial writes.
- graphs.yaml `metric: ""` or `metric: [""]` — rejected, not silently accepted as a series.
- One series of several failing to fetch (agent 500 on one name) — whole panel shows error state, never a half-rendered chart.

---

### Task 1: Agent config — `multi` type and `columns` validation

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `MetricTypeMulti MetricType = "multi"`; `Metric.Columns map[string]MetricType` (yaml `columns`).

- [ ] **Step 1: Write failing tests** — table-driven `TestLoadMetricsMultiValidation` covering: valid multi accepted (columns non-empty, counter+gauge mix); multi without columns / empty `columns: {}` rejected (`requires a non-empty columns map`); `columns` on gauge rejected (`columns is only valid for type multi`); unknown column type rejected (names metric + column). Plus `TestLoadMetricsSubSeriesCollisions`: metric `db_stats.commits` vs multi `db_stats{commits}` rejected (multi declared before AND after the colliding metric); multi `a{b.c}` vs multi `a.b{c}` rejected; distinct parents same column name accepted.
- [ ] **Step 2: Run** `go test ./internal/config/` — expect FAIL (new cases).
- [ ] **Step 3: Implement** — add `MetricTypeMulti` constant and `Columns` field; in `LoadMetrics`'s per-metric loop add the type switch (multi: non-empty columns + per-column counter/gauge check with error naming metric and column; counter/gauge: reject non-nil `Columns`); after the loop, register each multi's `<name>.<column>` in the existing `seen` map, returning an error naming the colliding sub-series name on duplicate.
- [ ] **Step 4: Run** `go test ./internal/config/` — expect PASS (all existing tests too).

### Task 2: Postgres `QueryRow`

**Files:**
- Modify: `internal/postgres/postgres.go`
- Test: `internal/postgres/postgres_test.go`

**Interfaces:**
- Produces: `func (p *Pool) QueryRow(ctx context.Context, query string) (map[string]float64, error)` — exactly one row, one or more columns, all numeric, keyed by column name; errors name the violation.

- [ ] **Step 1: Write failing tests** — table-driven tests for a pure helper `buildRowMap(names []string, raws []any) (map[string]float64, error)`: int32/int64/float32/float64/pgtype.Numeric/[]byte-numeric map by name; []byte non-numeric and string values error naming the column; mismatched names/raws lengths error. (Live-DB row-count enforcement stays thin and untested, consistent with `QueryValue`.)
- [ ] **Step 2: Run** `go test ./internal/postgres/` — expect FAIL.
- [ ] **Step 3: Implement** `QueryRow`: query, require `len(fields) >= 1`, scan rows into `[]any`, convert via `buildRowMap` (wrapping `toFloat64` errors with the column name), enforce exactly one row (error `query must return exactly 1 row, got %d`), return the map.
- [ ] **Step 4: Run** `go test ./internal/postgres/` — expect PASS.

### Task 3: Collector — multi path + scheduler confirmation

**Files:**
- Modify: `internal/collect/collector.go`
- Test: `internal/collect/collect_test.go`

**Interfaces:**
- Consumes: `config.MetricTypeMulti`, `config.Metric.Columns`, `Pool.QueryRow` (Task 2).
- Produces: `Querier` interface extended with `QueryRow(ctx, query) (map[string]float64, error)`.

- [ ] **Step 1: Write failing tests** — extend `fakeQuerier` with `rowResult map[string]float64`, `rowErr error`, `rowCalls int` (implementing `QueryRow`). New `TestCollectOnceMulti`: success writes one point per column with names `parent.col`, declared types, values, and the exact passed timestamp shared across all; extra returned column fails with an error naming both sorted column sets and writes no points; missing declared column fails likewise; `rowErr` propagates with the metric name. New `TestSchedulerMultiOneQueryPerCycle`: a multi metric with 2 columns on a 60ms interval run ~300ms produces >= 3 `rowCalls`, 0 `QueryValue` calls, and each sub-series has equal point counts with identical timestamps per cycle.
- [ ] **Step 2: Run** `go test ./internal/collect/` — expect FAIL (build error until interface extended).
- [ ] **Step 3: Implement** — extend `Querier` with `QueryRow`; in `CollectOnce`, dispatch `m.Type == config.MetricTypeMulti` to a helper that queries, compares the returned column set to `m.Columns` by name in both directions (sorted lists in the mismatch error), then writes points in sorted column order named `m.Name + "." + col` with the column's declared type and `ts.UTC()`. Scheduler needs no change (one job per multi metric).
- [ ] **Step 4: Run** `go test ./internal/collect/ ./cmd/...` — expect PASS.

### Task 4: Web config — scalar-or-list `metric`

**Files:**
- Modify: `internal/web/config/config.go`
- Test: `internal/web/config/config_test.go`

**Interfaces:**
- Produces: `type MetricList []string` with `UnmarshalYAML` (scalar `!!str` → one entry; sequence of `!!str` → entries; anything else → error `invalid metric value: must be a string or a list of strings`); `Graph.Metric` becomes `MetricList`. Consumers get `[]string` via the slice directly.

- [ ] **Step 1: Write failing tests** — extend `TestLoadGraphsRejections`/new table: scalar accepted; list of 2 accepted (order preserved); single-entry list accepted; empty list rejected (`is missing a metric`); duplicate entries rejected (names graph + duplicate); `metric: {a: b}` rejected; `metric: [1]` rejected; `metric: ""` / `[""]` rejected (empty metric name); unknown fields still rejected. Update `TestLoadGraphsAllFields` for the new `MetricList` type.
- [ ] **Step 2: Run** `go test ./internal/web/config/` — expect FAIL.
- [ ] **Step 3: Implement** `MetricList` unmarshaler + `LoadGraphs` validation: `len(g.Metric) == 0` → missing-metric error; per-entry empty-string and duplicate-series checks naming the graph. Keep KnownFields strictness.
- [ ] **Step 4: Run** `go test ./internal/web/config/` — expect PASS.

### Task 5: Chart — multi-series rendering

**Files:**
- Modify: `internal/web/chart/chart.go`
- Test: `internal/web/chart/chart_test.go`

**Interfaces:**
- Produces: `type Series struct { Name string; Points []agent.Point }`; `Clip(series []Series, opts Options) []Series`; `Render(series []Series, opts Options) string` (replaces the single-series signatures; server and tests are the only callers).
- Consumes: nothing new.

- [ ] **Step 1: Write failing tests** — port existing single-series tests to `[]Series{{Points: ...}}` (legend, axes, scaling, tooltip, smoothing assertions unchanged — single-series output must keep the min/max/current legend and no inline styles). New `TestRenderMultiSeries`: two series produce two `<path class="line"` elements with distinct `style="stroke:` colors; dots carry matching `style="fill:`; y-domain spans both series (axis labels reflect the union); legend contains `name1: <cur1> · name2: <cur2>`; a series with no in-window points draws no path but appears in the legend as `name: —`; three-series palette cycles (series 3 reuses color 0); `Render` of all-empty series returns "".
- [ ] **Step 2: Run** `go test ./internal/web/chart/` — expect FAIL.
- [ ] **Step 3: Implement** — `Series` type; `Clip` filters per series preserving order; `Render` computes the y-domain across all series' clipped points, renders shared axes/grid/x-labels as today, then one path+dot set per series. Color mode: single series renders exactly as before (no inline styles); multiple series use the 6-color palette `{"#d97d45", "#3f72af", "#5aa469", "#8e6bbf", "#c44e52", "#4c9a9f"}` (index 0 = existing CSS accent) applied via inline `style="stroke:...` / `style="fill:..."` (inline style outranks the stylesheet). Legend: single series keeps `min X · max Y · current Z`; multi renders `name: value` joined by ` · `, empty series as `name: —`.
- [ ] **Step 4: Run** `go test ./internal/web/chart/` — expect PASS.

### Task 6: Server — per-series fetch and aggregation

**Files:**
- Modify: `internal/web/server/server.go`
- Test: `internal/web/server/server_test.go`

**Interfaces:**
- Consumes: `config.Graph.Metric` as `MetricList` (Task 4), `chart.Series/Clip/Render` (Task 5).
- Produces: unchanged HTTP surface (`/`, `/panels/{name}`, `/banner`, `/static/`).

- [ ] **Step 1: Write failing tests** — new/extended tests with the stub agent: a graph with `MetricList{"a", "b"}` renders ONE panel whose fragment contains two `class="line"` paths; agent handler serving data for `a` and `[]` for `b` still renders the chart with both legend names; agent returning 500 on one series name yields the error state (`Data unavailable`) and the down banner; existing single-scalar tests keep passing.
- [ ] **Step 2: Run** `go test ./internal/web/server/` — expect FAIL (build error: `g.Metric` type changed).
- [ ] **Step 3: Implement** — `fetchPanel` fetches all series of `g.Metric` concurrently (per-series goroutine, preallocated result slice preserving order), any fetch error → error state; otherwise clip, chart state when at least one series has points (else no-data), render `chart.Render`. Templates unchanged (panel view keeps `Graph`/`State`/`Chart`).
- [ ] **Step 4: Run** `go test ./internal/web/...` — expect PASS.

### Task 7: Docs and examples

**Files:**
- Modify: `README.md`, `configs/metrics.yaml.example`, `configs/graphs.yaml.example`

- [ ] **Step 1: Update `configs/metrics.yaml.example`** — add a commented multi example (`type: multi`, `columns: {commits: counter, ...}`) noting sub-series naming `parent.column` and the runtime exact-match rule.
- [ ] **Step 2: Update `configs/graphs.yaml.example`** — show the list form (`metric: [db_stats.commits, db_stats.reads]`) alongside the scalar form.
- [ ] **Step 3: Update `README.md`** — "Adding Metrics": the third type, per-column types, `<parent>.<column>` naming, exact-match failure semantics (logged, run skipped); dashboard section: `metric` accepts a scalar or list, mixed freely, one panel with one line per series. Keep `TestExampleConfigLoads` passing (examples must parse).

### Task 8: Final verification

- [ ] **Step 1:** `make lint` — expect clean (go vet + gofmt).
- [ ] **Step 2:** `make test` — expect all packages pass.
- [ ] **Step 3:** `make build` — expect both binaries build.
