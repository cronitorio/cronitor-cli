package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
)

const (
	fixtureLogLineA = "2026-10-29T03:00:10Z  INFO   billing-api  -  starting"
	fixtureLogLineB = `2026-10-29T03:00:12Z  WARN   nightly-job  run-123  retrying export\nattempt 2`
	fixtureLogLineC = "2026-10-29T03:00:14Z  ERROR  nightly-job  run-123  Traceback (most recent call last):"
)

func withLogsTest(t *testing.T, mock *testutil.MockAPI) {
	t.Helper()
	withReadAPITest(t, mock)
	oldWait := logsFollowWait
	t.Cleanup(func() { logsFollowWait = oldWait })
}

// executeWithExitStderr is executeWithExit that also returns stderr.
func executeWithExitStderr(args ...string) (stdout, stderr string, code int, err error) {
	_, stderr = testutil.CaptureStdoutStderr(func() {
		stdout, code, err = executeWithExit(args...)
	})
	return stdout, stderr, code, err
}

func TestLogsCommandStructure(t *testing.T) {
	found := false
	for _, c := range logsCmd.Commands() {
		if c.Name() == "search" {
			found = true
			if !c.HasAlias("list") {
				t.Error("logs search should have the alias list")
			}
		}
	}
	if !found {
		t.Fatal("logs has no search subcommand")
	}
	for _, flag := range []string{"env", "monitor", "service", "series", "level", "since", "start", "end", "search", "trace-id", "before", "limit", "json", "follow", "attributes"} {
		if logsSearchCmd.Flag(flag) == nil {
			t.Errorf("logs search has no --%s flag", flag)
		}
	}
	if f := logsSearchCmd.Flag("search"); f == nil || f.Shorthand != "q" {
		t.Error("logs search --search should have the -q shorthand")
	}
}

func TestLogsSearch_QueryAndLines(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, testutil.LoadFixture("logs_search.json"))
	withLogsTest(t, mock)

	stdout, stderr, code, err := executeWithExitStderr("logs", "search",
		"--env", "production", "--monitor", "nightly-job", "--service", "billing-api",
		"--series", "run-123", "--level", "warning", "--since", "1h",
		"-q", "timeout", "--search", "db", "--trace-id", "4bf92f35",
		"--before", "2026-10-29T03:01:00Z", "--limit", "3")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	req := mock.LastRequest()
	if req.Method != "GET" || req.Path != "/logs" {
		t.Fatalf("got %s %s, want GET /logs", req.Method, req.Path)
	}
	assertBasicAuthKey(t, req)
	assertQuery(t, req, map[string]string{
		"env": "production", "monitor": "nightly-job", "service": "billing-api",
		"series": "run-123", "level": "warn",
		"start":    fmt.Sprint(readAPITestNow.Add(-time.Hour).Unix()),
		"q":        "timeout db",
		"trace_id": "4bf92f35",
		"before":   "2026-10-29T03:01:00Z",
		"limit":    "3",
	})

	want := fixtureLogLineA + "\n" + fixtureLogLineB + "\n" + fixtureLogLineC + "\n"
	if stdout != want {
		t.Errorf("stdout, oldest first:\n%s\nwant:\n%s", stdout, want)
	}
	if !strings.Contains(stderr, "next: 2026-10-29T03:00:10.000000Z") {
		t.Errorf("stderr should carry the next cursor, got %q", stderr)
	}
}

func TestLogsList_AliasAndAttributes(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, testutil.LoadFixture("logs_search.json"))
	withLogsTest(t, mock)

	stdout, code, err := executeWithExit("logs", "list", "--attributes")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	assertQuery(t, mock.LastRequest(), map[string]string{})
	assertContainsAll(t, stdout,
		fixtureLogLineA+"  line=1\n",
		fixtureLogLineB+`  cronitor.monitor=nightly-job line=2 note="two words"`+"\n",
		fixtureLogLineC+"  cronitor.monitor=nightly-job cronitor.series=run-123 line=3\n",
	)
}

func TestLogsSearch_JSON(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, testutil.LoadFixture("logs_search.json"))
	withLogsTest(t, mock)

	stdout, stderr, code, err := executeWithExitStderr("logs", "search", "--json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	assertContainsAll(t, stdout, `"has_more": true`, `"severity_number": 17`)
	// --json keeps the API's order, newest first.
	if strings.Index(stdout, "03:00:14.500000Z") > strings.Index(stdout, "03:00:10.000000Z") {
		t.Errorf("--json should keep the API order:\n%s", stdout)
	}
	if strings.Contains(stderr, "next:") {
		t.Errorf("--json should not print the cursor hint, got %q", stderr)
	}
}

