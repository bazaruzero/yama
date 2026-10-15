# metric-collection delta — add-multi-metrics

## MODIFIED Requirements

### Requirement: Data point production
Each successful query execution SHALL produce data point(s) containing: the metric name, the metric type, the numeric value(s) returned by the query, and the collection timestamp in UTC.

A metric of type `counter` or `gauge` MUST return exactly one row with one numeric column to produce a data point, producing a single data point carrying the metric's name and type.

A metric of type `multi` MUST return exactly one row with one or more numeric columns. Each column SHALL produce one data point named `<metric-name>.<column-name>` carrying the column's declared type (from the metric's `columns` map), the column's value, and the collection timestamp in UTC. All data points produced by one multi-metric execution SHALL carry the same collection timestamp. The set of column names returned by the query MUST exactly match the set of column names declared in the metric's `columns` map: a query returning a column that is not declared, or omitting a declared column, SHALL be treated as a failed collection for that metric — logged with the metric name and the mismatch, no data points produced for that run, and future scheduled executions unaffected. A multi-metric execution is one scheduled collection job: one query per due cycle, bounded by the metric's effective query timeout, occupying one connection slot.

#### Scenario: Successful collection
- **WHEN** a metric query executes successfully and returns one numeric value
- **THEN** a data point with the metric name, type, value, and current UTC timestamp is passed to storage

#### Scenario: Query failure does not stop collection
- **WHEN** a metric query fails (SQL error, timeout, or lost connection)
- **THEN** the failure is logged with the metric name and reason, no data point is produced for that run, and future scheduled executions of that and other metrics continue

#### Scenario: Unexpected result shape
- **WHEN** a metric query returns zero rows, multiple rows, or a non-numeric value
- **THEN** the run is treated as a query failure for that metric

#### Scenario: Successful multi-metric collection
- **WHEN** a multi metric's query returns one row with columns exactly matching its declared `columns` map
- **THEN** one data point per column is passed to storage, each named `<metric-name>.<column-name>`, typed per the column's declaration, and all carrying the identical collection timestamp

#### Scenario: Multi-metric with extra returned column fails the run
- **WHEN** a multi metric's query returns a column not present in the declared `columns` map
- **THEN** the run is treated as a failed collection: the mismatch is logged with the metric name and both column sets, no data points are produced for that run, and later runs of that and other metrics continue

#### Scenario: Multi-metric with missing declared column fails the run
- **WHEN** a multi metric's query omits a column declared in its `columns` map
- **THEN** the run is treated as a failed collection: the mismatch is logged with the metric name and both column sets, no data points are produced for that run, and later runs of that and other metrics continue

#### Scenario: Multi-metric with multiple rows fails the run
- **WHEN** a multi metric's query returns more than one row
- **THEN** the run is treated as a failed collection for that metric

#### Scenario: Multi-metric is one scheduled job
- **WHEN** a multi metric declaring three columns is due in a cycle
- **THEN** exactly one query is executed for it in that cycle, and the resulting three data points carry the cycle's shared UTC timestamp
