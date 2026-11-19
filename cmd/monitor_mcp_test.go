package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const mcpSavedJSON = `{"key":"docs-mcp","request":{"url":"https://mcp.example.com/mcp"},"mcp":{"auth":"%s","headers":%s,"tools":null,"protocol_version":"2025-11-25","baseline_tools":["fetch","search"],"oauth":%s}}`

func withMcpTest(t *testing.T, handler http.HandlerFunc) (*[]string, func()) {
	t.Helper()
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(body))
		if v := r.Header.Get("Cronitor-Version"); v != "2025-11-28" {
			t.Errorf("Cronitor-Version = %q", v)
		}
		handler(w, r)
	}))
	cleanup := withConnectTest(t, server.URL)
	resetMcpFlags()
	return &calls, func() {
		resetMcpFlags()
		cleanup()
		server.Close()
	}
}

func TestMonitorMcp_HeaderCreate_HidesValues(t *testing.T) {
	calls, cleanup := withMcpTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /monitors/docs-mcp":
			http.NotFound(w, r)
		case "POST /monitors":
			w.WriteHeader(201)
			fmtWrite(w, fmt.Sprintf(mcpSavedJSON, "headers", `["x-api-key"]`, "null"))
		}
	})
	defer cleanup()
	verbose = true

	output, code, err := executeWithExit("monitor", "mcp", "https://mcp.example.com/mcp", "--key", "docs-mcp",
		"--header", "X-Api-Key: s3cret-value")
	if err != nil || code != 0 {
		t.Fatalf("exit %d, err %v\n%s", code, err, output)
	}
	if strings.Contains(output, "s3cret-value") {
		t.Errorf("header value printed:\n%s", output)
	}
	for _, want := range []string{"docs-mcp", "2025-11-25", "fetch, search", "x-api-key"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	var sent map[string]interface{}
	json.Unmarshal([]byte(strings.SplitN((*calls)[1], " ", 3)[2]), &sent)
	headers := sent["mcp"].(map[string]interface{})["headers"].(map[string]interface{})
	if sent["type"] != "check" || sent["platform"] != "mcp" || headers["X-Api-Key"] != "s3cret-value" {
		t.Errorf("unexpected create body: %v", sent)
	}
}

func TestMonitorMcp_OAuth_StartPendingReadyCreate(t *testing.T) {
	polls := 0
	var opened []string
	calls, cleanup := withMcpTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /monitors/mcp/oauth/start":
			fmtWrite(w, `{"authorize_url":"https://auth.example.com/authorize?state=x","connection":"conn_1","expires_in":900}`)
		case "GET /monitors/mcp/oauth/connections/conn_1":
			polls++
			if polls == 1 {
				fmtWrite(w, `{"status":"pending","error":null,"issuer":null}`)
				return
			}
			fmtWrite(w, `{"status":"ready","error":null,"issuer":"https://auth.example.com"}`)
		case "POST /monitors":
			w.WriteHeader(201)
			fmtWrite(w, fmt.Sprintf(mcpSavedJSON, "oauth", "[]", `{"issuer":"https://auth.example.com"}`))
		}
	})
	defer cleanup()
	openBrowserFn = func(u string) { opened = append(opened, u) }

	output, code, err := executeWithExit("monitor", "mcp", "https://mcp.example.com/mcp", "--oauth")
	if err != nil || code != 0 {
		t.Fatalf("exit %d, err %v\n%s", code, err, output)
	}
	if !strings.Contains(output, "Open this URL to sign in:") || len(opened) != 1 || polls != 2 {
		t.Errorf("opened %v, polls %d:\n%s", opened, polls, output)
	}
	last := (*calls)[len(*calls)-1]
	if !strings.HasPrefix(last, "POST /monitors ") || !strings.Contains(last, `"oauth_connection":"conn_1"`) {
		t.Errorf("expected create with the connection, got %s", last)
	}
	if !strings.Contains((*calls)[0], `{"url":"https://mcp.example.com/mcp"}`) {
		t.Errorf("unexpected start body: %s", (*calls)[0])
	}
}

func TestMonitorMcp_OAuthFailed_ExitsNonZero(t *testing.T) {
	calls, cleanup := withMcpTest(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /monitors/mcp/oauth/start":
			fmtWrite(w, `{"authorize_url":"https://auth.example.com/authorize","connection":"conn_1","expires_in":900}`)
		case "GET /monitors/mcp/oauth/connections/conn_1":
			fmtWrite(w, `{"status":"failed","error":"access_denied","issuer":null}`)
		}
	})
	defer cleanup()

	output, code, _ := executeWithExit("monitor", "mcp", "https://mcp.example.com/mcp", "--oauth")
	if code != 1 || !strings.Contains(output, "Sign-in was denied or cancelled.") {
		t.Errorf("exit %d:\n%s", code, output)
	}
	if len(*calls) != 2 {
		t.Errorf("expected no monitor write, got %v", *calls)
	}
}

func TestMonitorMcp_HeaderAndOAuthAreExclusive(t *testing.T) {
	calls, cleanup := withMcpTest(t, func(w http.ResponseWriter, r *http.Request) {})
	defer cleanup()

	output, code, _ := executeWithExit("monitor", "mcp", "https://mcp.example.com/mcp", "--oauth", "--header", "X-Key: v")
	if code != 1 || !strings.Contains(output, "Use --header or --oauth, not both") || len(*calls) != 0 {
		t.Errorf("exit %d, calls %v:\n%s", code, *calls, output)
	}
}
