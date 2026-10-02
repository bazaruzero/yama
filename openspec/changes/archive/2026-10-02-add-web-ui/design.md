## Context

The backend (`cmd/yama` + `internal/{config,postgres,collect,store,api}`) is complete and stable: it serves `GET /api/v1/metrics/{name}/data?from=<rfc3339>&to=<rfc3339>` (defaults: last 3h up to now) as `[{"timestamp":...,"value":...}]` ascending, with no CORS headers and no auth. See proposal.md for motivation. This change adds a consumer, not a producer: the agent is consumed as-is, nothing under `internal/` is touched. Hard constraints: separate single static binary, no Node/npm in the toolchain, no new Go modules beyond what's vendored as static files.

## Goals / Non-Goals

**Goals:**
- `yama-web` binary: load two YAML files, serve a light warm dashboard of SVG line charts for the last hour, auto-refreshing via htmx, with degraded states and automatic recovery.
- Walking skeleton first (one panel end-to-end), then grid/theme/no-data/banner polish — mirrored in task ordering.
- Frontend packages fully independent of backend `internal/` packages (no shared imports) so the binaries can evolve/deploy independently.

**Non-Goals:**
- Anything the proposal excludes (multi-series, zoom/pan, pickers, auth, non-1h windows, backend edits).
- No server-side caching of agent responses in baseline (see Risks); revisit only if profiling shows agent load problems.
- Real-time push (SSE/WebSockets) — polling only.

## Decisions

### Binary and package layout: `cmd/yama-web` + `internal/web/{config,agent,server,chart}`
`cmd/yama-web/main.go` is wiring only (flags, config load, `http.Server`), mirroring `cmd/yama`. Frontend logic lives under `internal/web/`: `config` (both YAML files), `agent` (typed HTTP client for the metric data endpoint), `server` (handlers, htmx routing, embedded templates/assets), `chart` (points → SVG). Alternative: `internal/webui/` flat package — rejected; one package per concern matches the backend's layout and keeps `chart`/`agent` independently testable. Alternative: importing backend `internal/config` — rejected; shapes differ and it would couple deployables.

### Frontend config shapes (mirroring backend conventions)
- `webui.yaml` (system): `agent.host` (default `localhost`), `agent.port` (default `8080`), `agent.timeout` (default `5s`), `ui.listen` (default `:8081`), `refresh.interval` (default `10s`). Durations as strings parsed by `time.ParseDuration`; zero/negative rejected.
- `graphs.yaml`: `graphs: [{name, metric, description}]`; `name`/`metric` required, `description` optional, empty list allowed.
- Both parsed with `yaml.v3` + `KnownFields(true)` (strict), same fail-fast style as `internal/config` (exit non-zero naming file/field). Defaults chosen so `ui.listen` (`:8081`) never collides with the agent's default `:8080` when co-located.

### Rendering: server-side HTML + htmx fragments + SVG charts
- Full page `GET /` renders all panels from `graphs.yaml`; each panel body carries `hx-get="/panels/{name}" hx-trigger="every {interval}s" hx-swap="outerHTML"`, so htmx re-fetches the panel fragment and swaps it in place — this alone implements auto-refresh, no-data, and error states, because the fragment is the state.
- `GET /panels/{name}` (HTML fragment) fetches `from=now-1h&to=now` from the agent server-side and returns either the chart SVG, a no-data placeholder, or an error state.
- Connectivity banner: `GET /banner` with `hx-trigger="every {interval}s"`; the fragment returns the banner only when the last panel refresh cycle observed an agent failure (kept simple: any panel fetch failure in the last cycle marks the state; success clears it). State lives in the server process — coarse but sufficient for baseline.
- Charts: `chart` package maps points to a viewBox-scaled smooth monotone cubic Bézier path (Fritsch–Carlson tangents: the curve passes exactly through every point and never overshoots) with round joins/caps, plus axes, min/max/current legend below the chart, and per-point `<circle><title>` native tooltips. Alternative: straight `<polyline>` — rejected as too angular (user feedback); heavier smoothing that misses points — rejected, dots must sit on the line.

### Agent client and failure mapping
`agent.Client` wraps the metric endpoint with the configured timeout, one shared `http.Client` (transport reuse), and normalizes failures: unreachable/timeout/non-2xx/unparseable body → a single error type surfaced as the panel error state + banner trigger. It requests explicit `from`/`to` (not the API's 3h default) so the 1h window holds regardless of agent defaults.

### Assets: embedded, vendored, no CDN
`internal/web/server/assets/` holds `htmx.min.js` (vendored, version-pinned in the file name or a `VERSION` note) and `app.css`; templates in `templates/*.tmpl`; all embedded via `embed.FS`. `app.css` is Tailwind output, compiled by the standalone CLI (`tailwindcss` single binary, `make css`), but the compiled file is committed — day-to-day builds and `go build` need only Go; the Tailwind binary is a dev-only convenience. Rationale: keeps the "no Node" ethos while allowing theme iteration. Alternative: hand-written CSS — viable fallback if Tailwind proves awkward; the build doesn't depend on it either way.

### Time handling
`from`/`to` computed per request as `now().UTC().Add(-time.Hour)` / `now().UTC()` by the frontend server; display axis labels rendered server-side in UTC. Clock skew between the laptop and the agent host may shift the visible window — accepted (noted in Intent), follow-up if it bites.

### Build integration
`Makefile`: extend `build` to also `CGO_ENABLED=0 go build -o bin/yama-web ./cmd/yama-web`; add `css` target (invokes standalone Tailwind CLI if present, else no-ops with a hint); `BINARY` stays `bin/yama`, add `WEB_BINARY := bin/yama-web`. `configs/webui.yaml.example` + `configs/graphs.yaml.example` shipped alongside the existing examples.

## Risks / Trade-offs

- [N panels × 1/interval requests to the agent per cycle (via the frontend proxy)] → Accepted for baseline; typical configs (a handful of panels, 10s interval) are trivial load. If it bites: single server-side fan-out endpoint or a short-TTL response cache — explicit follow-up.
- [Banner state is coarse (any panel failure ⇒ banner)] → Accepted; a single agent serves all panels, so per-panel distinction adds complexity without information.
- [htmx vendored file drifts from upstream] → Pin the version, note the source URL and version in a comment/task; upgrades are a file swap.
- [Tailwind standalone CLI availability] → Committed CSS means builds never require it; document `make css` as optional for theme work.
- [Native SVG tooltips are minimal (no crosshair/zoom)] → Accepted per agreed baseline bar; richer interactivity is follow-up scope.

## Migration Plan

Purely additive: new binary, new example configs, new `internal/web` tree; no changes to existing binaries, specs, or configs. Rollback = stop using `yama-web`; nothing to migrate back.

## Open Questions

(none — the palette and grid questions were settled by user decision: light
warm shades with a terracotta accent, at most two panels per row, and the
min/max/current legend rendered below the chart)
