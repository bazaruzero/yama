## MODIFIED Requirements

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
