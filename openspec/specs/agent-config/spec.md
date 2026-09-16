## Purpose

Defines how the agent loads and validates its two YAML configuration files (agent config and metrics config) so that misconfiguration fails fast at startup with a clear error instead of causing runtime surprises.

## Requirements

### Requirement: Agent configuration file
The system SHALL load an agent configuration file in YAML format containing at minimum: the PostgreSQL connection fields for the monitored database (`host` and `user` are required; `port` defaults to 5432, `database` defaults to `postgres`, `password` is optional and MAY be supplied via the `PG_PASSWORD` environment variable, `sslmode` defaults to `prefer`), the global metric collection interval, the data directory for local storage, and the HTTP listen address for the API. The system SHALL apply documented defaults for every optional field.

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

### Requirement: Metrics configuration file
The system SHALL load a separate metrics configuration file in YAML format defining the metrics to collect. Each metric definition SHALL include: a unique name, the SQL query to execute, and the metric type (`counter` or `gauge`). Each metric MAY declare an enabled flag (default enabled) and a per-metric collection interval.

#### Scenario: Metric with all fields
- **WHEN** the metrics config defines a metric with name, query, type `gauge`, and a custom interval
- **THEN** that metric is registered for collection at its custom interval

#### Scenario: Unknown metric type rejected
- **WHEN** a metric definition uses a type other than `counter` or `gauge`
- **THEN** the agent exits with a non-zero status and an error naming the offending metric

#### Scenario: Duplicate metric names rejected
- **WHEN** two metric definitions share the same name
- **THEN** the agent exits with a non-zero status and an error naming the duplicate

### Requirement: Per-metric interval override
The effective collection interval of a metric SHALL be its own configured interval when present, and the global collection interval from the agent config otherwise.

#### Scenario: Metric without explicit interval
- **WHEN** a metric definition omits the interval field and the global interval is 10 seconds
- **THEN** the metric is scheduled for collection every 10 seconds

#### Scenario: Metric with explicit interval
- **WHEN** a metric definition sets an interval of 30 seconds and the global interval is 10 seconds
- **THEN** the metric is scheduled for collection every 30 seconds