func TestLogsSearch_EmptyAndErrors(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, `{"data": [], "has_more": false, "next": null}`)
	withLogsTest(t, mock)

	stdout, stderr, code, err := executeWithExitStderr("logs", "search", "--monitor", "nightly-job")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	if stdout != "" || !strings.Contains(stderr, "No log records found") {
		t.Errorf("stdout %q stderr %q", stdout, stderr)
	}

	mock.On("GET", "/logs", 404, `{"error": "Service not found"}`)
	resetLogsFlags()
	stdout, code, _ = executeWithExit("logs", "search", "--service", "nope")
	if code != 1 || !strings.Contains(stdout, "API Error (404): Service not found") {
		t.Errorf("exit %d:\n%s", code, stdout)
	}

	for _, args := range [][]string{
		{"logs", "search", "--follow", "--before", "1761706800"},
		{"logs", "search", "--follow", "--end", "1761706800"},
		{"logs", "search", "--since", "1h", "--start", "1761706800"},
	} {
		resetLogsFlags()
		mock.Reset()
		stdout, code, _ = executeWithExit(args...)
		if code != 1 || mock.RequestCount() != 0 {
			t.Errorf("%v: exit %d, %d requests:\n%s", args, code, mock.RequestCount(), stdout)
		}
	}
}

func TestLogsSearch_Follow(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	initial := `{"data": [
		{"stamp": "2026-10-29T03:00:12.250000Z", "level": "warn", "body": "B", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:10.000000Z", "level": "info", "body": "A", "monitor": {"key": "nightly-job"}, "series": "run-123"}
	], "has_more": false, "next": null}`
	// The first poll returns a full page and a cursor, then the page before it.
	poll1page1 := `{"data": [
		{"stamp": "2026-10-29T03:00:15.000000Z", "level": "error", "body": "D", "monitor": {"key": "nightly-job"}, "series": "run-123"}
	], "has_more": true, "next": "2026-10-29T03:00:15.000000Z"}`
	poll1page2 := `{"data": [
		{"stamp": "2026-10-29T03:00:13.000000Z", "level": "info", "body": "C", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:12.250000Z", "level": "warn", "body": "B", "monitor": {"key": "nightly-job"}, "series": "run-123"}
	], "has_more": false, "next": null}`
	// The second poll re-reads the last minute: E is stamped before D but
	// arrived after D was printed, as when an SDK sends a batch late. F is
	// new. A, B, C and D were printed already.
	poll2 := `{"data": [
		{"stamp": "2026-10-29T03:00:16.000000Z", "level": "info", "body": "F", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:15.000000Z", "level": "error", "body": "D", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:14.000000Z", "level": "info", "body": "E", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:13.000000Z", "level": "info", "body": "C", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:12.250000Z", "level": "warn", "body": "B", "monitor": {"key": "nightly-job"}, "series": "run-123"},
		{"stamp": "2026-10-29T03:00:10.000000Z", "level": "info", "body": "A", "monitor": {"key": "nightly-job"}, "series": "run-123"}
	], "has_more": false, "next": null}`
	mock.OnSequence("GET", "/logs", 200, initial, poll1page1, poll1page2, poll2)
	withLogsTest(t, mock)

	waits := 0
	logsFollowWait = func(ctx context.Context, d time.Duration) bool {
		if d != 2*time.Second {
			t.Errorf("poll interval = %v, want 2s", d)
		}
		waits++
		return waits <= 2
	}

	stdout, code, err := executeWithExit("logs", "search", "--monitor", "nightly-job", "--follow")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	want := strings.Join([]string{
		"2026-10-29T03:00:10Z  INFO   nightly-job  run-123  A",
		"2026-10-29T03:00:12Z  WARN   nightly-job  run-123  B",
		"2026-10-29T03:00:13Z  INFO   nightly-job  run-123  C",
		"2026-10-29T03:00:15Z  ERROR  nightly-job  run-123  D",
		"2026-10-29T03:00:14Z  INFO   nightly-job  run-123  E",
		"2026-10-29T03:00:16Z  INFO   nightly-job  run-123  F",
	}, "\n") + "\n"
	if stdout != want {
		t.Errorf("follow output:\n%s\nwant:\n%s", stdout, want)
	}

	reqs := mock.AllRequests()
	if len(reqs) != 4 {
		t.Fatalf("requests = %d, want 4", len(reqs))
	}
	// Each poll starts a minute before the newest record printed.
	assertQuery(t, reqs[0], map[string]string{"monitor": "nightly-job"})
	assertQuery(t, reqs[1], map[string]string{"monitor": "nightly-job", "start": "2026-10-29T02:59:12.250000Z"})
	assertQuery(t, reqs[2], map[string]string{"monitor": "nightly-job", "start": "2026-10-29T02:59:12.250000Z", "before": "2026-10-29T03:00:15.000000Z"})
	assertQuery(t, reqs[3], map[string]string{"monitor": "nightly-job", "start": "2026-10-29T02:59:15.000000Z"})
}

