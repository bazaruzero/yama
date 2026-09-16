## 1. Project scaffolding

- [x] 1.1 Initialize Go module (`go mod init github.com/<owner>/yama`, Go 1.22+) and create the layout `cmd/yama/` + `internal/{config,postgres,collect,store,api}`; verify `go build ./...` succeeds
- [x] 1.2 Add dependencies `github.com/jackc/pgx/v5`, `gopkg.in/yaml.v3`, `github.com/dgraph-io/badger/v4`; verify `go mod tidy` resolves and `CGO_ENABLED=0 go build ./cmd/yama` produces a static binary (`file bin/yama` shows "statically linked")
- [x] 1.3 Add `Makefile` with `build`/`test`/`lint` targets and `.gitignore` (bin/, data/); verify `make build` produces `./bin/yama`

## 2. Configuration (capability: agent-config)

- [x] 2.1 Implement agent config loading in `internal/config`: YAML parse with strict known-fields, required `postgres.host` and `postgres.user`, defaults for `postgres.port` (5432), `postgres.database` (`postgres`), `postgres.password` (empty; falls back to `PG_PASSWORD` env var), `postgres.sslmode` (`prefer`), `collector.interval` (10s), `collector.query_timeout` (3s), `storage.data_dir` (`./data`), `api.listen` (`:8080`); verify unit tests cover defaults applied, missing host error, missing user error, and password fallback from the environment
- [x] 2.2 Implement metrics config loading: `{name, query, type, enabled, interval}` with validation for unknown type and duplicate names; verify unit tests cover both rejection cases and the enabled default
- [x] 2.3 Implement effective-interval resolution (per-metric override else global); verify unit test: metric without interval inherits global, metric with 30s keeps 30s
- [x] 2.4 Wire startup validation so any config error exits non-zero with a message naming the file/field; verify manually: malformed YAML, missing host, missing user, and unknown metric type each produce exit code != 0 and a clear message

## 3. Storage (capability: metric-storage)

- [x] 3.1 Define the `store.Store` interface (`Write(point)`, `Query(name, from, to)`, `Close()`) and data-point type `{name, type, value, timestamp}`; verify interface compiles against a stub
- [x] 3.2 Implement BadgerDB store: open with data dir, key = `name + 0x00 + big-endian unixnano`, value = 8-byte big-endian float64, prefix-scan query with ascending order; verify unit tests (in-memory Badger option) cover write→query round-trip, range filtering, unknown metric returns empty, and ordering
- [x] 3.3 Handle open failures (uncreatable/locked data dir) as fatal startup errors; verify unit test or manual run with an unwritable data dir exits non-zero with a storage error
- [x] 3.4 Verify persistence across restarts manually: run agent, stop cleanly, restart with same data dir, confirm previously stored points are returned by query

## 4. Collection (capability: metric-collection)

- [x] 4.1 Implement `internal/postgres`: build pool connection from the config's structured fields (host/port/database/user/password/sslmode), startup `PingContext` with timeout; verify manual test: unreachable host and bad credentials each exit non-zero with a connection error
- [x] 4.2 Implement the collector: execute one metric query with per-run context timeout, require exactly one row / one numeric column, produce a data point with UTC timestamp; verify unit tests (against a test PostgreSQL or sqlmock) cover success, query error, zero rows, and non-numeric result
- [x] 4.3 Implement the scheduler: one ticker goroutine per enabled metric sharing a `signal.NotifyContext`, WaitGroup drain; verify unit test with short intervals observes per-metric independence and that disabled metrics never execute
- [x] 4.4 Wire collector → store so points persist before a run counts as successful; verify integration test: collected points are retrievable from the store afterwards
- [x] 4.5 Ship default `metrics.yaml` with the `select_1` test metric (`SELECT 1`, gauge); verify manual run against real PostgreSQL logs successful collections of value 1 at the configured interval

## 5. REST API (capability: metric-api)

- [x] 5.1 Implement `GET /api/v1/metrics/{name}/data` with optional `from`/`to` RFC3339 params (defaults: `from` = now − 3h, `to` = now), returning JSON `[{"timestamp":...,"value":...}]` ascending; verify handler tests cover explicit range filtering, the 3-hour default window when params are omitted, and start-only requests ending at now
- [x] 5.2 Return `200` + `[]` for metrics with no data in the effective window, `400` + JSON error for unparseable from/to, `405` for non-GET methods; verify handler tests cover all three
- [x] 5.3 Bind the API to the configured listen address and serve concurrently with collection; verify manual test: while the agent collects, `curl` the endpoint and get points without waiting
- [x] 5.4 Fail startup when the listen address is already bound; verify manual test: occupy the port, start agent, confirm non-zero exit naming the address

## 6. Wiring, shutdown, and acceptance

- [x] 6.1 Wire everything in `cmd/yama/main.go`: parse `--config`/`--metrics` flags, load config, open store, ping PostgreSQL, start scheduler and API; verify `yama --config config.yaml --metrics metrics.yaml` starts cleanly with informative logs (slog)
- [x] 6.2 Implement shutdown ordering (signal → cancel schedulers → `http.Server.Shutdown` → drain collectors → close BadgerDB); verify manual test: SIGTERM during idle and mid-collection both exit 0, and data is intact on next start
- [x] 6.3 Provide example `configs/config.yaml.example` and `configs/metrics.yaml.example`; verify a fresh `cp` + edit + run works per README quick-start
- [x] 6.4 End-to-end acceptance: run agent 2 minutes against local PostgreSQL with default configs, then `curl "localhost:8080/api/v1/metrics/select_1/data"` returns multiple points with value 1; stop, restart, confirm the same points are still returned
- [x] 6.5 Run `go vet`, `gofmt`, and the full test suite; verify clean, and confirm the final binary is statically linked with no runtime dependencies (`ldd bin/yama` → "not a dynamic executable")
