## Context

The agent today applies one global `collector.query_timeout` (default 3s) to every metric query. Two consumption points exist in `cmd/yama/main.go`: `postgres.Open` (startup connectivity ping, main.go:58) and `collect.NewCollector` (per-run query deadline, main.go:71). `Collector.CollectOnce` (internal/collect/collector.go) applies that single timeout via `context.WithTimeout`. Per-metric overrides already have an established pattern: `Metric.Interval` + `Metric.EffectiveInterval(global)` (internal/config/config.go:134–149), with the scheduler holding the global interval as fallback. Both YAML files are parsed with strict known-fields decoding, and durations unmarshal through the shared `Duration` type (string or integer seconds). `LoadAgent` and `LoadMetrics` are independent: the metrics loader has no access to agent-config values, and defaults are applied inside `LoadAgent`.

## Goals / Non-Goals

**Goals:**

- Per-metric `query_timeout` with global fallback, indistinguishable from current behavior when unset.
- Strict load-time invariant: effective `query_timeout` ≤ effective `interval` for the global pair and every metric, with actionable errors naming the metric and both values with their sources.
- Collector executes each metric under that metric's effective timeout, in both directions (longer and shorter than global).
- A conscious, documented decision for the pool-ping consumption point.

**Non-Goals:**

- No change to scheduling, concurrency caps, storage, or API.
- No per-metric settings beyond `query_timeout` (no per-metric max connections, etc.).
- No validation of per-metric `interval` values (pre-existing behavior; negative interval already falls back to global and stays that way).
- No dynamic reload of configuration.

## Decisions

### D1: Field shape — `QueryTimeout Duration` (zero = unset), mirroring `Interval`

`Metric` gains `QueryTimeout Duration \`yaml:"query_timeout"\`` and `EffectiveQueryTimeout(global time.Duration)` returning the metric's value when `> 0`, else the global — a direct mirror of `EffectiveInterval`. Alternative: a `*Duration` pointer (like `MaxDBConnections *int`) to distinguish "unset" from "0". Rejected: within the metric struct the `Interval` precedent is the closer pattern, zero-as-unset keeps YAML merging semantics simple, and an explicit `0` meaning "unset" matches the existing interval convention. A negative explicit `query_timeout` is rejected at load (see D3) rather than silently falling back — consistent with the fail-fast, name-the-field philosophy used for `max_db_connections`, and an explicit value the system ignores is exactly the kind of silent misconfiguration this change exists to remove.

### D2: Validation split — global pair inside `LoadAgent`, cross-file check as a separate config-package function

- `LoadAgent` validates the global pair after defaults are applied: `QueryTimeout > Interval` → error naming `collector.query_timeout` and `collector.interval` with both values. It has everything it needs locally, and this also guards agents started with a metrics file that has no overrides.
- A new exported `config.ValidateQueryTimeouts(agent *AgentConfig, mcfg *MetricsConfig) error` checks each metric's `EffectiveQueryTimeout` vs `EffectiveInterval` (with the agent's globals as fallbacks) and returns an error naming the metric and both effective values tagged with their source (metric override vs global).
- `cmd/yama/main.go` calls it immediately after both files load, so rejection happens at startup with a non-zero exit — "config load" in observable terms.

Alternatives: (a) change `LoadMetrics` to accept the agent config — couples the two loaders, breaks the existing API and tests for no behavioral gain; (b) validate inline in `main.go` — puts error-formatting policy outside the config package where it isn't unit-testable alongside the other validation tests. Both rejected.

Error message shape (metric case):

```
metrics config: metric "bloat_ratio": effective query_timeout 5s (global collector.query_timeout) exceeds effective interval 2s (metric interval)
```

Global case (from `LoadAgent`):

```
agent config "config.yaml": collector.query_timeout 15s exceeds collector.interval 10s
```

### D3: Negative explicit `query_timeout` rejected in `LoadMetrics`

`LoadMetrics` gains one check: `QueryTimeout.Duration < 0` → error naming the metric and field. Without it, a typo like `-30s` would silently fall back to global. Zero remains "unset" (consistent with interval integer-seconds convention). This is an assumption recorded here, slightly ahead of the stated success criteria, in the spirit of strict validation.

### D4: Collector applies the effective timeout per run

`Collector` keeps its constructor signature; the `timeout` field's meaning becomes "global fallback query timeout". `CollectOnce` computes `m.EffectiveQueryTimeout(c.timeout)` before building the per-run context. Alternative: resolve the effective timeout in the scheduler and pass it alongside the job. Rejected: the scheduler already resolves interval where it needs it (scheduling decisions), while deadline policy belongs to the executor; this also mirrors `EffectiveInterval` consumption without extra plumbing through `collectJob`.

### D5: Pool ping stays on the global timeout

`postgres.Open`'s ping remains bounded by the global `collector.query_timeout`. Rationale: the ping is a connectivity sanity check, not a metric query — it has no per-metric identity, and mixing per-metric timeouts into pool setup would raise the question "which metric's?" with no good answer. The global pair validation in D2 additionally guarantees the global timeout itself is now always ≤ the global interval. Alternative: use `max(effective timeouts)` for the ping — rejected as surprising and unbounded-by-invariant. This is the conscious decision for the second consumption point called out in the proposal.

### D6: Config examples updated in place

`configs/metrics.yaml.example` gains a commented `# query_timeout: 30s   # optional per-metric override of collector.query_timeout` line; `configs/config.yaml.example`'s `query_timeout` line gains a matching per-metric-override comment (like `interval` already has). Both examples must remain loadable — `TestExampleConfigLoads` parses `config.yaml.example`, and the examples stay within the invariant.

## Risks / Trade-offs

- [Strict validation rejects previously-accepted configs at upgrade] → Accepted trade-off (marked BREAKING). Mitigation: the error names the exact metric and both conflicting values with sources, so the operator fix is mechanical; defaults 3s/10s pass unchanged.
- [Sleep-based collector tests can be flaky on loaded CI] → Use generous margins (e.g. query sleeps well beyond the short deadline, well within the raised one), matching the timing style already present in `collect_test.go`.
- [Long per-metric timeouts hold pool connections longer] → Bounded by the invariant (timeout ≤ interval) plus existing scheduler dedup (`tryEnqueue`) that prevents a slow metric's jobs from piling up; `max_db_connections` still caps concurrency.
- [Equality edge (`timeout == interval`) misrejected by off-by-one] → Spec'd as accepted; covered by an explicit unit test.

## Migration Plan

Ship as a single release. Operators upgrading whose configs now fail startup get the named-metric/named-values error and adjust either `query_timeout` or `interval` for the offending metric (or the global pair). No data or config-format migration; rollback is reverting to the previous binary (old behavior resumes since the new field is simply absent from old configs' metrics).

## Open Questions

None — pool-ping semantics (D5) and negative-value handling (D3) were the two judgment calls and are decided above.
