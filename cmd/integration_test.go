package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// setupIntegrationTest configures the test environment to point at a mock server.
// It returns a cleanup function that restores the original state.
func setupIntegrationTest(mockURL string) func() {
	oldBaseURL := lib.BaseURLOverride
	oldAPIKey := viper.GetString("CRONITOR_API_KEY")

	lib.BaseURLOverride = mockURL
	viper.Set("CRONITOR_API_KEY", "test-api-key-1234567890")
	viper.Set("CRONITOR_API_VERSION", "")

	return func() {
		lib.BaseURLOverride = oldBaseURL
		viper.Set("CRONITOR_API_KEY", oldAPIKey)
		viper.Set("CRONITOR_API_VERSION", "")
	}
}

// executeCmd runs a command through the root cobra command and captures stdout.
func executeCmd(args ...string) (string, error) {
	RootCmd.SetArgs(args)
	var execErr error
	output := testutil.CaptureStdout(func() {
		execErr = RootCmd.Execute()
	})
	return output, execErr
}

// --- Step 7: Table Output Tests ---

func TestIntegration_MonitorList_TableOutput(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("monitors_list.json")
	mock.On("GET", "/monitors", 200, fixture)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	// Reset flag state
	monitorFormat = ""
	monitorOutput = ""
	monitorFetchAll = false
	monitorPage = 1

	output, err := executeCmd("monitor", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify table headers
	for _, header := range []string{"NAME", "KEY", "TYPE", "STATUS"} {
		if !strings.Contains(output, header) {
			t.Errorf("expected table header %q in output, got:\n%s", header, output)
		}
	}

	// Verify monitor data from fixture
	for _, name := range []string{"Nightly Backup", "Health Check", "Paused Monitor"} {
		if !strings.Contains(output, name) {
			t.Errorf("expected monitor name %q in output, got:\n%s", name, output)
		}
	}
	for _, key := range []string{"abc123", "def456", "ghi789"} {
		if !strings.Contains(output, key) {
			t.Errorf("expected monitor key %q in output, got:\n%s", key, output)
		}
	}
}

func TestIntegration_IssueList_TableOutput(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("issues_list.json")
	mock.On("GET", "/issues", 200, fixture)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	// Reset flag state
	issueFormat = ""
	issueOutput = ""
	issueFetchAll = false
	issuePage = 1

	output, err := executeCmd("issue", "list", "--format", "table")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify table headers
	for _, header := range []string{"NAME", "KEY", "STATE", "SEVERITY"} {
		if !strings.Contains(output, header) {
			t.Errorf("expected table header %q in output, got:\n%s", header, output)
		}
	}

	// Verify issue data from fixture
	if !strings.Contains(output, "issue-001") {
		t.Errorf("expected issue key 'issue-001' in output")
	}
	if !strings.Contains(output, "issue-002") {
		t.Errorf("expected issue key 'issue-002' in output")
	}
}

// --- Step 8: JSON Output Tests ---

func TestIntegration_MonitorList_JSONOutput(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("monitors_list.json")
	mock.On("GET", "/monitors", 200, fixture)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	monitorFormat = ""
	monitorOutput = ""
	monitorFetchAll = false
	monitorPage = 1

	output, err := executeCmd("monitor", "list", "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid JSON output, got:\n%s", output)
	}

	// Verify it contains expected keys from fixture
	if !strings.Contains(trimmed, "abc123") {
		t.Error("expected JSON to contain monitor key 'abc123'")
	}
	if !strings.Contains(trimmed, "Nightly Backup") {
		t.Error("expected JSON to contain monitor name 'Nightly Backup'")
	}
}

func TestIntegration_MonitorGet_JSONOutput(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("monitor_get.json")
	mock.On("GET", "/monitors/my-job", 200, fixture)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	monitorFormat = ""
	monitorOutput = ""

	output, err := executeCmd("monitor", "get", "my-job")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid pretty-printed JSON output, got:\n%s", output)
	}

	// Should be indented (pretty-printed)
	if !strings.Contains(trimmed, "\n") {
		t.Error("expected pretty-printed JSON (multi-line)")
	}
}

// --- Step 9: YAML Output Test ---

