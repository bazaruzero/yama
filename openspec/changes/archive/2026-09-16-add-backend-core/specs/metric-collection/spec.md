## Purpose

Defines how the agent connects to the monitored PostgreSQL database and executes configured metric queries on independent schedules, producing typed, timestamped data points without ever crashing on query errors.

## ADDED Requirements

### Requirement: PostgreSQL connectivity
The system SHALL connect to the monitored PostgreSQL instance using the connection fields (host, port, database, user, password, sslmode) from the agent config at startup, and SHALL verify connectivity before starting collection.

#### Scenario: Successful connection
- **WHEN** the agent starts with a reachable PostgreSQL and valid credentials
- **THEN** the agent establishes the connection and begins scheduled collection

#### Scenario: Unreachable database
- **WHEN** PostgreSQL is unreachable or credentials are invalid at startup
- **THEN** the agent exits with a non-zero status and an error describing the connection failure

### Requirement: Scheduled collection
The system SHALL execute each enabled metric's SQL query repeatedly at its effective interval. Disabled metrics SHALL NOT be executed. Each metric SHALL be scheduled independently so one metric's execution does not delay another's.

#### Scenario: Enabled metric collected on schedule
- **WHEN** a metric is enabled with an effective interval of 5 seconds
- **THEN** its query is executed approximately every 5 seconds while the agent runs

#### Scenario: Disabled metric never collected
- **WHEN** a metric is defined with the enabled flag set to false
- **THEN** its query is never executed

### Requirement: Data point production
Each successful query execution SHALL produce a data point containing: the metric name, the metric type, the numeric value returned by the query, and the collection timestamp in UTC. A query MUST return exactly one row with one numeric column to produce a data point.

#### Scenario: Successful collection
- **WHEN** a metric query executes successfully and returns one numeric value
- **THEN** a data point with the metric name, type, value, and current UTC timestamp is passed to storage

#### Scenario: Query failure does not stop collection
- **WHEN** a metric query fails (SQL error, timeout, or lost connection)
- **THEN** the failure is logged with the metric name and reason, no data point is produced for that run, and future scheduled executions of that and other metrics continue

#### Scenario: Unexpected result shape
- **WHEN** a metric query returns zero rows, multiple rows, or a non-numeric value
- **THEN** the run is treated as a query failure for that metric

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
