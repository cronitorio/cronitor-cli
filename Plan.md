# Cronitor CLI — Full API Support Plan

## Overview

Add first-class CLI support for the entire Cronitor REST API as top-level resource commands with consistent subcommands (list, get, create, update, delete, plus resource-specific actions).

## Task Tracking

When working on a task, prefix it with **`[WORKING]`** to indicate it is actively in progress. When the task is complete, remove the prefix and mark it as done (`[x]`). Only one task should be marked `[WORKING]` at a time.

**Branch:** `claude/cronitor-api-support-PLatz`
**API Version:** Configurable via `--api-version` flag, `CRONITOR_API_VERSION` env var, or config file (header omitted when unset)
**Base URL:** `https://cronitor.io/api/`

---

## Architecture

- Each API resource is a top-level cobra command (e.g. `cronitor monitor`, `cronitor group`)
- Subcommands follow CRUD conventions: `list`, `get`, `create`, `update`, `delete`
- Resources with special actions get additional subcommands (e.g. `monitor pause`, `group resume`)
- Shared flags across all resources: `--format` (json/table/yaml), `--output`, `--page`
- API client lives in `lib/cronitor.go` / `lib/api_client.go` with GET/POST/PUT/DELETE helpers
- Table output uses lipgloss styling via shared helpers in `cmd/ui.go`

---

## Completed Work

### Phase 1: Core Infrastructure [DONE]

- [x] API client with HTTP Basic Auth (`lib/api_client.go`, `lib/cronitor.go`)
- [x] `Cronitor-Version: 2025-11-28` header sent on all requests
- [x] Shared output formatting: JSON, YAML, table (`--format`, `--output` flags)
- [x] Table rendering with lipgloss styling (`cmd/ui.go`)
- [x] Color palette, status badges, and formatting helpers

### Phase 2: Resource Commands [DONE]

All 9 resources implemented with `Run` functions wired to real API calls:

- [x] **monitor** — list, get, search, create, update, delete, clone, pause, unpause
  - Filters: `--type`, `--group`, `--tag`, `--state`, `--search`, `--sort`, `--env`
  - File: `cmd/monitor.go`

- [x] **group** — list, get, create, update, delete, pause, resume
  - Filters: `--env`, `--with-status`, `--page-size`, `--sort`
  - File: `cmd/group.go`

- [x] **environment** (alias: `env`) — list, get, create, update, delete
  - File: `cmd/environment.go`

- [x] **notification** (alias: `notifications`) — list, get, create, update, delete
  - Supports all channels: email, slack, pagerduty, opsgenie, victorops, microsoft-teams, discord, telegram, gchat, larksuite, webhooks
  - File: `cmd/notification.go`

- [x] **issue** — list, get, create, update, resolve, delete
  - Filters: `--state`, `--severity`, `--monitor`, `--group`, `--tag`, `--env`, `--search`, `--time`, `--order-by`
  - File: `cmd/issue.go`

- [x] **maintenance** (alias: `maint`) — list, get, create, update, delete
  - Filters: `--past`, `--ongoing`, `--upcoming`, `--statuspage`, `--env`, `--with-monitors`
  - File: `cmd/maintenance.go`

- [x] **statuspage** — list, get, create, update, delete
  - Nested: `component list`, `component create`, `component delete`
  - Filters: `--with-status`, `--with-components`
  - File: `cmd/statuspage.go`

- [x] **metric** (alias: `metrics`) — get, aggregate
  - Filters: `--monitor`, `--group`, `--tag`, `--type`, `--time`, `--start`, `--end`, `--env`, `--region`, `--with-nulls`
  - Fields: duration_p10/p50/p90/p99, duration_mean, success_rate, run_count, complete_count, fail_count, tick_count, alert_count
  - File: `cmd/metric.go`

- [x] **site** — list, get, create, update, delete, query, error {list, get}
  - Query kinds: aggregation, breakdown, timeseries, search_options, error_groups
  - File: `cmd/site.go`

- [x] **ping** — updated with richer flags: `--run`, `--complete`, `--fail`, `--ok`, `--tick`, `--msg`, `--series`, `--status-code`, `--duration`, `--metric`
  - File: `cmd/ping.go`

### Phase 3: Structural Tests [DONE]

All resources have test files verifying:
- Command and subcommand hierarchy
- Flag presence and types
- Argument validation
- Aliases
- Help text and examples

