## ADDED Requirements

### Requirement: Per-metric query execution timeout
Each metric query execution SHALL be bounded by that metric's effective query timeout: the metric's own `query_timeout` when configured, otherwise the global `collector.query_timeout`. A query still running when its effective timeout elapses SHALL be cancelled and treated as a failed collection for that metric — logged with the metric name, no data point produced, future scheduled executions unaffected. Per-metric timeouts take effect in both directions: a metric MAY run longer or shorter than the global timeout. The startup connectivity check SHALL remain bounded by the global `collector.query_timeout`, independent of any per-metric value.

#### Scenario: Slow metric with raised per-metric timeout collects successfully
- **WHEN** a metric's query takes 5 seconds, the global `collector.query_timeout` is 3 seconds, and the metric sets `query_timeout: 10s`
- **THEN** its query completes and a data point is stored

#### Scenario: Heavy metric with tightened per-metric timeout is cut off
- **WHEN** a metric's query takes 5 seconds, the global `collector.query_timeout` is 3 seconds, and the metric sets `query_timeout: 1s`
- **THEN** its query is cancelled after 1 second, the failure is logged with the metric name, no data point is produced, and later runs of that and other metrics continue

#### Scenario: Per-metric timeout does not affect other metrics
- **WHEN** one metric sets `query_timeout: 30s` and other metrics omit it with the global timeout at 3 seconds
- **THEN** only the overriding metric is bounded by 30 seconds; the others remain bounded by 3 seconds

#### Scenario: Startup connectivity check uses the global timeout
- **WHEN** the agent starts with metrics that override `query_timeout` and verifies PostgreSQL connectivity
- **THEN** the connectivity check is bounded by the global `collector.query_timeout`, not by any per-metric value
