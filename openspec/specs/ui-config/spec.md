## Purpose

Defines how the frontend loads and validates its two YAML configuration files (system config and graphs config) so that misconfiguration fails fast at startup with a clear error instead of causing runtime surprises.

## Requirements

### Requirement: Frontend system configuration file
The system SHALL load a frontend system configuration file in YAML format containing: the agent backend address (`agent.host` defaults to `localhost`, `agent.port` defaults to `8080`), the HTTP timeout for calls to the agent (`agent.timeout`, a positive duration defaulting to `5s`), the dashboard listen address (`ui.listen`, defaulting to `:8081`), and the polling interval for automatic refresh (`refresh.interval`, a positive duration defaulting to `10s`). The system SHALL apply documented defaults for every optional field. Any duration field set to zero or a negative value, and any field that cannot be parsed, SHALL cause exit with a non-zero status and an error naming the file and field. Unknown fields SHALL be rejected.

#### Scenario: Minimal config applies defaults
- **WHEN** the frontend is started with a system config containing only the agent address fields
- **THEN** the frontend listens on `:8081`, polls every 10 seconds, and bounds agent calls at 5 seconds

#### Scenario: Fully specified config honored
- **WHEN** the system config sets the agent host/port to a remote agent, `ui.listen` to a custom address, and a 30-second polling interval
- **THEN** the frontend connects to that remote agent and serves the dashboard on the custom address with 30-second refresh

#### Scenario: Non-positive duration rejected
- **WHEN** the system config sets `refresh.interval` to `0s` or a negative duration
- **THEN** the frontend exits with a non-zero status and an error naming `refresh.interval` and its file

#### Scenario: Unknown field rejected
- **WHEN** the system config contains a field other than the documented agent, ui, and refresh fields
- **THEN** the frontend exits with a non-zero status and a parse error identifying the unknown field

#### Scenario: Malformed YAML
- **WHEN** the system config file is not valid YAML
- **THEN** the frontend exits with a non-zero status and an error message identifying the file and parse problem

### Requirement: Graphs configuration file
The system SHALL load a separate graphs configuration file in YAML format defining the dashboard panels. Each graph definition SHALL include: a unique display `name`, and the `metric` name it visualizes from the backend. Each graph MAY include a `description` (default empty) shown in its panel. A graph definition with an empty or missing `name` or `metric` SHALL be rejected, duplicate graph names SHALL be rejected, and unknown fields SHALL be rejected; each rejection SHALL exit with a non-zero status and an error naming the offending graph and file.

#### Scenario: Graph with all fields
- **WHEN** the graphs config defines a graph with name, metric, and description
- **THEN** the dashboard renders one panel titled with that name, showing that metric with the description beneath

#### Scenario: Duplicate graph names rejected
- **WHEN** two graph definitions share the same name
- **THEN** the frontend exits with a non-zero status and an error naming the duplicate

#### Scenario: Missing metric rejected
- **WHEN** a graph definition omits the metric field
- **THEN** the frontend exits with a non-zero status and an error naming the graph and the missing field

#### Scenario: Unknown graph field rejected
- **WHEN** a graph definition contains a field other than name, metric, or description
- **THEN** the frontend exits with a non-zero status and a parse error identifying the unknown field

#### Scenario: Empty graphs list accepted
- **WHEN** the graphs config defines an empty list of graphs
- **THEN** the frontend starts and serves a dashboard with no panels
