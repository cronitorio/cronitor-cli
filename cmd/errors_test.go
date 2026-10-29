package cmd

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/spf13/viper"
)

var readAPITestNow = time.Date(2026, 10, 29, 3, 5, 30, 0, time.UTC)

const (
	testGroupID = "a95936e78b4b51bf4abdbb64d45624ca804c2f4a"
	testEventID = "cd3dc8b6cffb41e4163dcbd857ca87da"
)

// withReadAPITest points the CLI at mock, fixes the clock at readAPITestNow
// and resets the read command flags before and after the test.
func withReadAPITest(t *testing.T, mock *testutil.MockAPI) {
	t.Helper()
	cleanup := setupIntegrationTest(mock.Server.URL)
	oldNow := nowFn
	nowFn = func() time.Time { return readAPITestNow }
	resetErrorsFlags()
	resetLogsFlags()
	t.Cleanup(func() {
		cleanup()
		nowFn = oldNow
		resetErrorsFlags()
		resetLogsFlags()
		RootCmd.SetArgs([]string{"--help"})
	})
}

func assertQuery(t *testing.T, req testutil.RecordedRequest, want map[string]string) {
	t.Helper()
	if len(req.QueryParams) != len(want) {
		t.Errorf("query = %v, want %v", req.QueryParams, want)
	}
	for k, v := range want {
		if got := req.QueryParams.Get(k); got != v {
			t.Errorf("query %s = %q, want %q (all: %v)", k, got, v, req.QueryParams)
		}
	}
}

func assertContainsAll(t *testing.T, output string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output:\n%s", want, output)
		}
	}
}

func assertBasicAuthKey(t *testing.T, req testutil.RecordedRequest) {
	t.Helper()
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("test-api-key-1234567890:"))
	if got := req.Headers.Get("Authorization"); got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
}

func TestErrorsCommandStructure(t *testing.T) {
	for _, name := range []string{"list", "get", "events"} {
		found := false
		for _, c := range errorsCmd.Commands() {
			if c.Name() == name {
				found = true
			}
		}
		if !found {
			t.Errorf("errors has no %q subcommand", name)
		}
	}
	for _, flag := range []string{"env", "monitor", "service", "level", "since", "start", "end", "search", "order-by", "page", "page-size", "json"} {
		if errorsListCmd.Flag(flag) == nil {
			t.Errorf("errors list has no --%s flag", flag)
		}
	}
	if f := errorsListCmd.Flag("search"); f == nil || f.Shorthand != "q" {
		t.Error("errors list --search should have the -q shorthand")
	}
	for _, flag := range []string{"env", "monitor", "service", "series", "before", "limit", "json"} {
		if errorsEventsCmd.Flag(flag) == nil {
			t.Errorf("errors events has no --%s flag", flag)
		}
	}
}

func TestErrorsList_TableAndQuery(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors", 200, testutil.LoadFixture("errors_list.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "list",
		"--env", "production", "--monitor", "nightly-job", "--service", "billing-api",
		"--level", "fatal", "--level", "error,warn", "--since", "24h", "-q", "decode",
		"--order-by", "-count", "--page", "2", "--page-size", "2")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}

	req := mock.LastRequest()
	if req.Method != "GET" || req.Path != "/errors" {
		t.Fatalf("got %s %s, want GET /errors", req.Method, req.Path)
	}
	assertBasicAuthKey(t, req)
	assertQuery(t, req, map[string]string{
		"env":      "production",
		"monitor":  "nightly-job",
		"service":  "billing-api",
		"level":    "fatal,error,warning",
		"start":    fmt.Sprint(readAPITestNow.Add(-24 * time.Hour).Unix()),
		"q":        "decode",
		"orderBy":  "-count",
		"page":     "2",
		"pageSize": "2",
	})

	assertContainsAll(t, output,
		"GROUP", "LEVEL", "COUNT", "LAST SEEN", "TITLE", "MONITORS", "SERVICES",
		"a95936e78b4b", "c83435e089f9",
		"error", "fatal", "42",
		"3m ago", "4h ago",
		"JSONDecodeError: Expecting value",
		"nightly-job,hourly-sync", "billing-api",
		"Showing page 1 of 3 • 5 error groups total",
	)
	if strings.Contains(output, testGroupID) {
		t.Errorf("table should show only the first 12 characters of the group id:\n%s", output)
	}
	if strings.Contains(output, "\n   ") {
		t.Errorf("the footer should not add a line of padding:\n%q", output)
	}
	if strings.Contains(output, "upstream export") {
		t.Errorf("long title should be truncated:\n%s", output)
	}
}