Test files: `cmd/*_test.go` (monitor, environment, issue, notification, statuspage, group, maintenance, metric, site, discover)

---

## Remaining Work

### Phase 4: Missing Subcommands & Flags [DONE]

- [x] **Statuspage component update** — `component update` subcommand (`PUT /statuspage_components/:key`)
  - Updatable fields: name, description, autopublish
  - File: `cmd/statuspage.go`

- [x] **Issue bulk actions** — `issue bulk` subcommand (`POST /issues/bulk`)
  - Actions: delete, change_state, assign_to
  - Accepts: `--action`, `--issues` (comma-separated keys), `--state`, `--assign-to`
  - File: `cmd/issue.go`

- [x] **Issue expansion flags** — `--with-statuspage-details`, `--with-monitor-details`, `--with-alert-details`, `--with-component-details` on `issue list` and `issue get`
  - These map to query params: `withStatusPageDetails`, `withMonitorDetails`, `withAlertDetails`, `withComponentDetails`
  - File: `cmd/issue.go`

### Phase 5: Testing

Current state: All 10 resource test files (`cmd/*_test.go`) only verify command structure (subcommands, flags, aliases, argument counts). There is **no HTTP mocking, no behavioral testing, and no output verification**. This phase adds robust API-level testing.

#### 5a: Test Infrastructure [DONE]

- [x] **Shared test helpers** — Create `lib/api_test_helpers.go` (or `lib/testutil_test.go`)
  - `NewMockAPIServer()` — returns an `httptest.NewServer` that:
    - Records incoming requests (method, path, query params, headers, body) for assertion
    - Returns configurable JSON responses per route (method + path pattern)
    - Supports setting response status codes (200, 400, 403, 404, 429, 500)
    - Validates `Authorization` header (HTTP Basic with API key)
    - Validates `Cronitor-Version` header presence/absence
  - `AssertRequest(t, recorded, expected)` — helper to compare method, path, query params, body fields
  - `LoadFixture(name string)` — reads JSON fixture files from `testdata/` directory
  - `CaptureOutput(fn func()) string` — captures stdout for output format assertions

- [x] **Test fixtures** — Create `testdata/` directory with representative API responses
  - `testdata/monitors_list.json` — paginated list response with 2-3 monitors
  - `testdata/monitor_get.json` — single monitor with all fields populated
  - `testdata/groups_list.json`, `testdata/group_get.json`
  - `testdata/environments_list.json`, `testdata/environment_get.json`
  - `testdata/notifications_list.json`, `testdata/notification_get.json`
  - `testdata/issues_list.json`, `testdata/issue_get.json`
  - `testdata/maintenance_list.json`, `testdata/maintenance_get.json`
  - `testdata/statuspages_list.json`, `testdata/statuspage_get.json`
  - `testdata/components_list.json`
  - `testdata/metrics_get.json`, `testdata/aggregates_get.json`
  - `testdata/sites_list.json`, `testdata/site_get.json`
  - `testdata/site_query.json`, `testdata/site_errors_list.json`
  - `testdata/error_responses/` — 400, 403, 404, 429, 500 responses

#### 5b: API Client Tests (`lib/api_client_test.go`) [DONE]

- [x] **Authentication** — Verify API key is sent as HTTP Basic Auth (username = API key, no password)
- [x] **Cronitor-Version header** — Verify header is sent (currently hardcoded to `2025-11-28`). Version-absent test will be added after Phase 6 makes it configurable.
- [x] **HTTP methods** — Each helper (GET, POST, PUT, DELETE) sends the correct method
- [x] **URL construction** — Base URL + resource path + query params are built correctly
- [x] **Request body** — POST/PUT send correct JSON body from `--data` flag
- [x] **Error handling** — Client returns meaningful errors for:
  - 400 Bad Request (validation errors from API)
  - 403 Forbidden (invalid API key)
  - 404 Not Found (invalid resource key)
  - 429 Rate Limited (with Retry-After header)
  - 500 Server Error
  - Network errors (connection refused, timeout)
  - Malformed JSON response

#### 5c: Per-Resource Request Tests [DONE]

All per-resource endpoint tests are in `lib/api_client_test.go` using table-driven tests against the mock server. Tests cover correct HTTP method, path, query params, and request body for every endpoint.

