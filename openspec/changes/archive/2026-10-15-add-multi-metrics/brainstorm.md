# Brainstorm — add-multi-metrics

<!--
Raw capture of the brainstorming process (superpowers:brainstorming,
vendored at .opencode/node_modules/superpowers/skills/brainstorming/),
run as a seeded session: the exploration conversation that preceded this
change already covered most of the skill's ground; this file records the
decision log as it emerged, plus the skill's playback/validation pass.
-->

## Background

YAMA has two metric types — counter and gauge — both enforcing a strict
query contract: one query, one row, one numeric column. Each metric costs
one query, so related values (e.g. several counters from the same
`pg_stat_*` view) each pay the full query price and cannot share a scan.
The human partner wants a third metric type whose single SQL query
returns several numeric columns, e.g.

    select 1 as metric1, 2 as metric2, 3 as metric3;

and a dashboard that renders such a multi-metric as ONE graph with
multiple lines, not one panel per series.

## Decision chain

**Q1: Is this feasible without breaking the storage/API layers?**
Yes. A multi-metric's single row can be exploded by the collector into
N ordinary points; the Store interface, on-disk format, and read-only
API are untouched. The scheduler treats a multi-metric as one job, so
cycle-shared timestamps, the max_db_connections ceiling, and the
timeout<=interval invariant all carry over.

**Q2: What are the sub-metric names?**
Decision: prefixed — `parent.column` (dot separator). Avoids collisions
with other configured metrics. The duplicate-name check across all
stored series applies at runtime.

**Q3: How does the frontend discover sub-metrics?**
Three options were compared:
(a) graphs.yaml lists series names explicitly;
(b) agent exposes a metadata/listing endpoint, frontend auto-expands;
(c) parent-name fetch returns all sub-series grouped.
Decision: (a) — the most YAMA-ish: explicit, validated at startup,
zero API change. "One graph = a list of series names" falls out for
free. (b)/(c) reintroduce magic and grow the API; rejected.

**Q4: Chart rendering for multiple series?**
Fixed color palette cycled per series; per-series legend (name +
current value); shared absolute y-axis; the single-series
min/max/current legend line is dropped for multi-series, values remain
in point tooltips. Scale normalization (mixed magnitudes on one axis)
explicitly out of scope for this change.

**Q5: Is "multi" a type or a shape? Should columns be typed?**
"Multi" is a shape; counter/gauge are semantics. A single query can mix
cumulative and point values.
Decision: per-column types are required — `columns:` maps column name
to counter/gauge. No default; every column must be declared.

**Q6: Row contract?**
Exactly one row, N numeric columns. Multiple rows would require
naming/tagging rows — a much bigger feature, rejected.

**Q7: Runtime strictness?**
The declared `columns:` map is the schema; the actual column set is
only known at query time. Every returned column must be declared and
every declared column must be returned; any mismatch fails that
collection run (logged, skipped — same treatment as a query error),
it does not crash the agent. Startup cannot validate this without
executing the query.

**Q8: graphs.yaml shape?**
Single `metric` field that accepts a scalar string OR a list of
strings (not separate `metric`/`metrics` fields). Normalized
internally to a list. At least one entry, no duplicates within a
graph. A graph may mix any stored series freely — sub-metrics, plain
metrics, or both — because the frontend is deliberately name-driven
and dumb: it knows nothing about types, parents, columns, or "multi".
Unknown names keep today's behavior: no-data panel (the agent is
remote and may be down at web startup, so no cross-validation).

**Q9: What failure would make the change a failure even if it works?**
Human partner: no specific anti-goal; proceed.

## Design trade-offs accepted

- Explicit graphs.yaml listing vs auto-discovery: explicitness chosen;
  cost is hand-written sub-metric names in graphs.yaml.
- Runtime schema enforcement vs lenient column handling: strict
  exact-match chosen; a query schema drift becomes a visible (logged)
  failure rather than silent missing series.
- Shared absolute y-axis vs normalization: normalization deferred
  (out of scope); mixing magnitudes is documented as the user's
  responsibility.

## Frontend architecture stance (explicit)

All multi-metric intelligence (explosion, prefixing, per-column types)
lives in the agent and dissolves into ordinary series names before the
frontend ever sees them. The frontend delta is exactly two things:
`metric:` accepting scalar-or-list, and the chart rendering N series
instead of 1.

## Size

A project with a written design (OpenSpec change: proposal, design,
delta specs, tasks).