func TestLogsSearch_FollowStartsNoEarlierThanFirstPageOrStart(t *testing.T) {
	record := func(stamp, body string) string {
		return `{"stamp": "` + stamp + `", "level": "info", "body": "` + body + `", "monitor": {"key": "nightly-job"}}`
	}
	cases := []struct {
		name      string
		args      []string
		initial   string
		wantStart string
	}{
		{
			// A first page with more behind it ends at its oldest record:
			// what came before it is history, not late arrivals.
			name:      "first page had more",
			args:      []string{"logs", "search", "--follow"},
			initial:   `{"data": [` + record("2026-10-29T03:05:12.000000Z", "B") + `, ` + record("2026-10-29T03:05:10.000000Z", "A") + `], "has_more": true, "next": "2026-10-29T03:05:10.000000Z"}`,
			wantStart: "2026-10-29T03:05:10.000000Z",
		},
		{
			name:      "since",
			args:      []string{"logs", "search", "--follow", "--since", "30s"},
			initial:   `{"data": [` + record("2026-10-29T03:05:12.000000Z", "B") + `, ` + record("2026-10-29T03:05:10.000000Z", "A") + `], "has_more": false, "next": null}`,
			wantStart: "2026-10-29T03:05:00.000000Z",
		},
		{
			// A --start the CLI cannot parse is passed through; the first
			// record printed, which the API returned for it, is the floor.
			name:      "start the CLI cannot parse",
			args:      []string{"logs", "search", "--follow", "--start", "2026-10-29T03:05"},
			initial:   `{"data": [` + record("2026-10-29T03:05:12.000000Z", "B") + `, ` + record("2026-10-29T03:05:10.000000Z", "A") + `], "has_more": false, "next": null}`,
			wantStart: "2026-10-29T03:05:10.000000Z",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := testutil.NewMockAPI()
			defer mock.Close()
			// The mock ignores start=, so the poll also returns a record
			// from before it, which must not print.
			poll := `{"data": [` + record("2026-10-29T03:05:13.000000Z", "C") + `, ` +
				record("2026-10-29T03:05:12.000000Z", "B") + `, ` +
				record("2026-10-29T03:05:10.000000Z", "A") + `, ` +
				record("2026-10-29T03:04:59.000000Z", "old") + `], "has_more": false, "next": null}`
			mock.OnSequence("GET", "/logs", 200, tc.initial, poll)
			withLogsTest(t, mock)
			waits := 0
			logsFollowWait = func(ctx context.Context, d time.Duration) bool {
				waits++
				return waits <= 1
			}

			stdout, code, err := executeWithExit(tc.args...)
			if err != nil || code != 0 {
				t.Fatalf("exit %d err %v\n%s", code, err, stdout)
			}
			want := strings.Join([]string{
				"2026-10-29T03:05:10Z  INFO   nightly-job  -  A",
				"2026-10-29T03:05:12Z  INFO   nightly-job  -  B",
				"2026-10-29T03:05:13Z  INFO   nightly-job  -  C",
			}, "\n") + "\n"
			if stdout != want {
				t.Errorf("follow output:\n%s\nwant:\n%s", stdout, want)
			}
			reqs := mock.AllRequests()
			if len(reqs) != 2 {
				t.Fatalf("requests = %d, want 2", len(reqs))
			}
			if got := reqs[1].QueryParams.Get("start"); got != tc.wantStart {
				t.Errorf("poll start = %q, want %q", got, tc.wantStart)
			}
		})
	}
}