- [x] **Monitor tests** (`cmd/monitor_test.go` — extend existing file)
  - `list` — GET /monitors, with each filter flag mapped to correct query param (`--type`→`type`, `--group`→`group`, `--tag`→`tag`, `--state`→`state`, `--search`→`search`, `--sort`→`sort`, `--env`→`env`, `--page`→`page`)
  - `get KEY` — GET /monitors/KEY
  - `create --data '{...}'` — POST /monitors with JSON body
  - `update KEY --data '{...}'` — PUT /monitors with JSON body containing key
  - `delete KEY` — DELETE /monitors/KEY
  - `delete KEY1 KEY2` — bulk delete via DELETE /monitors with body
  - `clone KEY --name NEW` — POST /monitors/clone with correct body
  - `pause KEY` — GET /monitors/KEY/pause (no duration)
  - `pause KEY --hours 4` — GET /monitors/KEY/pause/4
  - `unpause KEY` — GET /monitors/KEY/pause/0
  - `search QUERY` — GET /api/search?query=QUERY

- [x] **Group tests** (`cmd/group_test.go` — extend)
  - `list` — GET /groups, with filters (`--env`, `--with-status`, `--page-size`, `--sort`)
  - `get KEY` — GET /groups/KEY
  - `create --data '{...}'` — POST /groups
  - `update KEY --data '{...}'` — PUT /groups/KEY
  - `delete KEY` — DELETE /groups/KEY
  - `pause KEY 4` — GET /groups/KEY/pause/4
  - `resume KEY` — GET /groups/KEY/pause/0

- [x] **Environment tests** (`cmd/environment_test.go` — extend)
  - `list` — GET /environments
  - `get KEY` — GET /environments/KEY
  - `create --data '{...}'` — POST /environments
  - `update KEY --data '{...}'` — PUT /environments/KEY
  - `delete KEY` — DELETE /environments/KEY

- [x] **Notification tests** (`cmd/notification_test.go` — extend)
  - `list` — GET /notifications
  - `get KEY` — GET /notifications/KEY
  - `create --data '{...}'` — POST /notifications
  - `update KEY --data '{...}'` — PUT /notifications/KEY
  - `delete KEY` — DELETE /notifications/KEY

- [x] **Issue tests** (`cmd/issue_test.go` — extend)
  - `list` — GET /issues, with all filter flags (`--state`, `--severity`, `--monitor`, `--group`, `--tag`, `--env`, `--search`, `--time`, `--order-by`)
  - `get KEY` — GET /issues/KEY
  - `create --data '{...}'` — POST /issues
  - `update KEY --data '{...}'` — PUT /issues/KEY
  - `resolve KEY` — PUT /issues/KEY with state=resolved
  - `delete KEY` — DELETE /issues/KEY
  - `bulk --action delete --issues KEY1,KEY2` — POST /issues/bulk (after Phase 4)

- [x] **Maintenance tests** (`cmd/maintenance_test.go` — extend)
  - `list` — GET /maintenance_windows, with filters (`--past`, `--ongoing`, `--upcoming`, `--statuspage`, `--env`, `--with-monitors`)
  - `get KEY` — GET /maintenance_windows/KEY
  - `create --data '{...}'` — POST /maintenance_windows
  - `update KEY --data '{...}'` — PUT /maintenance_windows/KEY
  - `delete KEY` — DELETE /maintenance_windows/KEY

- [x] **Statuspage tests** (`cmd/statuspage_test.go` — extend)
  - `list` — GET /statuspages, with filters (`--with-status`, `--with-components`)
  - `get KEY` — GET /statuspages/KEY
  - `create --data '{...}'` — POST /statuspages
  - `update KEY --data '{...}'` — PUT /statuspages/KEY
  - `delete KEY` — DELETE /statuspages/KEY
  - `component list` — GET /statuspage_components
  - `component create --data '{...}'` — POST /statuspage_components
  - `component update KEY --data '{...}'` — PUT /statuspage_components/KEY (after Phase 4)
  - `component delete KEY` — DELETE /statuspage_components/KEY

- [x] **Metric tests** (`cmd/metric_test.go` — extend)
  - `get` — GET /metrics, with filters (`--monitor`, `--group`, `--tag`, `--type`, `--time`, `--start`, `--end`, `--env`, `--region`, `--with-nulls`, `--field`)
  - `aggregate` — GET /aggregates, with same filters

