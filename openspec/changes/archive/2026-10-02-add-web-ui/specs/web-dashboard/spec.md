## Purpose

Defines the frontend's dashboard server: it turns any running, unmodified agent's read-only metric API into a live, Grafana-style web page with a light warm theme, served by a standalone binary, including refresh behavior and degraded states.

## ADDED Requirements

### Requirement: Dashboard page from graphs config
The system SHALL serve a single dashboard page in HTML at the root path on the configured listen address, rendering one panel per graph defined in the graphs config, ordered as configured. Each panel SHALL show the graph's title and description and a line chart of its metric. The page SHALL use a light warm color theme, panels SHALL be laid out at most two per row (stacking to one on narrow screens), and each chart SHALL show its min/max/current legend below the chart. All page assets (styles and scripts) SHALL be served by the frontend itself, with no requests to external hosts. The frontend SHALL start and serve the page even when the agent is unreachable.

#### Scenario: Dashboard renders all configured panels
- **WHEN** the graphs config defines three graphs and the browser opens the dashboard root
- **THEN** the page shows three panels in configuration order, each with its title and description

#### Scenario: Assets served locally
- **WHEN** the dashboard page is loaded in a browser
- **THEN** all styles and scripts are fetched from the frontend's own origin and no external host is contacted

#### Scenario: Frontend starts with agent down
- **WHEN** the frontend is started while the agent is unreachable
- **THEN** the frontend starts, serves the dashboard page, and shows the backend-unreachable state instead of failing to start

#### Scenario: Listen port already in use
- **WHEN** the configured listen address is already bound by another process at startup
- **THEN** the frontend exits with a non-zero status and an error naming the address

### Requirement: Server-side metric retrieval from the agent
The system SHALL retrieve metric data for panels by calling the agent's existing read-only metric data endpoint server-side, and SHALL NOT require the browser to call the agent directly. All data requests issued by the dashboard SHALL target the frontend's own origin.

#### Scenario: Browser-origin-only data flow
- **WHEN** the dashboard displays data in a browser that has no cross-origin access to the agent
- **THEN** panels still render, because only the frontend server contacts the agent

#### Scenario: Agent queried with the one-hour range
- **WHEN** the frontend server fetches data for a panel
- **THEN** it requests the metric's points for the last hour up to now from the agent's metric data endpoint

### Requirement: Fixed one-hour display window
Each panel SHALL display the metric's data points falling within the last hour up to the current time, and SHALL NOT display points older than one hour.

#### Scenario: Older points excluded
- **WHEN** a metric has stored points both inside and outside the last hour
- **THEN** the panel's chart shows only the points inside the last hour

### Requirement: Automatic refresh without reload
The system SHALL refresh every panel's data automatically at the configured polling interval, without reloading the page and without any user action.

#### Scenario: New point appears automatically
- **WHEN** the dashboard is open and idle, and a new data point is collected by the agent
- **THEN** the new point appears in the panel within roughly one polling interval, with no page reload

### Requirement: Panel no-data state
When a panel's metric has no data points in the display window, the system SHALL render an explicit no-data indication in that panel, and other panels SHALL remain unaffected.

#### Scenario: Metric without data
- **WHEN** a configured metric name has no stored points in the last hour
- **THEN** that panel shows an explicit no-data indication instead of a chart, and the rest of the page renders normally

### Requirement: Backend connectivity state and automatic recovery
When the agent cannot be reached, the system SHALL show a visible indication of the backend problem on the page, SHALL keep serving the dashboard, and SHALL recover automatically — resuming data display and clearing the indication — once the agent becomes reachable again, without a frontend restart or page reload.

#### Scenario: Agent stops mid-session
- **WHEN** the dashboard is open and the agent process is stopped
- **THEN** the page shows a visible backend-unreachable indication at the next refresh

#### Scenario: Automatic recovery after agent restart
- **WHEN** the agent becomes reachable again after an outage
- **THEN** panels resume showing current data and the backend-unreachable indication clears, without restarting the frontend or reloading the page
