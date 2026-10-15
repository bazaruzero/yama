## Why

Today every YAMA metric costs one query, and related values (e.g. several counters from the same `pg_stat_*` view) each pay the full query price without being able to share a scan. Operators writing metrics.yaml want one query to feed several series at once — cheaper collection, atomic related values, and one dashboard panel with multiple lines instead of N near-identical panels.

## What Changes

**Multi metric type (agent)**
- From: metric type is `counter` or `gauge`; the query must return one row, one numeric column.
- To: a third type `multi` whose query returns exactly one row with N numeric columns; a `columns:` map declares each column's type (`counter` or `gauge`).
- Reason: one query, many series, with per-column semantics (a single query can mix cumulative and point values).
- Impact: non-breaking — existing counter/gauge metrics, storage format, and the read-only agent API are unchanged.

**Sub-metric explosion (collector)**
- From: one metric definition produces one series under its own name.
- To: a `multi` metric's row is exploded into N ordinary data points named `parent.column`, sharing the cycle timestamp; a multi metric is one scheduled job (one query per cycle), so the `max_db_connections` ceiling and timeout<=interval invariants carry over. The returned column set must exactly match the declared `columns:`; any mismatch fails that run only (logged, skipped — same treatment as a query error).
- Impact: non-breaking; no new series collisions (duplicate-name check applies).

**Graphs config accepts multiple series (frontend)**
- From: a graph references exactly one metric via `metric: <name>`.
- To: `metric:` accepts a scalar string or a list of strings (at least one, no duplicates); any stored series may be mixed freely in one graph — sub-metrics, plain metrics, or both. Unknown names keep the no-data behavior (no cross-validation against the agent).
- Impact: non-breaking — scalar form behaves exactly as before.

**Multi-series chart rendering (frontend)**
- From: a panel renders one line with a min/max/current legend.
- To: a panel with multiple series renders one line per series from a fixed cycled color palette, with a per-series legend (name + current value) replacing the min/max/current line; values remain in point tooltips; shared absolute y-axis.
- Impact: non-breaking visual change for multi-series graphs only; single-series graphs keep the existing legend.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `agent-config`: metrics config gains the `multi` metric type with the required `columns:` map (per-column counter/gauge types); validation rules for columns (non-empty, known types, no duplicate sub-metric names).
- `metric-collection`: data point production for `multi` metrics — one row, N numeric columns, exact match with declared columns, explosion into `parent.column` points sharing the cycle timestamp; mismatch handled as a failed run.
- `ui-config`: graphs config `metric` field accepts a scalar or a list; validation (at least one entry, no duplicates, scalar/list both valid).
- `web-dashboard`: panel rendering with multiple series — one panel per graph regardless of series count, per-series colors and legend, shared absolute y-axis.

## Impact

- Code: `internal/config` (multi type + columns validation), `internal/postgres` (row/columns query path), `internal/collect` (explosion), `internal/web/config` (scalar-or-list metric), `internal/web/chart` (multi-series rendering), `internal/web/server` (fetch N series per panel).
- Configs: additive fields only (`type: multi`, `columns:`, `metric:` list form) — no breaking YAML change; strict KnownFields parsing preserved.
- API/storage: unchanged (`GET /api/v1/metrics/{name}/data` serves `parent.column` series like any other).
- Dependencies: none added.
- Docs/examples: README and `configs/*.example` updated for the new type and list-form `metric`.

## Non-goals

- Multi-row query results (tagging/naming rows).
- Per-series or normalized y-axes; scale modes (shared absolute axis stays).
- Agent metadata/listing endpoint or frontend auto-discovery of sub-metrics.
- Changes to the agent API shape or on-disk storage format.