- [x] **Site tests** (`cmd/site_test.go` — extend)
  - `list` — GET /sites
  - `get KEY` — GET /sites/KEY
  - `create --data '{...}'` — POST /sites
  - `update KEY --data '{...}'` — PUT /sites/KEY
  - `delete KEY` — DELETE /sites/KEY
  - `query --site KEY --type aggregation` — POST /sites/query with correct body
  - `error list --site KEY` — GET /site_errors?site=KEY
  - `error get KEY` — GET /site_errors/KEY

#### 5d: Response Parsing & Output Tests

- [x] **JSON output** — `FormatJSON()` tested: pretty-prints valid JSON, returns raw on invalid

The remaining items require command-level integration tests that execute cobra commands against a mock server and verify stdout/file output. These test the glue between "API returns JSON" and "user sees formatted output."

**Known limitation:** Commands call `os.Exit(1)` on errors, which kills the test process. Error-path integration tests are deferred. A future improvement would be to refactor commands to return errors instead of calling `os.Exit` directly.

**Scope note:** These integration tests are intentionally representative, not exhaustive. The goal is to verify each output format works end-to-end for a couple of commands, not to re-test every endpoint (already covered by `lib/api_client_test.go`).

##### Step 1: Create `internal/testutil/mock_api.go` [DONE]

- [x] Create `internal/testutil/mock_api.go` with exported `MockAPI`, `NewMockAPI()`, `RecordedRequest`, `On()`, `OnWithHeaders()`, `SetDefault()`, `LastRequest()`, `RequestCount()`, `Reset()` — copied from the existing package-private implementation in `lib/api_client_test.go`

##### Step 2: Create `internal/testutil/capture.go` [DONE]

- [x] Create `CaptureStdout(fn func()) string` helper
  - Redirects `os.Stdout` to an `os.Pipe()`, runs `fn`, reads the pipe, restores stdout
  - Needed because commands use `fmt.Println` directly, not cobra's `cmd.OutOrStdout()`

##### Step 3: Create `internal/testutil/command.go` [DONE]

- [x] Create `ExecuteCommand(root *cobra.Command, args ...string) (string, error)` helper
  - Calls `root.SetArgs(args)`, wraps `root.Execute()` inside `CaptureStdout`, returns captured output + error
  - Also handles setup boilerplate: sets `lib.BaseURLOverride` to the mock server URL and `viper.Set("CRONITOR_API_KEY", "test-key")`

##### Step 4: Refactor `lib/api_client_test.go` to use shared mock [DONE]

- [x] Replace the local `MockAPI` / `RecordedRequest` / `NewMockAPI` in `lib/api_client_test.go` with imports from `internal/testutil`
  - Verify all existing lib tests still pass after refactor

##### Step 5: Unit test `MergePagedJSON` [DONE]