func TestErrorsList_StartEndPassThrough(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors", 200, `{"page":1,"page_count":0,"page_size":50,"total_count":0,"data":[]}`)
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "list", "--start", "2026-10-28T00:00:00Z", "--end", "1761696000")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertQuery(t, mock.LastRequest(), map[string]string{"start": "2026-10-28T00:00:00Z", "end": "1761696000"})
	assertContainsAll(t, output, "No error groups found")
}

func TestErrorsList_SinceConflictsWithStart(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withReadAPITest(t, mock)

	output, code, _ := executeWithExit("errors", "list", "--since", "1h", "--start", "1761696000")
	if code != 1 || !strings.Contains(output, "--since") {
		t.Fatalf("exit %d, want 1 with a --since error:\n%s", code, output)
	}
	if mock.RequestCount() != 0 {
		t.Errorf("no request should be sent, got %d", mock.RequestCount())
	}

	resetErrorsFlags()
	output, code, _ = executeWithExit("errors", "list", "--since", "yesterday")
	if code != 1 || !strings.Contains(output, "30m, 24h, 7d") {
		t.Fatalf("exit %d, want 1 with a duration hint:\n%s", code, output)
	}
}

func TestErrorsList_JSON(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors", 200, testutil.LoadFixture("errors_list.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "list", "--json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertContainsAll(t, output, `"page_count": 3`, `"group": "`+testGroupID+`"`)
	if strings.Contains(output, "LAST SEEN") {
		t.Errorf("--json should not print the table:\n%s", output)
	}
	assertQuery(t, mock.LastRequest(), map[string]string{})
}

func TestErrorsList_APIErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   []string
	}{
		{404, `{"error": "Monitor not found"}`, []string{"API Error (404): Monitor not found"}},
		{400, `{"error": "Invalid orderBy"}`, []string{"API Error (400): Invalid orderBy"}},
		{403, `{"detail": "You do not have permission to perform this action."}`, []string{"API Error (403)", "monitor:read"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			mock := testutil.NewMockAPI()
			defer mock.Close()
			mock.On("GET", "/errors", tc.status, tc.body)
			withReadAPITest(t, mock)

			output, code, _ := executeWithExit("errors", "list", "--monitor", "nope")
			if code != 1 {
				t.Fatalf("exit %d, want 1:\n%s", code, output)
			}
			assertContainsAll(t, output, tc.want...)
		})
	}
}

func TestErrorsLogsRequireAPIKey(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withReadAPITest(t, mock)
	viper.Set("CRONITOR_API_KEY", "")

	for _, args := range [][]string{{"errors", "list"}, {"errors", "get", testGroupID}, {"errors", "events", testGroupID}, {"logs", "search"}} {
		_, _, err := executeWithExit(args...)
		if err == nil || !strings.Contains(err.Error(), "API key required") {
			t.Errorf("%v: err = %v, want the API key error", args, err)
		}
	}
	if mock.RequestCount() != 0 {
		t.Errorf("no request should be sent without a key, got %d", mock.RequestCount())
	}
}

