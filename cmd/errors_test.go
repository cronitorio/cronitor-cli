package cmd

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/spf13/viper"
)

var readAPITestNow = time.Date(2026, 10, 29, 3, 5, 30, 0, time.UTC)

const testGroupID = "a95936e78b4b51bf4abdbb64d45624ca804c2f4a"

// withReadAPITest points the CLI at mock, fixes the clock at readAPITestNow
// and resets the read command flags before and after the test.
func withReadAPITest(t *testing.T, mock *testutil.MockAPI) {
	t.Helper()
	cleanup := setupIntegrationTest(mock.Server.URL)
	oldNow := nowFn
	nowFn = func() time.Time { return readAPITestNow }
	resetErrorsFlags()
	t.Cleanup(func() {
		cleanup()
		nowFn = oldNow
		resetErrorsFlags()
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
	for _, name := range []string{"list"} {
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

	for _, args := range [][]string{{"errors", "list"}} {
		_, _, err := executeWithExit(args...)
		if err == nil || !strings.Contains(err.Error(), "API key required") {
			t.Errorf("%v: err = %v, want the API key error", args, err)
		}
	}
	if mock.RequestCount() != 0 {
		t.Errorf("no request should be sent without a key, got %d", mock.RequestCount())
	}
}
