## Purpose

Defines how the agent connects to the monitored PostgreSQL database and executes configured metric queries on independent schedules, producing typed, timestamped data points without ever crashing on query errors.

## Requirements

### Requirement: PostgreSQL connectivity
The system SHALL connect to the monitored PostgreSQL instance using the connection fields (host, port, database, user, password, sslmode) from the agent config at startup, and SHALL verify connectivity before starting collection.

#### Scenario: Successful connection
- **WHEN** the agent starts with a reachable PostgreSQL and valid credentials
- **THEN** the agent establishes the connection and begins scheduled collection

#### Scenario: Unreachable database
- **WHEN** PostgreSQL is unreachable or credentials are invalid at startup
- **THEN** the agent exits with a non-zero status and an error describing the connection failure

### Requirement: Scheduled collection
The system SHALL organize collection into cycles driven by the global collection interval: at each cycle wake at time X the agent SHALL determine every enabled metric that is due (time since the metric's last collection is at least its effective interval) and collect all of them within that cycle. Disabled metrics SHALL NOT be executed. Within a cycle, collections SHALL be executed over at most `max_db_connections` (default 1) concurrent connections; with the default of 1 the cycle executes metrics one by one in configuration order.

All data points produced in a single cycle SHALL carry the same timestamp: the cycle wake time X in UTC, not individual query completion times.

A metric whose effective interval is shorter than the global interval SHALL be collected more frequently than the global cadence: after each cycle the scheduler SHALL set its wake timer to the earliest next due time among all enabled metrics (its own last collection time plus its effective interval), so every metric is collected at approximately its own effective interval — never delayed by a grid step or another metric's cadence. A metric whose effective interval is longer than the global interval SHALL only be collected when due — intermediate cycles MUST NOT collect it redundantly.

When a metric's collection from a previous cycle is still queued or executing at the next cycle in which it is due, the system SHALL NOT enqueue a duplicate collection; that metric resumes on its subsequent due cycles. The number of concurrent PostgreSQL connections SHALL NOT exceed `max_db_connections` at any point in time.

#### Scenario: Enabled metric collected on schedule
- **WHEN** a metric is enabled with an effective interval of 5 seconds
- **THEN** its query is executed approximately every 5 seconds while the agent runs

#### Scenario: Disabled metric never collected
- **WHEN** a metric is defined with the enabled flag set to false
- **THEN** its query is never executed

#### Scenario: Cycle collects all due metrics with a shared timestamp
- **WHEN** a cycle wakes at time X with 3 metrics due and `max_db_connections` is 1
- **THEN** all 3 metrics are collected one by one within the cycle, and all 3 resulting data points carry the identical timestamp X

#### Scenario: Concurrent connections bounded by configuration
- **WHEN** 100 metrics become due in the same cycle and `max_db_connections` is 2
- **THEN** all metrics are collected in that cycle using at most 2 concurrent PostgreSQL connections, and every metric produces a data point

#### Scenario: Default single connection
- **WHEN** `max_db_connections` is not set in the config and many metrics become due in the same cycle
- **THEN** all collections in the cycle are executed serially over at most 1 concurrent PostgreSQL connection

#### Scenario: Shorter per-metric interval collected more frequently
- **WHEN** the global interval is 500ms and a metric overrides its interval to 100ms
- **THEN** that metric is collected roughly every 100ms while metrics on the global interval are collected roughly every 500ms — the short-interval metric is neither skipped nor delayed to the global cadence

#### Scenario: Longer per-metric interval not redundantly collected
- **WHEN** the global interval is 50ms and a metric overrides its interval to 200ms
- **THEN** that metric is collected only about once every 200ms, not on every 50ms cycle

#### Scenario: No duplicate queueing under contention
- **WHEN** a metric's collection job is still queued or running and a later cycle in which it is due fires
- **THEN** no duplicate collection is enqueued for that metric, and its following collection occurs on a subsequent due cycle

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

### Requirement: Built-in connectivity test metric
The system SHALL provide a built-in test metric whose query is `SELECT 1`, so a fresh deployment can verify the full collection pipeline against any reachable PostgreSQL without authoring custom SQL.

#### Scenario: Default test metric collected
- **WHEN** the agent runs with the default test metric enabled
- **THEN** data points with value 1 are produced and stored at the configured interval

### Requirement: Graceful shutdown
On receiving SIGINT or SIGTERM the system SHALL stop scheduling new collections, allow in-flight queries a bounded time to finish, and then shut down cleanly.

#### Scenario: Signal received during idle period
- **WHEN** the agent receives SIGTERM while no query is in flight
- **THEN** it shuts down promptly with exit code 0

#### Scenario: Signal received mid-query
- **WHEN** the agent receives SIGINT while a query is executing
- **THEN** the in-flight query is allowed to complete or is cancelled after a bounded timeout, and the agent exits cleanly

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
