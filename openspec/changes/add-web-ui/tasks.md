## 1. Scaffolding and assets

- [ ] 1.1 Create `cmd/yama-web/main.go` (empty wiring: flags `--config`/`--graphs`, placeholder config load, start HTTP server) and package dirs `internal/web/{config,agent,server,chart}`; verify `go build ./...` succeeds
- [ ] 1.2 Vendor `htmx.min.js` (pin the version, record source URL and version in the directory) into `internal/web/server/assets/`; add `app.css` placeholder and embed both plus `templates/` via `embed.FS`; verify a test serving the asset returns 200 from the binary's own origin
- [ ] 1.3 Extend `Makefile`: `build` also produces `bin/yama-web` (`CGO_ENABLED=0 go build`), add `WEB_BINARY`, add optional `css` target (no-op with hint when the Tailwind standalone CLI is absent); verify `make build` yields both statically linked binaries (`ldd` → not dynamic)
- [ ] 1.4 Add `configs/webui.yaml.example` and `configs/graphs.yaml.example` with comments matching the existing examples' style; verify a fresh `cp` + edit works with the quick start below

## 2. Frontend configuration (capability: ui-config)

- [ ] 2.1 Implement system config loading in `internal/web/config`: strict YAML (`KnownFields`) with `agent.host` (default `localhost`), `agent.port` (default `8080`), `agent.timeout` (default `5s`), `ui.listen` (default `:8081`), `refresh.interval` (default `10s`); verify unit tests cover defaults, non-positive duration rejection naming field+file, unknown field rejection, and malformed YAML
- [ ] 2.2 Implement graphs config loading: `{name, metric, description}` with duplicate-name rejection, missing name/metric rejection, unknown-field rejection, empty-list acceptance; verify unit tests cover each rejection case and the empty list

## 3. Agent client (supports web-dashboard)

- [ ] 3.1 Implement `internal/web/agent`: typed client for `GET /api/v1/metrics/{name}/data` with the configured timeout, one shared `http.Client`, and a single normalized error for unreachable/timeout/non-2XX/unparseable responses; verify unit tests (httptest) cover success parsing, non-2XX, timeout, and malformed body
- [ ] 3.2 Verify the client sends explicit `from`/`to` (now−1h to now, RFC3339): unit test asserts the query parameters on a captured request

## 4. Chart rendering (supports web-dashboard)

- [ ] 4.1 Implement `internal/web/chart`: points → SVG polyline scaled into a fixed viewBox, with axis labels (UTC), min/max/current legend values, and per-point `<circle><title>` tooltips; verify unit tests cover empty input (caller renders no-data instead), single point, scaling to flat series (min == max), and out-of-window points being excluded by the caller-supplied range
- [ ] 4.2 Render the dark theme styling in templates + committed `app.css` (Tailwind output or hand-written fallback); verify visual smoke check in a browser against a live agent

## 5. Server and dashboard (capability: web-dashboard)

- [ ] 5.1 Implement `internal/web/server`: `GET /` renders the full page — one panel per graph in config order, each panel body with `hx-get="/panels/{name}" hx-trigger="every {interval}s" hx-swap="outerHTML"`; `GET /panels/{name}` returns the fragment (chart SVG, no-data, or error state); verify handler tests (httptest agent) cover panel order/content, no-data fragment, and error fragment
- [ ] 5.2 Implement the connectivity banner: `GET /banner` fragment polled at the refresh interval, driven by server-side state set by the last panel fetch cycle (any failure ⇒ banner, success ⇒ clear); verify handler test: agent returns 500 then recovers ⇒ banner fragment appears then clears without restart
- [ ] 5.3 Serve assets from `embed.FS` at fixed paths and confirm the page makes no external-host requests (inspect browser network tab or assert asset URLs are same-origin in the handler test)
- [ ] 5.4 Wire `cmd/yama-web/main.go` end-to-end: flags → configs → server on `ui.listen`; fail fast on config errors (non-zero exit naming file/field) and on listen-address bind failure (non-zero exit naming the address), but NOT on unreachable agent; verify manual runs for the three startup outcomes

## 6. Acceptance

- [ ] 6.1 Auto-refresh end-to-end: dashboard open against a live agent with a short refresh interval; verify a newly collected point appears within ~one interval with no reload, and that points older than 1h never appear
- [ ] 6.2 Degraded-state end-to-end: stop the agent, verify the banner appears and panels show error/no-data states; restart the agent, verify data resumes and the banner clears without restarting the frontend
- [ ] 6.3 Remote-agent laptop scenario: run `bin/yama-web` on a different machine (or against a remote host:port) pointed at the agent; verify the dashboard works with only the agent's HTTP port reachable (no CORS, no agent-side changes)
- [ ] 6.4 Run `go vet`, `gofmt` check, and full `go test ./...`; verify clean and both binaries build static
