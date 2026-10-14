# ui-config delta — add-multi-metrics

## MODIFIED Requirements

### Requirement: Graphs configuration file
The system SHALL load a separate graphs configuration file in YAML format defining the dashboard panels. Each graph definition SHALL include: a unique display `name`, and the `metric` it visualizes from the backend — either a single metric name (scalar string) or a list of metric names (list of strings), referencing any stored series (plain metrics and multi-metric sub-series alike), which MAY be mixed freely within one graph. Each graph MAY include a `description` (default empty) shown in its panel. A graph definition with an empty or missing `name`, a missing or empty `metric` (neither scalar nor non-empty list), or a `metric` list containing duplicate names SHALL be rejected; the `metric` field with any other YAML type (e.g. mapping) SHALL be rejected; duplicate graph names SHALL be rejected; and unknown fields SHALL be rejected. Each rejection SHALL exit with a non-zero status and an error naming the offending graph and file. Graph metric names SHALL NOT be validated against the agent's metric set (the agent is remote and may be unreachable at startup).

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

#### Scenario: Metric list accepted
- **WHEN** a graph definition sets `metric` to a list of two or more series names
- **THEN** the frontend starts and the graph is registered with those series, rendered as one panel

#### Scenario: Metric list with one entry accepted
- **WHEN** a graph definition sets `metric` to a list containing exactly one series name
- **THEN** the frontend starts and the graph behaves identically to the scalar form for that series

#### Scenario: Empty metric list rejected
- **WHEN** a graph definition sets `metric` to an empty list
- **THEN** the frontend exits with a non-zero status and an error naming the graph and the empty metric field

#### Scenario: Duplicate metric names in one graph rejected
- **WHEN** a graph definition's `metric` list contains the same series name twice
- **THEN** the frontend exits with a non-zero status and an error naming the graph and the duplicate series name

#### Scenario: Non-scalar non-list metric rejected
- **WHEN** a graph definition sets `metric` to a YAML mapping
- **THEN** the frontend exits with a non-zero status and an error naming the graph and the invalid metric field

#### Scenario: Mixed series in one graph accepted
- **WHEN** a graph definition's `metric` list mixes a multi-metric sub-series name with an unrelated plain metric name
- **THEN** the frontend starts and the graph is registered with all listed series
