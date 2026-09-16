## Purpose

Defines the read-only HTTP REST API through which downstream consumers (initially the future dashboard frontend) retrieve collected metric data points for a metric over a time range.

## ADDED Requirements

### Requirement: Metric data retrieval endpoint
The system SHALL expose an HTTP endpoint that returns the stored data points for a named metric, bounded by optional start and end time query parameters, as a JSON array ordered by timestamp ascending. Each data point SHALL include its timestamp and value. When the start parameter is omitted, the window SHALL default to the last 3 hours up to now; when the end parameter is omitted, it SHALL default to now.

#### Scenario: Retrieve points in range
- **WHEN** a client requests data points for a metric with stored points inside the requested start/end range
- **THEN** the response is `200 OK` with a JSON array of those points in ascending timestamp order

#### Scenario: Default window when parameters omitted
- **WHEN** a client requests data points for a metric without start/end parameters
- **THEN** the response contains only points from the last 3 hours up to now, in ascending timestamp order

#### Scenario: Start-only request defaults end to now
- **WHEN** a client supplies a start parameter but no end parameter
- **THEN** the response contains points from the given start up to now, in ascending timestamp order

#### Scenario: Metric with no data
- **WHEN** a client requests data points for a metric name that has no stored points in the effective window
- **THEN** the response is `200 OK` with an empty JSON array

#### Scenario: Invalid time range
- **WHEN** a client supplies a start/end parameter that cannot be parsed as a timestamp
- **THEN** the response is `400 Bad Request` with a JSON error message naming the invalid parameter

### Requirement: Read-only API
The API SHALL expose only read operations for metric data. No endpoint SHALL create, modify, or delete stored data or configuration.

#### Scenario: Write attempt rejected
- **WHEN** a client sends a POST, PUT, PATCH, or DELETE request to the metric data endpoint
- **THEN** the response is `405 Method Not Allowed`

### Requirement: Configurable listen address
The API SHALL bind to the listen address from the agent config and SHALL serve requests concurrently with ongoing metric collection.

#### Scenario: API available while collecting
- **WHEN** the agent is running and collecting metrics on schedule
- **THEN** the API responds to requests on the configured address without waiting for collection runs

#### Scenario: Port already in use
- **WHEN** the configured listen address is already bound by another process at startup
- **THEN** the agent exits with a non-zero status and an error naming the address
