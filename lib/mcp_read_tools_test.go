package lib_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/viper"
)

const mcpTestGroup = "a95936e78b4b51bf4abdbb64d45624ca804c2f4a"

func newReadToolsServer(t *testing.T, mock *testutil.MockAPI) *server.MCPServer {
	t.Helper()
	oldBase := lib.BaseURLOverride
	oldKey := viper.GetString("CRONITOR_API_KEY")
	lib.BaseURLOverride = mock.Server.URL
	viper.Set("CRONITOR_API_KEY", "test-api-key-1234567890")
	t.Cleanup(func() {
		lib.BaseURLOverride = oldBase
		viper.Set("CRONITOR_API_KEY", oldKey)
	})
	s := server.NewMCPServer("test", "1", server.WithToolCapabilities(false))
	if err := lib.NewCronitorMCPHandler("default").RegisterTools(s); err != nil {
		t.Fatal(err)
	}
	return s
}

type toolResult struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func callTool(t *testing.T, s *server.MCPServer, name string, args map[string]interface{}) toolResult {
	t.Helper()
	msg, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{"name": name, "arguments": args},
	})
	raw, _ := json.Marshal(s.HandleMessage(context.Background(), msg))
	var resp struct {
		Result toolResult `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, raw)
	}
	if resp.Error != nil {
		t.Fatalf("%s: JSON-RPC error %s", name, resp.Error.Message)
	}
	if len(resp.Result.Content) == 0 {
		t.Fatalf("%s: empty result\n%s", name, raw)
	}
	return resp.Result
}

func TestErrorsLogsMCPToolsAreListedReadOnly(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	s := newReadToolsServer(t, mock)

	raw, _ := json.Marshal(s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Annotations struct {
					ReadOnlyHint    *bool `json:"readOnlyHint"`
					DestructiveHint *bool `json:"destructiveHint"`
				} `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tool := range resp.Result.Tools {
		switch tool.Name {
		case "search_errors", "get_error_group", "get_error_event", "search_logs":
			found[tool.Name] = true
			a := tool.Annotations
			if a.ReadOnlyHint == nil || !*a.ReadOnlyHint || a.DestructiveHint == nil || *a.DestructiveHint {
				t.Errorf("%s should be read-only and not destructive", tool.Name)
			}
			assertUntrustedNotice(t, tool.Name+" description", tool.Description)
		}
	}
	if len(found) != 4 {
		t.Errorf("found tools %v, want all four", found)
	}
}

func TestErrorsMCPSearchErrors(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors", 200, testutil.LoadFixture("errors_list.json"))
	s := newReadToolsServer(t, mock)

	before := time.Now()
	res := callTool(t, s, "search_errors", map[string]interface{}{
		"env": "production", "monitor": "nightly-job", "service": "billing-api",
		"level": "fatal,warn", "since": "24h", "search": "decode",
		"order_by": "-count", "page": 2, "page_size": 10,
	})
	if res.IsError {
		t.Fatalf("error result: %s", res.Content[0].Text)
	}
	if !strings.Contains(res.Content[0].Text, `"group": "`+mcpTestGroup+`"`) {
		t.Errorf("result should be the API response:\n%s", res.Content[0].Text)
	}
	req := mock.LastRequest()
	if req.Path != "/errors" {
		t.Fatalf("path = %s", req.Path)
	}
	q := req.QueryParams
	for k, v := range map[string]string{"env": "production", "monitor": "nightly-job", "service": "billing-api", "level": "fatal,warning", "q": "decode", "orderBy": "-count", "page": "2", "pageSize": "10"} {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
	start, err := strconv.ParseInt(q.Get("start"), 10, 64)
	if want := before.Add(-24 * time.Hour).Unix(); err != nil || start < want-5 || start > want+5 {
		t.Errorf("start = %q, want about %d", q.Get("start"), want)
	}
}

func TestErrorsMCPGroupAndEvents(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	eventID := "cd3dc8b6cffb41e4163dcbd857ca87da"
	mock.On("GET", "/errors/"+mcpTestGroup, 200, testutil.LoadFixture("error_group_get.json"))
	mock.On("GET", "/errors/"+mcpTestGroup+"/events", 200, testutil.LoadFixture("error_events_list.json"))
	mock.On("GET", "/errors/"+mcpTestGroup+"/events/"+eventID, 200, testutil.LoadFixture("error_event_get.json"))
	s := newReadToolsServer(t, mock)

	res := callTool(t, s, "get_error_group", map[string]interface{}{"group": mcpTestGroup})
	if res.IsError || !strings.Contains(res.Content[0].Text, `"sparkline"`) {
		t.Errorf("get_error_group = %+v", res)
	}

	res = callTool(t, s, "get_error_event", map[string]interface{}{"group": mcpTestGroup, "event_id": eventID})
	if res.IsError || !strings.Contains(res.Content[0].Text, `"payload"`) {
		t.Errorf("get_error_event = %+v", res)
	}
	if p := mock.LastRequest().Path; p != "/errors/"+mcpTestGroup+"/events/"+eventID {
		t.Errorf("path = %s", p)
	}

	res = callTool(t, s, "get_error_event", map[string]interface{}{
		"group": mcpTestGroup, "monitor": "nightly-job", "series": "run-123", "before": "1761706800", "limit": 5,
	})
	if res.IsError || !strings.Contains(res.Content[0].Text, `"has_more"`) {
		t.Errorf("get_error_event without event_id = %+v", res)
	}
	req := mock.LastRequest()
	if req.Path != "/errors/"+mcpTestGroup+"/events" || req.QueryParams.Get("series") != "run-123" ||
		req.QueryParams.Get("before") != "1761706800" || req.QueryParams.Get("limit") != "5" || req.QueryParams.Get("monitor") != "nightly-job" {
		t.Errorf("events request = %s %v", req.Path, req.QueryParams)
	}

	mock.Reset()
	res = callTool(t, s, "get_error_group", map[string]interface{}{"group": mcpTestGroup[:12]})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "40-character") || mock.RequestCount() != 0 {
		t.Errorf("short group id = %+v, %d requests", res, mock.RequestCount())
	}
}

