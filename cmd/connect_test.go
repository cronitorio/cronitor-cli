package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

type exitSentinel int

type recordedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

func withConnectTest(t *testing.T, mockURL string) func() {
	t.Helper()
	cleanup := setupIntegrationTest(mockURL)
	resetConnectFlags()
	resetIntegrationFlags()
	oldExit := exitFn
	oldSleep := sleepFn
	oldOpen := openBrowserFn
	oldSecret := readSecretFn
	oldNow := nowFn
	sleepFn = func(time.Duration) {}
	openBrowserFn = func(string) {}
	return func() {
		cleanup()
		resetConnectFlags()
		resetIntegrationFlags()
		exitFn = oldExit
		sleepFn = oldSleep
		openBrowserFn = oldOpen
		readSecretFn = oldSecret
		nowFn = oldNow
	}
}

func executeWithExit(args ...string) (output string, code int, err error) {
	oldExit := exitFn
	code = 0
	exitFn = func(c int) {
		panic(exitSentinel(c))
	}
	defer func() { exitFn = oldExit }()

	RootCmd.SetArgs(args)
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				if c, ok := rec.(exitSentinel); ok {
					code = int(c)
					return
				}
				panic(rec)
			}
		}()
		err = RootCmd.Execute()
	}()

	w.Close()
	os.Stdout = oldStdout
	output = <-done
	return output, code, err
}

func catalogueJSON() string {
	return `{
  "services": [
    {"service":"slack","service_name":"Slack","type":"oauth","method":"oauth","fields":{},"available":true},
    {"service":"pagerduty","service_name":"PagerDuty","type":"oauth","method":"oauth","fields":{},"available":true},
    {"service":"discord","service_name":"Discord","type":"webhook","method":"api_key","fields":{"url":{"label":"Webhook URL","secret":true,"required":true}},"available":true},
    {"service":"opsgenie","service_name":"Opsgenie","type":"api_key","method":"api_key","fields":{"api_key":{"label":"API Key","secret":true,"required":true}},"available":true},
    {"service":"webhook","service_name":"Webhook","type":"webhook","method":"api_key","fields":{"url":{"label":"URL","secret":true,"required":true}},"available":true},
    {"service":"telegram","service_name":"Telegram","type":"telegram","method":"telegram","fields":{},"available":true,"instructions":"Message the Cronitor Telegram bot to finish connecting."}
  ]
}`
}

func TestConnect_ServicesTable(t *testing.T) {
	var mu sync.Mutex
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(body)})
		mu.Unlock()
		if r.Method == "GET" && r.URL.Path == "/integrations/services" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	for _, header := range []string{"SERVICE", "METHOD", "AVAILABLE"} {
		if !strings.Contains(output, header) {
			t.Errorf("expected table header %q in output, got:\n%s", header, output)
		}
	}
	for _, service := range []string{"slack", "pagerduty", "opsgenie", "telegram"} {
		if !strings.Contains(output, service) {
			t.Errorf("expected service %q in output, got:\n%s", service, output)
		}
	}
	if !strings.Contains(output, "oauth") {
		t.Errorf("expected method oauth in output, got:\n%s", output)
	}
}

