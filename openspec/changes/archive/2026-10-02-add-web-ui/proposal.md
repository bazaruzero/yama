## Why

YAMA's backend collects and stores metrics, but the only consumption path is `curl` against the read-only API — visually assessing trends is effectively impossible. The existing `metric-api` spec was written explicitly for a "future dashboard frontend"; the API contract is ready and waiting for this consumer. Operators need a fast feedback loop on the health of monitored PostgreSQL instances without installing a Grafana server.

## What Changes

- Add an independent frontend binary (`yama-web`, `cmd/yama-web`) deployable separately from the agent: point it at any running, unmodified YAMA agent and it serves a Grafana-style dashboard with a light warm theme of that agent's metrics.
- Server-rendered UI: Go HTML templates, vendored single-file htmx for declarative auto-refresh polling (`hx-trigger="every Ns"`), server-rendered SVG line charts, Tailwind CSS (compiled at build via the standalone CLI; committed CSS keeps day-to-day builds Go-only). No Node/npm toolchain.
- Two YAML config files for the frontend: a **system** config (agent backend URL, UI listen address, polling interval, HTTP timeout) and a **graphs** config (panel definitions: display name, backend metric name, description).
- Fixed rolling last-1-hour window with auto-refresh at the polling interval; no page reload, no time picker in baseline.
- The frontend server proxies API calls to the agent (server-side); the browser never calls the agent directly. Zero backend changes.
- Panel-level "no data" states and a visible backend-connectivity error banner with automatic recovery.

Non-goals for this change: any backend/agent modification, multi-series panels, zoom/pan/time pickers, UI-side graph editing, UI authentication, and windows other than the last hour.

## Capabilities

### New Capabilities

- `ui-config`: Loading and validating the frontend's two YAML configuration files (system config + graphs config) with defaults and fail-fast validation with clear errors.
- `web-dashboard`: The frontend HTTP server — serving the dashboard page, proxying metric data from the agent over the existing read-only API, rendering panels as server-side SVG line charts for the fixed last-hour window, auto-refresh via htmx polling, and degraded states (panel "no data", backend-connectivity banner with automatic recovery).

### Modified Capabilities

(none — the change consumes `metric-api` as-is; `metric-api`, `metric-collection`, `metric-storage`, and `agent-config` remain untouched)

## Impact

- **New code**: `cmd/yama-web/` (wiring) and new `internal/` packages for the frontend (config, agent client, server/rendering) — no edits to existing backend packages.
- **Dependencies**: no new Go modules. Static assets vendored in-repo: `htmx.min.js` (single file) and compiled Tailwind CSS; templates embedded via `embed`.
- **Build**: `Makefile` gains the `yama-web` binary target (and an optional CSS build step using the Tailwind standalone CLI; not required when committed CSS is current).
- **External systems**: one running YAMA agent reachable over HTTP (read-only usage of `GET /api/v1/metrics/{name}/data`).
- **Deployment**: one additional static binary plus two YAML files; runs anywhere the agent is reachable (e.g., a laptop pointed at a remote agent).