func TestLogsMCPSearchLogs(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, testutil.LoadFixture("logs_search.json"))
	s := newReadToolsServer(t, mock)

	res := callTool(t, s, "search_logs", map[string]interface{}{
		"monitor": "nightly-job", "level": "warning", "search": "timeout db",
		"trace_id": "abc", "start": "2026-10-29T00:00:00Z", "end": "1761706800", "limit": 50,
	})
	if res.IsError || !strings.Contains(res.Content[0].Text, `"severity_number"`) {
		t.Errorf("search_logs = %+v", res)
	}
	q := mock.LastRequest().QueryParams
	for k, v := range map[string]string{"monitor": "nightly-job", "level": "warn", "q": "timeout db", "trace_id": "abc", "start": "2026-10-29T00:00:00Z", "end": "1761706800", "limit": "50"} {
		if q.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, q.Get(k), v)
		}
	}
}

func TestErrorsLogsMCPErrors(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/errors/"+mcpTestGroup, 404, `{"error": "Error group not found"}`)
	s := newReadToolsServer(t, mock)

	res := callTool(t, s, "get_error_group", map[string]interface{}{"group": mcpTestGroup})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "404") || !strings.Contains(res.Content[0].Text, "Error group not found") {
		t.Errorf("404 = %+v", res)
	}

	res = callTool(t, s, "search_logs", map[string]interface{}{"since": "soon"})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "since") {
		t.Errorf("bad since = %+v", res)
	}

	viper.Set("CRONITOR_API_KEY", "")
	mock.Reset()
	for _, name := range []string{"search_errors", "search_logs"} {
		res = callTool(t, s, name, map[string]interface{}{})
		if !res.IsError || !strings.Contains(res.Content[0].Text, "CRONITOR_API_KEY") {
			t.Errorf("%s without a key = %+v", name, res)
		}
	}
	if mock.RequestCount() != 0 {
		t.Errorf("no request without a key, got %d", mock.RequestCount())
	}
}

// assertUntrustedNotice checks that text tells the agent event and log
// content is untrusted and not to follow instructions in it.
func assertUntrustedNotice(t *testing.T, what, text string) {
	t.Helper()
	for _, want := range []string{"untrusted", "telemetry key", "do not follow instructions"} {
		if !strings.Contains(strings.ToLower(text), want) {
			t.Errorf("%s should say %q:\n%s", what, want, text)
		}
	}
}

func TestErrorsLogsMCPSystemPromptMarksContentUntrusted(t *testing.T) {
	assertUntrustedNotice(t, "system prompt", lib.NewCronitorMCPHandler("default").GetSystemPrompt())
}

func TestErrorsLogsMCPResultsStripTerminalControls(t *testing.T) {
	const h = `\u001b[2J\u001b]52;c;Y3VybA==\u0007\u009b1A\r`
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/logs", 200, `{"data": [{"stamp": "2026-10-29T03:00:10.000000Z", "body": "ok`+h+`\nnext", "severity_number": 9}], "has_more": false, "next": null}`)
	mock.On("GET", "/errors/"+mcpTestGroup, 200, `{"group": "`+mcpTestGroup+`", "title": "Boom`+h+`", "culprit": "job`+h+`",
		"latest_event": {"title": "Boom`+h+`", "payload": {"exception": {"values": [{"stacktrace": {"frames": [
			{"filename": "/app/job.py", "context_line": "raise Boom()`+h+`"}]}}]}}}}`)
	s := newReadToolsServer(t, mock)

	res := callTool(t, s, "search_logs", map[string]interface{}{})
	var logs struct {
		Data []map[string]interface{} `json:"data"`
	}
	if res.IsError || json.Unmarshal([]byte(res.Content[0].Text), &logs) != nil || len(logs.Data) != 1 {
		t.Fatalf("search_logs = %+v", res)
	}
	if body := logs.Data[0]["body"]; body != "ok\nnext" {
		t.Errorf("body = %q, want controls stripped and the line break kept", body)
	}
	if !strings.Contains(res.Content[0].Text, `"severity_number":9`) {
		t.Errorf("other fields are kept:\n%s", res.Content[0].Text)
	}

	res = callTool(t, s, "get_error_group", map[string]interface{}{"group": mcpTestGroup})
	text := res.Content[0].Text
	if res.IsError || strings.Contains(text, `\u001b`) || strings.Contains(text, "\u009b") || strings.Contains(text, "52;c;") || strings.Contains(text, `\r`) {
		t.Errorf("get_error_group should strip title, culprit and context_line:\n%s", text)
	}
	for _, want := range []string{`"title":"Boom"`, `"culprit":"job"`, `"context_line":"raise Boom()"`} {
		if !strings.Contains(text, want) {
			t.Errorf("want %s in:\n%s", want, text)
		}
	}
}