// The traceback of the fixture's latest event: two chained exceptions,
// library frames marked, oldest frame first.
const fixtureTraceback = `Traceback (most recent call last):
  File "jobs/nightly.py", line 18, in fetch_rows
    rows = cache['rows']
KeyError: 'rows'

The above exception was followed by:

Traceback (most recent call last):
  File "jobs/nightly.py", line 52, in <module>
    main()
  File "jobs/nightly.py", line 21, in fetch_rows
    rows = json.loads(body)
  [lib] File "/usr/lib/python3.12/json/__init__.py", line 346, in loads
    return _default_decoder.decode(s)
  [lib] File "/usr/lib/python3.12/json/decoder.py", line 355, in raw_decode
    raise JSONDecodeError("Expecting value", s, err.value) from None
json.decoder.JSONDecodeError: Expecting value: line 1 column 1 (char 0)
`

func TestErrorsGet_SummarySparklineAndTraceback(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID, 200, testutil.LoadFixture("error_group_get.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "get", strings.ToUpper(testGroupID))
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	req := mock.LastRequest()
	if req.Path != "/errors/"+testGroupID {
		t.Fatalf("path = %s", req.Path)
	}
	assertBasicAuthKey(t, req)
	assertQuery(t, req, map[string]string{})

	assertContainsAll(t, output,
		"JSONDecodeError: Expecting value: line 1 column 1 (char 0)\n",
		"Group:        "+testGroupID,
		"Culprit:      jobs.nightly in fetch_rows",
		"Type:         JSONDecodeError",
		"Level:        error (seen: error, warning)",
		"Count:        42",
		"First seen:   2026-10-27T03:00:12Z (2d ago)",
		"Last seen:    2026-10-29T03:02:00Z (3m ago)",
		"Environment:  production",
		"Monitors:     nightly-job",
		"Services:     billing-api",
		"Last 24h:     ▁▁▂▁▁▃▁▁▁▅▁▁▁▁▂▁▁▁▁▄▁▁▁█  19 events, peak 8/h",
		"Latest event",
		"Event:        "+testEventID,
		"Stamp:        2026-10-29T03:02:00Z (3m ago)",
		"Monitor:      nightly-job",
		"Series:       run-123",
		"Release:      2026.10.28",
		"Server:       worker-1",
		"Handled:      no",
		"SDK:          cronitor-cli 33.11",
		fixtureTraceback,
		"Breadcrumbs: 3",
		"Tags: cronitor.monitor=nightly-job, exit_code=1, host=worker-1",
	)
	if strings.Contains(output, "Service:") {
		t.Errorf("an event with no service should not print one:\n%s", output)
	}
}

func TestErrorsGet_JSON(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID, 200, testutil.LoadFixture("error_group_get.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "get", testGroupID, "--json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertContainsAll(t, output, `"sparkline": [`, `"latest_event": {`, `"context_line": "main()"`)
	if strings.Contains(output, "Latest event") || strings.Contains(output, "[lib]") {
		t.Errorf("--json should print only the response:\n%s", output)
	}
}

func TestErrorsGet_RequiresFullGroupID(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withReadAPITest(t, mock)

	output, code, _ := executeWithExit("errors", "get", testGroupID[:12])
	if code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, output)
	}
	assertContainsAll(t, output, "full 40-character hex id", "cronitor errors list --json")
	if mock.RequestCount() != 0 {
		t.Errorf("a short id should not reach the API, got %d requests", mock.RequestCount())
	}
}

func TestErrorsGet_NotFound(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID, 404, `{"error": "Error group not found"}`)
	withReadAPITest(t, mock)

	output, code, _ := executeWithExit("errors", "get", testGroupID)
	if code != 1 {
		t.Fatalf("exit %d, want 1:\n%s", code, output)
	}
	assertContainsAll(t, output, "API Error (404): Error group not found")
}

func TestErrorsGet_EventWithoutFrames(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID, 200, `{
		"group": "`+testGroupID+`", "title": "ExitStatus: Killed", "exception_type": "ExitStatus",
		"level": "fatal", "levels": ["fatal"], "count": 1, "monitors": [], "services": [], "sparkline": [],
		"latest_event": {"event_id": "`+testEventID+`", "stamp": "2026-10-29T03:00:00.000000Z", "level": "fatal",
			"exception_type": "ExitStatus", "exception_value": "Killed", "tags": {},
			"payload": {"exception": {"values": [{"type": "ExitStatus", "value": "Killed"}]}}}
	}`)
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "get", testGroupID)
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertContainsAll(t, output, "ExitStatus: Killed\n", "No stack trace", "Breadcrumbs: 0", "Tags: none", "Monitors:     -")
	if strings.Contains(output, "Traceback (most recent call last)") || strings.Contains(output, "Last 24h") {
		t.Errorf("no frames and no sparkline should print neither:\n%s", output)
	}
}

