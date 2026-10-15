## Context

YAMA (Go, static binaries, pgx + badger + yaml.v3 only) collects
PostgreSQL metrics in cycles. Today a metric is `counter` or `gauge`,
its query must return one row / one numeric column (`postgres.QueryValue`
enforces this), the collector writes one `store.Point` per run, and the
dashboard renders one panel per configured graph referencing a single
metric name. Stakeholder request: a third metric type whose single query
returns several numeric columns, collected once and rendered as one
graph with multiple lines.

Key architectural fact: `store.Point{Name, Type, Value, Timestamp}` and
the read-only API are name-keyed and type-agnostic. If the agent explodes
a multi-metric's row into ordinary points, neither storage nor API nor
the frontend's data-fetching needs to know "multi" exists.

## Goals / Non-Goals

**Goals:**
- `type: multi` metrics: one query, exactly one row, N numeric columns.
- Per-column types via a required `columns:` map (counter/gauge).
- Sub-series stored as ordinary points named `parent.column`, sharing the
  cycle timestamp; multi metric is one scheduled job.
- Graphs config `metric:` accepts scalar or list; any series mixable.
- One panel per graph regardless of series count; per-series colors and
  legend; shared absolute y-axis.
- Preserve all hard invariants (fail-fast strict config, strict YAML,
  max_db_connections ceiling, timeout<=interval, read-only API,
  cycle-shared timestamps, embedded assets).

**Non-Goals:**
- Multi-row query results.
- Y-axis normalization / scale modes / dual axes.
- Agent metadata endpoint; frontend auto-discovery of sub-metrics.
- Any change to the agent API JSON shape or on-disk storage format.

## Decisions

### D1: Explosion at the collector, not a new storage shape
- **Choice**: `Collector.CollectOnce` for a multi metric executes the
  query, validates the returned column set against the declared
  `columns:`, and writes N ordinary points named `parent.column`.
- **Rationale**: zero changes to `store.Store`, BadgerStore, and the
  API; the whole read path is untouched; sub-series become ordinary
  metrics everywhere downstream.
- **Alternatives**: storing the row as a composite point (new value
  encoding, API shape change — rejected); a parallel multi-store
  (rejected: complexity).

### D2: Querier gains a row-query method
- **Choice**: extend the `collect.Querier` interface (and the pgx
  wrapper) with a method returning the single row as
  column-name -> float64, enforcing one row / all-numeric columns.
- **Rationale**: keeps the existing `QueryValue` contract intact for
  counter/gauge; the multi path has its own strict contract
  (exactly one row; every column numeric).
- **Alternatives**: reusing QueryValue per column (N queries — defeats
  the purpose).

### D3: Runtime exact-match column validation, logged-and-skipped
- **Choice**: every returned column must be declared and every declared
  column must be returned; mismatch fails that run only, logged with
  the metric name, like a query error. No agent crash.
- **Rationale**: the actual column set is unknowable at startup;
  fail-fast applies to what config can check (types, non-empty
  columns map), runtime strictness catches schema drift visibly
  instead of silently dropping series.
- **Alternatives**: lenient subset matching (rejected: silent missing
  series); crash on mismatch (rejected: violates the
  never-crash-on-query-errors invariant).

### D4: Sub-metric naming `parent.column`
- **Choice**: dot separator; the duplicate-name check (declared metric
  names + all derived sub-series names) is enforced at startup where
  possible and at runtime for query-derived names.
- **Rationale**: collision-safe, readable, and appears naturally in the
  API path. Column names containing dots are not disambiguated
  (documented limitation, vanishingly rare).

### D5: Scalar-or-list `metric` in graphs.yaml
- **Choice**: custom unmarshaler accepting string or []string,
  normalized to []string; validation: at least one entry, no
  duplicates within a graph. No separate `metrics` field.
- **Rationale**: one field, backward compatible, list of one behaves
  exactly like today's scalar.
- **Alternatives**: a second `metrics` field (rejected: two ways to say
  the same thing, mutual-exclusion validation).

### D6: Frontend stays name-driven and dumb
- **Choice**: the dashboard knows nothing about types, parents,
  columns, or "multi". It fetches each series by name via the existing
  agent endpoint and renders one panel with N lines.
- **Rationale**: all multi-intelligence dissolves into ordinary names
  before the frontend sees them; the frontend delta is exactly two
  things (D5 + D7).
- **Alternatives**: metadata endpoint + auto-expansion (rejected:
  grows the API, magic behavior); parent-name grouped fetch (rejected:
  API shape change).

### D7: Multi-series chart rendering
- **Choice**: fixed palette (~6 colors) cycled per series; per-series
  legend showing name + current value; shared absolute y-axis;
  single-series graphs keep the existing min/max/current legend.
- **Rationale**: minimal honest rendering; no new deps (pure SVG as
  today).
- **Alternatives**: normalization (out of scope, deferred); per-series
  axes (out of scope).

### D8: Scheduler treatment of multi metrics
- **Choice**: a multi metric is one job — one dispatch per due cycle,
  one connection slot, one query, bounded by its effective
  query_timeout. All hard invariants (cycle timestamp sharing,
  dedup, connection ceiling, timeout<=interval) apply unchanged.
- **Rationale**: no scheduler changes at all; explosion happens inside
  the collector run.

## Risks / Trade-offs

- [Risk] Query schema drift (column renamed/dropped in the view) breaks
  collection silently from the user's perspective → Mitigation: every
  mismatch is logged with metric name and both column sets (declared vs
  returned); the panel shows no-data, prompting investigation.
- [Risk] Sub-series name collision with an existing declared metric
  (e.g. a metric literally named `db_stats.commits`) → Mitigation:
  startup validation rejects declared-name collisions among declared
  metrics + derived sub-series names where computable; runtime
  collisions are caught by the same duplicate-name guard.
- [Trade-off] Mixed magnitudes on a shared absolute axis can flatten
  small series → Accepted: normalization explicitly out of scope; values
  remain in tooltips; users can split into separate graphs.
- [Trade-off] graphs.yaml lists sub-metric names by hand; schema changes
  require editing graphs.yaml → Accepted: explicitness over magic (D6).
- [Trade-off] A multi metric with many columns writes N points per
  cycle → Accepted: ordinary per-series storage cost, identical to
  declaring N single metrics.

## Migration Plan

N/A — no deployment, endpoint, or on-disk format changes. Rollback is
redeploying the previous binaries; existing configs remain valid (the
new fields are additive and only used when present). Update README.md
and `configs/*.example` alongside implementation.

## Open Questions

(none — all decisions resolved during brainstorming)
