## Purpose

Defines how the agent loads and validates its two YAML configuration files (agent config and metrics config) so that misconfiguration fails fast at startup with a clear error instead of causing runtime surprises.

## Requirements

### Requirement: Agent configuration file
The system SHALL load an agent configuration file in YAML format containing at minimum: the PostgreSQL connection fields for the monitored database (`host` and `user` are required; `port` defaults to 5432, `database` defaults to `postgres`, `password` is optional and MAY be supplied via the `PG_PASSWORD` environment variable, `sslmode` defaults to `prefer`, `max_db_connections` is optional and defaults to 1 and SHALL be an integer of at least 1), the global metric collection interval, the data directory for local storage, and the HTTP listen address for the API. The system SHALL apply documented defaults for every optional field. If `max_db_connections` is explicitly set to a value less than 1 (including 0), the system SHALL exit with a non-zero status and an error naming the field.

#### Scenario: Valid minimal config starts the agent
- **WHEN** the agent is started with a config file containing a valid host and user and all optional fields omitted
- **THEN** the agent loads the config, applies defaults, and proceeds to start collection and the API

#### Scenario: Password from environment variable
- **WHEN** the config omits the password field and `PG_PASSWORD` is set in the environment
- **THEN** the agent uses the environment variable's value as the PostgreSQL password

#### Scenario: Missing host
- **WHEN** the agent is started with a config file that lacks the PostgreSQL host field
- **THEN** the agent exits with a non-zero status and an error message naming the missing field

#### Scenario: Missing user
- **WHEN** the agent is started with a config file that lacks the PostgreSQL user field
- **THEN** the agent exits with a non-zero status and an error message naming the missing field

#### Scenario: Malformed YAML
- **WHEN** the agent config file is not valid YAML
- **THEN** the agent exits with a non-zero status and an error message identifying the file and parse problem

#### Scenario: Default max_db_connections
- **WHEN** the agent config omits `postgres.max_db_connections`
- **THEN** the agent limits itself to at most 1 concurrent PostgreSQL connection

#### Scenario: Invalid max_db_connections
- **WHEN** the agent config sets `postgres.max_db_connections` to 0 or a negative number
- **THEN** the agent exits with a non-zero status and an error message naming `postgres.max_db_connections` and the minimum value of 1

### Requirement: Metrics configuration file
The system SHALL load a separate metrics configuration file in YAML format defining the metrics to collect. Each metric definition SHALL include: a unique name, the SQL query to execute, and the metric type (`counter` or `gauge`). Each metric MAY declare an enabled flag (default enabled), a per-metric collection interval, and a per-metric query timeout (duration string such as `30s` or integer seconds). Unknown fields in the metrics configuration SHALL be rejected.

#### Scenario: Metric with all fields
- **WHEN** the metrics config defines a metric with name, query, type `gauge`, a custom interval, and a custom query timeout
- **THEN** that metric is registered for collection at its custom interval with its custom query timeout

#### Scenario: Unknown metric type rejected
- **WHEN** a metric definition uses a type other than `counter` or `gauge`
- **THEN** the agent exits with a non-zero status and an error naming the offending metric

#### Scenario: Duplicate metric names rejected
- **WHEN** two metric definitions share the same name
- **THEN** the agent exits with a non-zero status and an error naming the duplicate

#### Scenario: Unknown metric field rejected
- **WHEN** a metric definition contains a field other than name, query, type, enabled, interval, or query timeout
- **THEN** the agent exits with a non-zero status and a parse error identifying the unknown field

### Requirement: Per-metric interval override
The effective collection interval of a metric SHALL be its own configured interval when present, and the global collection interval from the agent config otherwise.

#### Scenario: Metric without explicit interval
- **WHEN** a metric definition omits the interval field and the global interval is 10 seconds
- **THEN** the metric is scheduled for collection every 10 seconds

#### Scenario: Metric with explicit interval
- **WHEN** a metric definition sets an interval of 30 seconds and the global interval is 10 seconds
- **THEN** the metric is scheduled for collection every 30 seconds

### Requirement: Per-metric query timeout override
The effective query timeout of a metric SHALL be its own configured `query_timeout` when present (a positive duration), and the global `collector.query_timeout` from the agent config otherwise. A metric that omits `query_timeout` SHALL behave exactly as before this override existed.

#### Scenario: Metric without explicit query timeout
- **WHEN** a metric definition omits `query_timeout` and the global `collector.query_timeout` is 3 seconds
- **THEN** the metric's query executions are bounded by 3 seconds

#### Scenario: Metric with longer explicit query timeout
- **WHEN** a metric definition sets `query_timeout: 30s` and the global `collector.query_timeout` is 3 seconds
- **THEN** the metric's query executions are bounded by 30 seconds

#### Scenario: Metric with shorter explicit query timeout
- **WHEN** a metric definition sets `query_timeout: 500ms` and the global `collector.query_timeout` is 3 seconds
- **THEN** the metric's query executions are bounded by 500 milliseconds

### Requirement: Effective query timeout must not exceed effective interval
At config load, after both configuration files are parsed and defaults applied, the system SHALL exit with a non-zero status and a clear, actionable error whenever an effective query timeout strictly exceeds its corresponding effective collection interval. This SHALL be enforced for the global pair (`collector.query_timeout` vs `collector.interval`) and for every metric (its effective query timeout vs its effective interval, combining per-metric overrides with global values). The error SHALL identify the offending metric (when the violation is metric-specific) and both conflicting values with their sources (metric override or global setting). Equality — an effective query timeout exactly equal to the effective interval — SHALL be accepted.

#### Scenario: Explicit per-metric violation rejected
- **WHEN** a metric sets `query_timeout: 15s` with effective interval 10 seconds (from its own or the global interval)
- **THEN** the agent exits with a non-zero status and an error naming the metric and both conflicting values

#### Scenario: Global timeout with custom metric interval rejected
- **WHEN** the global `collector.query_timeout` is 5 seconds and a metric overrides only its `interval` to 2 seconds
- **THEN** the agent exits with a non-zero status and an error naming the metric, the effective timeout, and the effective interval

#### Scenario: Global pair violation rejected
- **WHEN** the agent config sets `collector.query_timeout: 15s` and `collector.interval: 10s`
- **THEN** the agent exits with a non-zero status and an error naming both global settings and their values

#### Scenario: Timeout equal to interval accepted
- **WHEN** a metric's effective query timeout equals its effective interval (from overrides, globals, or both)
- **THEN** the configuration loads and the agent starts

#### Scenario: Default configuration accepted
- **WHEN** both duration fields are omitted (defaults: query timeout 3s, interval 10s)
- **THEN** the configuration loads and the agent starts, with all metrics bounded by the 3-second global timeout