func TestErrorsEvents_ListQueryAndTable(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID+"/events", 200, testutil.LoadFixture("error_events_list.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "events", testGroupID,
		"--env", "production", "--monitor", "nightly-job", "--service", "billing-api",
		"--series", "run-123", "--before", "1761706800", "--limit", "2")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	req := mock.LastRequest()
	if req.Path != "/errors/"+testGroupID+"/events" {
		t.Fatalf("path = %s", req.Path)
	}
	assertQuery(t, req, map[string]string{
		"env": "production", "monitor": "nightly-job", "service": "billing-api",
		"series": "run-123", "before": "1761706800", "limit": "2",
	})
	assertContainsAll(t, output,
		"STAMP", "LEVEL", "EVENT ID", "MONITOR", "SERIES", "TITLE",
		"2026-10-29T03:02:00Z", testEventID, "nightly-job", "run-123",
		"2026-10-28T23:10:05Z", "7ed21b04c0dcde1c638e5c8480ad0026", "billing-api", "warning",
		"JSONDecodeError: Expecting value",
		"next: 2026-10-28T23:10:05.000001Z",
	)
}

func TestErrorsEvents_LastPageHasNoNext(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID+"/events", 200, `{"data": [], "has_more": false, "next": null}`)
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "events", testGroupID)
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertContainsAll(t, output, "No events found")
	if strings.Contains(output, "next:") {
		t.Errorf("no next cursor expected:\n%s", output)
	}
}

