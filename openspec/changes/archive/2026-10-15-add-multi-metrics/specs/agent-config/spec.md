# agent-config delta — add-multi-metrics

## MODIFIED Requirements

### Requirement: Metrics configuration file
The system SHALL load a separate metrics configuration file in YAML format defining the metrics to collect. Each metric definition SHALL include: a unique name, the SQL query to execute, and the metric type (`counter`, `gauge`, or `multi`). Each metric MAY declare an enabled flag (default enabled), a per-metric collection interval, and a per-metric query timeout (duration string such as `30s` or integer seconds). Unknown fields in the metrics configuration SHALL be rejected.

A metric with type `multi` SHALL additionally declare a `columns` map from column name to column type, where every column type SHALL be `counter` or `gauge`. The `columns` map SHALL be non-empty. The `columns` field SHALL be rejected on metrics of type `counter` or `gauge`. The system SHALL reject any configuration in which a declared metric name collides with another declared metric name or with a sub-series name (`<parent>.<column>`) derivable from any declared multi metric.

#### Scenario: Metric with all fields
- **WHEN** the metrics config defines a metric with name, query, type `gauge`, a custom interval, and a custom query timeout
- **THEN** that metric is registered for collection at its custom interval with its custom query timeout

#### Scenario: Unknown metric type rejected
- **WHEN** a metric definition uses a type other than `counter`, `gauge`, or `multi`
- **THEN** the agent exits with a non-zero status and an error naming the offending metric

#### Scenario: Duplicate metric names rejected
- **WHEN** two metric definitions share the same name
- **THEN** the agent exits with a non-zero status and an error naming the duplicate

#### Scenario: Unknown metric field rejected
- **WHEN** a metric definition contains a field other than name, query, type, columns, enabled, interval, or query timeout
- **THEN** the agent exits with a non-zero status and a parse error identifying the unknown field

#### Scenario: Valid multi metric accepted
- **WHEN** the metrics config defines a metric with type `multi`, a query, and a `columns` map declaring at least one column with type `counter` or `gauge`
- **THEN** the metric is registered for collection with its declared columns

#### Scenario: Multi metric without columns rejected
- **WHEN** a metric with type `multi` omits the `columns` field or declares an empty `columns` map
- **THEN** the agent exits with a non-zero status and an error naming the metric and the missing `columns` field

#### Scenario: Columns on non-multi metric rejected
- **WHEN** a metric with type `counter` or `gauge` declares a `columns` field
- **THEN** the agent exits with a non-zero status and an error naming the metric and stating that `columns` is only valid for type `multi`

#### Scenario: Unknown column type rejected
- **WHEN** a multi metric's `columns` map declares a column type other than `counter` or `gauge`
- **THEN** the agent exits with a non-zero status and an error naming the metric and the offending column

#### Scenario: Sub-series name collision rejected
- **WHEN** a multi metric named `db_stats` declares a column `commits` and another metric (of any type) is named `db_stats.commits`
- **THEN** the agent exits with a non-zero status and an error naming the colliding names