func TestLogFollowerForgetsRecordsOutsideTheWindow(t *testing.T) {
	rec := func(stamp, body string) logRecord {
		return logRecord{LogEntry: lib.LogEntry{Stamp: stamp, Body: body, Series: "run-1", Monitor: &lib.ResourceRef{Key: "job"}}}
	}
	f := newLogFollower("")
	testutil.CaptureStdoutStderr(func() {
		f.print([]logRecord{rec("2026-10-29T03:00:00.000000Z", "old"), rec("2026-10-29T03:00:30.000000Z", "mid")}, false, false)
		if got := f.nextStart(); got != "2026-10-29T02:59:30.000000Z" {
			t.Errorf("start = %q", got)
		}
		if len(f.seen) != 2 {
			t.Errorf("both records are inside the window, seen = %v", f.seen)
		}
		f.print([]logRecord{rec("2026-10-29T03:01:10.000000Z", "new")}, false, false)
		if got := f.nextStart(); got != "2026-10-29T03:00:10.000000Z" {
			t.Errorf("start = %q", got)
		}
	})
	if len(f.seen) != 2 {
		t.Errorf("the record before the window should be forgotten, seen = %v", f.seen)
	}
	if _, ok := f.seen[logKey{stamp: "2026-10-29T03:00:30.000000Z", body: "mid", series: "run-1", monitor: "job"}]; !ok {
		t.Errorf("records inside the window are kept, seen = %v", f.seen)
	}
}

func TestLogsSearch_FollowJSONLines(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.OnSequence("GET", "/logs", 200,
		`{"data": [], "has_more": false, "next": null}`,
		`{"data": [{"stamp": "2026-10-29T03:06:00.000000Z", "level": "info", "body": "late", "extra_field": 1}], "has_more": false, "next": null}`,
	)
	withLogsTest(t, mock)
	waits := 0
	logsFollowWait = func(ctx context.Context, d time.Duration) bool {
		waits++
		return waits <= 1
	}

	stdout, code, err := executeWithExit("logs", "search", "--follow", "--json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	want := `{"stamp":"2026-10-29T03:06:00.000000Z","level":"info","body":"late","extra_field":1}` + "\n"
	if stdout != want {
		t.Errorf("JSON lines = %q, want %q", stdout, want)
	}
	// With no record yet, following starts from now, a minute back for
	// records that arrive late.
	reqs := mock.AllRequests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %d, want 2", len(reqs))
	}
	assertQuery(t, reqs[1], map[string]string{"start": "2026-10-29T03:04:30.000000Z"})
}

func TestLogsSearch_FollowKeepsPollingAfterServerErrors(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withLogsTest(t, mock)
	mock.OnSequence("GET", "/logs", 200, `{"data": [], "has_more": false, "next": null}`)
	waits := 0
	logsFollowWait = func(ctx context.Context, d time.Duration) bool {
		waits++
		if waits == 1 {
			// From the first poll on, the API is overloaded.
			mock.OnSequence("GET", "/logs", 503, `{"error": "unavailable"}`)
		}
		return waits <= 2
	}

	stdout, stderr, code, _ := executeWithExitStderr("logs", "search", "--follow")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, stdout)
	}
	if mock.RequestCount() != 3 || !strings.Contains(stderr, "API Error (503); retrying") {
		t.Errorf("requests %d, stderr %q", mock.RequestCount(), stderr)
	}
}

func TestLogsSearch_StripsTerminalControls(t *testing.T) {
	h := hostileText
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, `{"data": [{"stamp": "2026-10-29T03:00:10.000000Z", "level": "info`+h+`",
		"body": "ok `+h+`done\r\nnext\ttab\rcr", "monitor": {"key": "job`+h+`"}, "series": "run`+h+`",
		"attributes": {"k`+h+`": "v`+h+`", "q": "two words`+h+`", "n": {"nested": "x\u009b2Jy"}}}],
		"has_more": true, "next": "2026-10-29T03:00:10.000000Z`+h+`"}`)
	withLogsTest(t, mock)

	stdout, stderr, code, err := executeWithExitStderr("logs", "search", "--attributes")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, stdout)
	}
	assertNoTerminalControls(t, stdout)
	assertNoTerminalControls(t, stderr)
	// Line breaks in the body still print as \n and \r, as do the
	// sender's own: the CR that ends hostileText shows up as \r.
	want := "2026-10-29T03:00:10Z  INFO   job  run  ok \\rdone\\nnext\ttab\\rcr" +
		`  k=v n={"nested":"xy"} q="two words"` + "\n"
	if stdout != want {
		t.Errorf("stdout:\n%q\nwant:\n%q", stdout, want)
	}
	if !strings.Contains(stderr, "next: 2026-10-29T03:00:10.000000Z  (") {
		t.Errorf("stderr %q", stderr)
	}
}