func TestConnect_APIKey_FieldFlag_SecretsAbsentFromStdout(t *testing.T) {
	const secret = "field-secret-value-do-not-print"
	var createBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			createBody = string(body)
			w.WriteHeader(201)
			fmtWrite(w, `{"id":"opsgenie:9","service":"opsgenie","name":"On-call","label":"On-call","method":"api_key","available":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "opsgenie", "--name", "On-call", "--field", "api_key="+secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "opsgenie:9") || !strings.Contains(output, "On-call") {
		t.Errorf("expected id and label in output, got:\n%s", output)
	}
	if strings.Contains(output, secret) {
		t.Errorf("secret leaked to stdout:\n%s", output)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(createBody), &payload); err != nil {
		t.Fatalf("create body is not JSON: %s", createBody)
	}
	if payload["service"] != "opsgenie" {
		t.Errorf("expected service opsgenie, got %#v", payload["service"])
	}
	if payload["name"] != "On-call" {
		t.Errorf("expected name On-call, got %#v", payload["name"])
	}
	fields, _ := payload["fields"].(map[string]interface{})
	if fields["api_key"] != secret {
		t.Errorf("expected fields.api_key in request, got %#v", payload["fields"])
	}
}

func TestConnect_APIKey_PromptsForCatalogueFields(t *testing.T) {
	const secret = "prompted-secret-value-do-not-print"
	var prompts []string
	var createBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			createBody = string(body)
			w.WriteHeader(201)
			fmtWrite(w, `{"id":"opsgenie:11","service":"opsgenie","label":"Opsgenie","name":"Opsgenie"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()
	readSecretFn = func(prompt string) (string, error) {
		prompts = append(prompts, prompt)
		return secret, nil
	}

	output, code, err := executeWithExit("connect", "opsgenie")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if len(prompts) == 0 {
		t.Fatal("expected a hidden prompt for catalogue fields")
	}
	if !strings.Contains(strings.Join(prompts, "\n"), "API Key") {
		t.Errorf("expected prompt to mention API Key, got %#v", prompts)
	}
	if strings.Contains(output, secret) {
		t.Errorf("prompted secret leaked to stdout:\n%s", output)
	}
	if !strings.Contains(createBody, secret) {
		t.Errorf("expected prompted secret in request body, got %s", createBody)
	}
}

func TestConnect_Slack_TwoPendingThenComplete(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	var startBody string
	opened := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations/connect":
			startBody = string(body)
			w.WriteHeader(200)
			fmtWrite(w, `{"authorize_url":"https://slack.example/oauth","token":"tok_123","expires_at":"2099-01-01T00:00:00Z","poll_interval":1}`)
		case r.Method == "GET" && r.URL.Path == "/integrations/connect/tok_123":
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			w.WriteHeader(200)
			if n <= 2 {
				fmtWrite(w, `{"status":"pending"}`)
				return
			}
			fmtWrite(w, `{"status":"complete","id":"slack:12","service":"slack","label":"Workspace","name":"Workspace"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()
	openBrowserFn = func(url string) {
		opened = append(opened, url)
	}

	output, code, err := executeWithExit("connect", "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "https://slack.example/oauth") {
		t.Errorf("expected authorize URL to be printed first, got:\n%s", output)
	}
	if !strings.Contains(output, "slack:12") || !strings.Contains(output, "Workspace") {
		t.Errorf("expected id and label after complete, got:\n%s", output)
	}
	if polls != 3 {
		t.Errorf("expected 3 status polls (2 pending + complete), got %d", polls)
	}
	if len(opened) != 1 || opened[0] != "https://slack.example/oauth" {
		t.Errorf("expected browser to open authorize URL, got %#v", opened)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(startBody), &payload); err != nil {
		t.Fatalf("start body is not JSON: %s", startBody)
	}
	if payload["service"] != "slack" {
		t.Errorf("expected start body service=slack, got %#v", payload)
	}
}

func TestConnect_TimeoutExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations/connect":
			w.WriteHeader(200)
			fmtWrite(w, `{"authorize_url":"https://slack.example/oauth","token":"tok_timeout","expires_at":"2099-01-01T00:00:00Z","poll_interval":1}`)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/integrations/connect/"):
			w.WriteHeader(200)
			fmtWrite(w, `{"status":"pending"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()
	// Use real sleeps so the short timeout can elapse.
	sleepFn = time.Sleep

	output, code, err := executeWithExit("connect", "slack", "--no-browser", "--timeout", "300ms")
	if err != nil && code == 0 {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected timeout exit 1, got %d\n%s", code, output)
	}
	if !strings.Contains(strings.ToLower(output), "timed out") {
		t.Errorf("expected timeout message, got:\n%s", output)
	}
}

func TestConnect_AddTo_AmbiguousLabelRetry(t *testing.T) {
	var mu sync.Mutex
	var putBodies []string
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations/connect":
			w.WriteHeader(200)
			fmtWrite(w, `{"authorize_url":"https://slack.example/oauth","token":"tok_add","expires_at":"2099-01-01T00:00:00Z","poll_interval":1}`)
		case r.Method == "GET" && r.URL.Path == "/integrations/connect/tok_add":
			w.WriteHeader(200)
			fmtWrite(w, `{"status":"complete","id":"slack:12","service":"slack","label":"Workspace"}`)
		case r.Method == "GET" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"key":"default","name":"Default","notifications":{"slack":["#existing"]}}`)
		case r.Method == "PUT" && r.URL.Path == "/notifications/default":
			mu.Lock()
			putCount++
			n := putCount
			putBodies = append(putBodies, string(body))
			mu.Unlock()
			if n == 1 {
				w.WriteHeader(400)
				fmtWrite(w, `{"error":"ambiguous label"}`)
				return
			}
			w.WriteHeader(200)
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "slack", "--no-browser", "--add-to", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if putCount != 2 {
		t.Fatalf("expected 2 PUTs (label then id retry), got %d", putCount)
	}

	var first map[string]interface{}
	if err := json.Unmarshal([]byte(putBodies[0]), &first); err != nil {
		t.Fatalf("first PUT not JSON: %s", putBodies[0])
	}
	firstSlack := toStringSlice(first["notifications"].(map[string]interface{})["slack"])
	if !containsString(firstSlack, "Workspace") {
		t.Errorf("first PUT should append label Workspace, got %#v", firstSlack)
	}

	var second map[string]interface{}
	if err := json.Unmarshal([]byte(putBodies[1]), &second); err != nil {
		t.Fatalf("second PUT not JSON: %s", putBodies[1])
	}
	secondSlack := toStringSlice(second["notifications"].(map[string]interface{})["slack"])
	if !containsString(secondSlack, "slack:12") {
		t.Errorf("retry PUT should use service:pk id, got %#v", secondSlack)
	}
	if containsString(secondSlack, "Workspace") {
		t.Errorf("retry PUT should replace the ambiguous label, got %#v", secondSlack)
	}
	if !strings.Contains(output, "default") {
		t.Errorf("expected add-to confirmation, got:\n%s", output)
	}
}

func TestConnect_Telegram_MatchByIDDiff(t *testing.T) {
	var mu sync.Mutex
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "GET" && r.URL.Path == "/integrations":
			if r.URL.Query().Get("service") != "telegram" {
				t.Errorf("expected service=telegram query, got %q", r.URL.RawQuery)
			}
			mu.Lock()
			listCalls++
			n := listCalls
			mu.Unlock()
			w.WriteHeader(200)
			if n == 1 {
				fmtWrite(w, `{"integrations":[{"id":"telegram:1","service":"telegram","name":"Existing","label":"Existing"}]}`)
				return
			}
			fmtWrite(w, `{"integrations":[{"id":"telegram:1","service":"telegram","name":"Existing","label":"Existing"},{"id":"telegram:2","service":"telegram","name":"New Bot","label":"New Bot"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "telegram")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "Cronitor Telegram bot") && !strings.Contains(output, "Message the Cronitor Telegram bot") {
		t.Errorf("expected Telegram bot instructions, got:\n%s", output)
	}
	if !strings.Contains(output, "telegram:2") || !strings.Contains(output, "New Bot") {
		t.Errorf("expected new integration from id-diff, got:\n%s", output)
	}
	if strings.Contains(output, "telegram:1") && !strings.Contains(output, "telegram:2") {
		t.Errorf("matched the pre-existing integration instead of the new one:\n%s", output)
	}
}

func TestConnect_Telegram_MatchByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "GET" && r.URL.Path == "/integrations":
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"id":"telegram:7","service":"telegram","name":"On-call bot","label":"On-call bot"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "telegram", "--name", "On-call bot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "telegram:7") || !strings.Contains(output, "On-call bot") {
		t.Errorf("expected --name match, got:\n%s", output)
	}
}

func TestConnect_NoBrowserSkipsOpen(t *testing.T) {
	opened := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations/connect":
			w.WriteHeader(200)
			fmtWrite(w, `{"authorize_url":"https://slack.example/oauth","token":"tok_nb","poll_interval":1}`)
		case r.Method == "GET" && r.URL.Path == "/integrations/connect/tok_nb":
			w.WriteHeader(200)
			fmtWrite(w, `{"status":"complete","id":"slack:1","label":"A"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()
	openBrowserFn = func(string) { opened++ }

	output, code, err := executeWithExit("connect", "slack", "--no-browser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if opened != 0 {
		t.Errorf("expected --no-browser to skip opener, opened=%d", opened)
	}
	if !strings.Contains(output, "https://slack.example/oauth") {
		t.Errorf("expected URL to still be printed, got:\n%s", output)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func fmtWrite(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, body)
}