func TestErrorsEvents_OneEvent(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID+"/events/"+testEventID, 200, testutil.LoadFixture("error_event_get.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "events", testGroupID, testEventID)
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if req := mock.LastRequest(); req.Path != "/errors/"+testGroupID+"/events/"+testEventID {
		t.Fatalf("path = %s", req.Path)
	}
	assertContainsAll(t, output,
		"JSONDecodeError: Expecting value: line 1 column 1 (char 0)\n",
		"Event:        "+testEventID,
		"Series:       run-123",
		fixtureTraceback,
		"Breadcrumbs: 3",
		"Tags: cronitor.monitor=nightly-job, exit_code=1, host=worker-1",
	)
}

func TestErrorsEvents_JSONAndBadEventID(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+testGroupID+"/events/"+testEventID, 200, testutil.LoadFixture("error_event_get.json"))
	mock.On("GET", "/errors/"+testGroupID+"/events", 200, testutil.LoadFixture("error_events_list.json"))
	withReadAPITest(t, mock)

	output, code, err := executeWithExit("errors", "events", testGroupID, testEventID, "--json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertContainsAll(t, output, `"payload": {`, `"event_id": "`+testEventID+`"`)

	resetErrorsFlags()
	output, code, err = executeWithExit("errors", "events", testGroupID, "--json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertContainsAll(t, output, `"has_more": true`)
	if strings.Contains(output, "next:") {
		t.Errorf("--json should not print the footer:\n%s", output)
	}

	resetErrorsFlags()
	mock.Reset()
	output, code, _ = executeWithExit("errors", "events", testGroupID, "abc")
	if code != 1 || !strings.Contains(output, "32 hex characters") || mock.RequestCount() != 0 {
		t.Fatalf("exit %d, requests %d:\n%s", code, mock.RequestCount(), output)
	}
}

// hostileText is what anyone holding a telemetry key can put in an event or
// log record: clear the screen, write the clipboard (OSC 52), move the
// cursor up (C1 CSI) and return to the start of the line.
const hostileText = `\u001b[2J\u001b]52;c;Y3VybCBldmlsLnNoIHwgc2g=\u0007\u009b1A\r`

// assertNoTerminalControls fails if output has any control character but
// a newline or tab.
func assertNoTerminalControls(t *testing.T, output string) {
	t.Helper()
	for _, r := range output {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			t.Fatalf("output has control character %U:\n%q", r, output)
		}
	}
	if strings.Contains(output, "52;c;") {
		t.Errorf("the OSC 52 payload should be stripped with its sequence:\n%q", output)
	}
}

func TestErrorsRenderingStripsTerminalControls(t *testing.T) {
	h := hostileText
	event := `{"event_id": "` + testEventID + `", "stamp": "2026-10-29T03:00:00.000000Z", "level": "error` + h + `",
		"title": "Boom` + h + `", "message": "msg` + h + `", "exception_type": "BoomError` + h + `",
		"exception_value": "first line` + h + `\nsecond line", "culprit": "job` + h + `",
		"transaction": "tx` + h + `", "logger": "lg` + h + `", "server_name": "srv` + h + `",
		"release": "rel` + h + `", "environment": "prod` + h + `", "series": "run` + h + `",
		"monitor": {"key": "nightly` + h + `"}, "user": {"id": "u` + h + `", "email": "e` + h + `"},
		"sdk": {"name": "sdk` + h + `", "version": "1` + h + `"}, "tags": {"tag` + h + `": "val` + h + `"},
		"payload": {"breadcrumbs": [], "exception": {"values": [{"type": "BoomError` + h + `", "module": "app` + h + `",
			"value": "first line` + h + `\nsecond line",
			"stacktrace": {"frames": [{"filename": "/app/job` + h + `.py", "function": "main` + h + `",
				"lineno": 3, "context_line": "raise BoomError()` + h + `", "in_app": true}]}}]}}}`
	group := `{"group": "` + testGroupID + `", "title": "Boom` + h + `", "culprit": "job` + h + `",
		"exception_type": "BoomError` + h + `", "level": "error", "levels": ["error", "fatal` + h + `"], "count": 1,
		"environment": "prod` + h + `", "monitors": [{"key": "nightly` + h + `"}], "services": [{"key": "api` + h + `"}],
		"first_seen": "2026-10-29T03:00:00.000000Z", "last_seen": "2026-10-29T03:00:00.000000Z` + h + `",
		"sparkline": [], "latest_event": ` + event + `}`

	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors", 200, `{"page": 1, "page_count": 1, "page_size": 50, "total_count": 1, "data": [`+group+`]}`)
	mock.On("GET", "/errors/"+testGroupID, 200, group)
	mock.On("GET", "/errors/"+testGroupID+"/events", 200, `{"data": [`+event+`], "has_more": true, "next": "2026-10-29T03:00:00.000000Z`+h+`"}`)
	mock.On("GET", "/errors/"+testGroupID+"/events/"+testEventID, 200, event)
	withReadAPITest(t, mock)

	detail := []string{
		"BoomError: first line\nsecond line\n", // multi-line values keep their line breaks
		`File "/app/job.py", line 3, in main`,
		"    raise BoomError()\n",
		"Tags: tag=val\n",
		"Series:       run\n",
	}
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"errors", "list"}, []string{"Boom", "nightly"}},
		{[]string{"errors", "get", testGroupID}, append([]string{"Boom\n", "Culprit:      job\n", "Monitors:     nightly\n", "Services:     api\n"}, detail...)},
		{[]string{"errors", "events", testGroupID}, []string{"Boom", "nightly", "run", "next: 2026-10-29T03:00:00.000000Z"}},
		{[]string{"errors", "events", testGroupID, testEventID}, append([]string{"Boom\n"}, detail...)},
	} {
		resetErrorsFlags()
		output, code, err := executeWithExit(tc.args...)
		if err != nil || code != 0 {
			t.Fatalf("%v: exit %d err %v\n%s", tc.args, code, err, output)
		}
		assertNoTerminalControls(t, output)
		assertContainsAll(t, output, tc.want...)
	}
}
