## MODIFIED Requirements

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