- [x] Add test in `cmd/ui_test.go` (or create it if it doesn't exist)
  - Given two page response bodies: `{"items":[{"id":1}]}` and `{"items":[{"id":2}]}`
  - Assert `MergePagedJSON(bodies, "items")` returns `[{"id":1},{"id":2}]`
  - Test edge cases: empty pages, single page, mismatched keys

##### Step 6: Unit test `FetchAllPages` [DONE]

- [x] Add test in `cmd/ui_test.go` using mock server from `internal/testutil`
  - Mock returns items on page 1 and 2, empty array on page 3
  - Assert `FetchAllPages` returns 2 bodies (stops at empty page)
  - Assert it sends incrementing `page` query param
  - Test safety limit behavior (mock always returns items, assert it stops at 200)

##### Step 7: Integration test — table output [DONE]

- [x] Add `cmd/integration_test.go`
  - Test `monitor list` (default format = table):
    - Mock returns `testdata/monitors_list.json` fixture on `GET /monitors`
    - Assert output contains column headers: "NAME", "KEY", "TYPE", "STATUS"
    - Assert output contains monitor names/keys from the fixture
  - Test `issue list --format table`:
    - Mock returns `testdata/issues_list.json` fixture
    - Assert output contains "NAME", "KEY", "STATE", "SEVERITY"

##### Step 8: Integration test — JSON output [DONE]

- [x] Test `monitor list --format json`:
  - Mock returns fixture on `GET /monitors`
  - Assert output is valid JSON (`json.Valid()`)
  - Assert output contains expected monitor keys from the fixture
- [x] Test `monitor get my-job --format json`:
  - Mock returns fixture on `GET /monitors/my-job`
  - Assert output is valid pretty-printed JSON

##### Step 9: Integration test — YAML output [DONE]

- [x] Test `monitor list --format yaml`:
  - Mock returns YAML-formatted body when `format=yaml` query param is present
  - Assert output is non-empty and matches what the mock returned (passthrough test)

##### Step 10: Integration test — output to file [DONE]

- [x] Test `monitor list --format json --output <tmpfile>`:
  - Execute command with `--output` pointing to `t.TempDir()` file
  - Assert file exists, contains valid JSON matching the fixture
  - Assert captured stdout contains "Output written to" but NOT the JSON data

##### Step 11: Integration test — pagination metadata [DONE]

- [x] Test `monitor list` (table format) with pagination:
  - Mock returns fixture with `page_info.totalMonitorCount` > page size
  - Assert output contains pagination string (e.g., "Showing page 1")

##### Step 12: Integration test — `--all` flag [DONE]

- [x] Test `monitor list --all --format json`:
  - Mock returns different items on `GET /monitors?page=1` vs `page=2`, empty on `page=3`
  - Assert output is a merged JSON array containing items from both pages

#### 5e: Error Handling Tests [DONE]

All error handling tested in `lib/api_client_test.go`:

- [x] **Invalid API key** — 403 response parses "Invalid API key" from error body
- [x] **Resource not found** — 404 `IsNotFound()` correctly returns true
- [x] **Validation errors** — 400 `ParseError()` extracts messages from `errors[]` array
- [x] **Rate limiting** — 429 response captures `Retry-After` header
- [x] **Server errors** — 500 `ParseError()` returns "Internal server error"
- [x] **Network errors** — Connection refused returns `request failed` error (not panic)
- [x] **Malformed responses** — Invalid JSON handled gracefully by `FormatJSON()` and `ParseError()`
- [x] **Response helpers** — `IsSuccess()` tested for all status code ranges (2xx true, 3xx/4xx/5xx false)

#### 5f: Configuration & Version Header Tests [DONE]

All version header tests implemented in `lib/api_client_test.go` after Phase 6 made the header configurable via viper:

- [x] **No version configured** — `TestVersionHeader_NotSentWhenUnset` and `TestVersionHeader_NotSentAcrossAllMethods` verify no header when `CRONITOR_API_VERSION` is empty
- [x] **Version in config file / env var** — `TestVersionHeader_SentWhenConfigured` and `TestVersionHeader_DifferentVersionValues` verify header sent with correct value via `viper.Set()`
- [x] **All HTTP methods** — `TestVersionHeader_AppliesAcrossAllMethods` verifies header on GET, POST, PUT, DELETE, PATCH
- [x] **Priority order** — `TestVersionHeader_ViperPriority_EnvOverridesConfig` verifies viper precedence (env var overrides config)

#### 5g: Run & Fix Existing Tests [DONE]

- [x] **Run all tests** — `go test ./cmd/... ./lib/...` passes (all existing structural tests + all new API client tests)
- [x] **Fix any failures** — No failures found; all tests pass
- [x] **Verify test coverage** — `go test -cover ./cmd/... ./lib/...` shows adequate coverage for new code

### Phase 6: Polish & Edge Cases [DONE]

- [x] **Configurable `Cronitor-Version` header** — Remove hardcoded version, make it configurable across the entire CLI
  - Removed hardcoded `2025-11-28` from `lib/api_client.go` and `lib/cronitor.go` (both `send()` and `sendWithContentType()`)
  - Added `varApiVersion = "CRONITOR_API_VERSION"` to `cmd/root.go`
  - Added `--api-version` persistent flag on `RootCmd` (available to all commands)
  - Added `ApiVersion` field to `ConfigFile` struct in `cmd/configure.go`
  - Configure command reads and displays API version
  - Header only sent when `CRONITOR_API_VERSION` is non-empty (via env var, config file, or `--api-version` flag)
  - Extended `Monitor.UnmarshalJSON()` to normalize singular `schedule` (string) into `schedules` ([]string) for cross-version compatibility

- [x] **Consistent error messaging** — Audited all commands for consistent error output
  - All API errors use: `Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))`
  - All network errors use: `Error(fmt.Sprintf("Failed to <action> <resource>: %s", err))`
  - Added missing `IsNotFound()` checks to: group get/delete, issue update, notification update, maintenance delete, statuspage update/component update/component delete, site delete, monitor update

- [x] **Pagination helpers** — Added `--all` flag to all list commands
  - `FetchAllPages()` and `MergePagedJSON()` helpers in `cmd/ui.go`
  - For JSON: merges all pages into a single JSON array
  - For table: accumulates rows from all pages, renders once
  - Added to: monitor, group, environment, issue, notification, maintenance, statuspage, site

- [x] **Output to file** — Verified `--output` flag works correctly across all commands
  - Fixed group.go: added missing newline in file write, standardized success message to `Info()`
  - Fixed bypass issues: routed "no results found" messages through output functions in group.go, maintenance.go, metric.go, site.go

---

## API Reference Quick Map

| CLI Command | API Endpoint | Methods |
|-------------|-------------|---------|
| `monitor list` | `GET /monitors` | GET |
| `monitor get KEY` | `GET /monitors/:key` | GET |
| `monitor search QUERY` | `GET /api/search` | GET |
| `monitor create` | `POST /monitors` (single), `PUT /monitors` (batch) | POST, PUT |
| `monitor update KEY` | `PUT /monitors` | PUT |
| `monitor delete KEY` | `DELETE /monitors/:key` or `DELETE /monitors` (bulk) | DELETE |
| `monitor clone KEY` | `POST /monitors/clone` | POST |
| `monitor pause KEY` | `GET /monitors/:key/pause[/:hours]` | GET |
| `monitor unpause KEY` | `GET /monitors/:key/pause/0` | GET |
| `group list` | `GET /groups` | GET |
| `group get KEY` | `GET /groups/:key` | GET |
| `group create` | `POST /groups` | POST |
| `group update KEY` | `PUT /groups/:key` | PUT |
| `group delete KEY` | `DELETE /groups/:key` | DELETE |
| `group pause KEY HOURS` | `GET /groups/:key/pause/:hours` | GET |
| `group resume KEY` | `GET /groups/:key/pause/0` | GET |
| `environment list` | `GET /environments` | GET |
| `environment get KEY` | `GET /environments/:key` | GET |
| `environment create` | `POST /environments` | POST |
| `environment update KEY` | `PUT /environments/:key` | PUT |
| `environment delete KEY` | `DELETE /environments/:key` | DELETE |
| `notification list` | `GET /notifications` | GET |
| `notification get KEY` | `GET /notifications/:key` | GET |
| `notification create` | `POST /notifications` | POST |
| `notification update KEY` | `PUT /notifications/:key` | PUT |
| `notification delete KEY` | `DELETE /notifications/:key` | DELETE |
| `issue list` | `GET /issues` | GET |
| `issue get KEY` | `GET /issues/:key` | GET |
| `issue create` | `POST /issues` | POST |
| `issue update KEY` | `PUT /issues/:key` | PUT |
| `issue resolve KEY` | `PUT /issues/:key` (state=resolved) | PUT |
| `issue delete KEY` | `DELETE /issues/:key` | DELETE |
| `issue bulk` | `POST /issues/bulk` | POST |
| `maintenance list` | `GET /maintenance_windows` | GET |
| `maintenance get KEY` | `GET /maintenance_windows/:key` | GET |
| `maintenance create` | `POST /maintenance_windows` | POST |
| `maintenance update KEY` | `PUT /maintenance_windows/:key` | PUT |
| `maintenance delete KEY` | `DELETE /maintenance_windows/:key` | DELETE |
| `statuspage list` | `GET /statuspages` | GET |
| `statuspage get KEY` | `GET /statuspages/:key` | GET |
| `statuspage create` | `POST /statuspages` | POST |
| `statuspage update KEY` | `PUT /statuspages/:key` | PUT |
| `statuspage delete KEY` | `DELETE /statuspages/:key` | DELETE |
| `statuspage component list` | `GET /statuspage_components` | GET |
| `statuspage component create` | `POST /statuspage_components` | POST |
| `statuspage component update KEY` | `PUT /statuspage_components/:key` | PUT |
| `statuspage component delete KEY` | `DELETE /statuspage_components/:key` | DELETE |
| `metric get` | `GET /metrics` | GET |
| `metric aggregate` | `GET /aggregates` | GET |
| `site list` | `GET /sites` | GET |
| `site get KEY` | `GET /sites/:key` | GET |
| `site create` | `POST /sites` | POST |
| `site update KEY` | `PUT /sites/:key` | PUT |
| `site delete KEY` | `DELETE /sites/:key` | DELETE |
| `site query` | `POST /sites/query` | POST |
| `site error list` | `GET /site_errors` | GET |
| `site error get KEY` | `GET /site_errors/:key` | GET |
