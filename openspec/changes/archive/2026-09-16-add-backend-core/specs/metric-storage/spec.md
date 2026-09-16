## Purpose

Defines how collected metric data points are persisted in an embedded local database so that they survive restarts and can be queried efficiently by metric name and time range, without any external database server.

## ADDED Requirements

### Requirement: Embedded local persistence
The system SHALL store all collected data points in an embedded database running in-process, using the data directory from the agent config. The system MUST NOT require SQLite or any standalone database server.

#### Scenario: Data survives restart
- **WHEN** the agent has stored data points, is stopped cleanly, and is started again with the same data directory
- **THEN** all previously stored data points are retrievable

#### Scenario: Uncreatable data directory
- **WHEN** the configured data directory cannot be created or opened (permissions, disk full)
- **THEN** the agent exits with a non-zero status and an error describing the storage failure

### Requirement: Durable write path
Every data point produced by collection SHALL be written to the store. A collection run is only considered successful after its data point has been persisted.

#### Scenario: Collected point is stored
- **WHEN** the collection engine produces a data point
- **THEN** it is durably present in the store and retrievable by subsequent queries

#### Scenario: Storage failure surfaced
- **WHEN** a write to the store fails
- **THEN** the failure is logged with the metric name and the agent continues collecting subsequent data points

### Requirement: Time-range retrieval
The system SHALL support retrieving all data points for a given metric name whose timestamps fall within an inclusive start/end time range, returned ordered by timestamp ascending.

#### Scenario: Range query returns matching points
- **WHEN** data points exist for a metric at times both inside and outside the requested range
- **THEN** only the points inside the range are returned, in ascending timestamp order

#### Scenario: Unknown metric returns empty result
- **WHEN** a range query names a metric with no stored data points
- **THEN** an empty result is returned (not an error)

### Requirement: Clean storage lifecycle
The store SHALL be opened at startup and closed cleanly during graceful shutdown, flushing pending writes, so that a clean stop never corrupts stored data.

#### Scenario: Clean shutdown preserves data
- **WHEN** the agent shuts down gracefully after storing data points
- **THEN** on next startup the store opens without repair and all data points are intact