func TestIntegration_MonitorList_YAMLOutput(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	yamlBody := "---\nmonitors:\n- key: abc123\n  name: Nightly Backup\n"
	mock.On("GET", "/monitors", 200, yamlBody)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	monitorFormat = ""
	monitorOutput = ""
	monitorFetchAll = false
	monitorPage = 1

	output, err := executeCmd("monitor", "list", "--format", "yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		t.Error("expected non-empty YAML output")
	}
	// Should be a passthrough of the mock body
	if !strings.Contains(trimmed, "abc123") {
		t.Error("expected YAML output to contain monitor key")
	}

	// Verify the format=yaml query param was sent
	req := mock.LastRequest()
	if req.QueryParams.Get("format") != "yaml" {
		t.Errorf("expected format=yaml query param, got %q", req.QueryParams.Get("format"))
	}
}

// --- Step 10: Output to File Test ---

func TestIntegration_MonitorList_OutputToFile(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("monitors_list.json")
	mock.On("GET", "/monitors", 200, fixture)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "output.json")

	monitorFormat = ""
	monitorOutput = ""
	monitorFetchAll = false
	monitorPage = 1

	output, err := executeCmd("monitor", "list", "--format", "json", "--output", outFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should exist with valid JSON
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected output file to exist: %v", err)
	}
	trimmedFile := strings.TrimSpace(string(data))
	if !json.Valid([]byte(trimmedFile)) {
		t.Errorf("expected file to contain valid JSON, got:\n%s", trimmedFile)
	}
	if !strings.Contains(trimmedFile, "abc123") {
		t.Error("expected file to contain monitor key 'abc123'")
	}

	// Stdout should mention file, not contain the JSON data
	if !strings.Contains(output, "Output written to") {
		t.Errorf("expected stdout to contain 'Output written to', got:\n%s", output)
	}
	// Stdout should NOT contain the JSON data itself
	if strings.Contains(output, `"abc123"`) {
		t.Error("expected stdout to NOT contain JSON data when writing to file")
	}
}

// --- Step 11: Pagination Metadata Test ---

func TestIntegration_MonitorList_PaginationMetadata(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("monitors_list.json")
	mock.On("GET", "/monitors", 200, fixture)

	cleanup := setupIntegrationTest(mock.Server.URL)
	defer cleanup()

	monitorFormat = ""
	monitorOutput = ""
	monitorFetchAll = false
	monitorPage = 1

	output, err := executeCmd("monitor", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The fixture has page_info.totalMonitorCount = 3
	// Should show pagination info
	if !strings.Contains(output, "Showing page 1") {
		t.Errorf("expected pagination metadata 'Showing page 1' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "3 monitors total") {
		t.Errorf("expected '3 monitors total' in output, got:\n%s", output)
	}
}

// --- Step 12: --all Flag Test ---

func TestIntegration_MonitorList_AllFlag(t *testing.T) {
	// Custom server that returns different items per page
	page1 := `{"monitors":[{"key":"mon-1","name":"Monitor 1","type":"job","passing":true,"paused":false}],"page_info":{"page":1,"pageSize":1,"totalMonitorCount":2}}`
	page2 := `{"monitors":[{"key":"mon-2","name":"Monitor 2","type":"check","passing":true,"paused":false}],"page_info":{"page":2,"pageSize":1,"totalMonitorCount":2}}`
	page3 := `{"monitors":[],"page_info":{"page":3,"pageSize":1,"totalMonitorCount":2}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		page := r.URL.Query().Get("page")
		switch page {
		case "1", "":
			w.WriteHeader(200)
			fmt.Fprint(w, page1)
		case "2":
			w.WriteHeader(200)
			fmt.Fprint(w, page2)
		default:
			w.WriteHeader(200)
			fmt.Fprint(w, page3)
		}
	}))
	defer server.Close()

	cleanup := setupIntegrationTest(server.URL)
	defer cleanup()

	monitorFormat = ""
	monitorOutput = ""
	monitorFetchAll = false
	monitorPage = 1

	output, err := executeCmd("monitor", "list", "--all", "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) {
		t.Errorf("expected valid JSON output, got:\n%s", output)
	}

	// Should be a merged array with items from both pages
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		t.Fatalf("expected JSON array, got parse error: %v\noutput:\n%s", err, trimmed)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items from merged pages, got %d", len(items))
	}

	// Verify both monitors are present
	if !strings.Contains(trimmed, "mon-1") {
		t.Error("expected merged output to contain 'mon-1'")
	}
	if !strings.Contains(trimmed, "mon-2") {
		t.Error("expected merged output to contain 'mon-2'")
	}
}
