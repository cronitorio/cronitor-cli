package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
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
	oldLine := readLineFn
	oldNow := nowFn
	sleepFn = func(time.Duration) {}
	readLineFn = func(string) (string, error) { return "", nil }
	openBrowserFn = func(string) {}
	resetSecretRedaction()
	verbose = false
	viper.Set("CRONITOR_LOG", "")
	return func() {
		cleanup()
		resetConnectFlags()
		resetIntegrationFlags()
		resetSecretRedaction()
		exitFn = oldExit
		sleepFn = oldSleep
		openBrowserFn = oldOpen
		readSecretFn = oldSecret
		readLineFn = oldLine
		nowFn = oldNow
		verbose = false
		viper.Set("CRONITOR_LOG", "")
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
    {"service":"discord","service_name":"Discord","type":"Messaging","method":"apikey","fields":{"key":"Webhook URL"},"available":true},
    {"service":"opsgenie","service_name":"Opsgenie","type":"api_key","method":"api_key","fields":{"api_key":{"label":"API Key","secret":true,"required":true}},"available":true},
    {"service":"webhook","service_name":"Webhook","type":"Messaging","method":"apikey","fields":{"key":"URL","username":"Username (optional)","password":"Password (optional)"},"available":true},
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
			fmtWrite(w, `{"service":"opsgenie","name":"On-call","label":"On-call","method":"api_key","available":true}`)
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
	if !strings.Contains(output, "On-call") || !strings.Contains(output, "opsgenie") {
		t.Errorf("expected label and service in output, got:\n%s", output)
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
			fmtWrite(w, `{"service":"opsgenie","label":"Opsgenie","name":"Opsgenie"}`)
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
			fmtWrite(w, `{"status":"complete","service":"slack","label":"Workspace","name":"Workspace"}`)
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
	if !strings.Contains(output, "Workspace") || !strings.Contains(output, "slack") {
		t.Errorf("expected label and service after complete, got:\n%s", output)
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

func TestConnect_AddTo_AmbiguousLabelSurfacesError(t *testing.T) {
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
			fmtWrite(w, `{"status":"complete","service":"slack","label":"Workspace"}`)
		case r.Method == "GET" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"key":"default","name":"Default","notifications":{"slack":["#existing"]},"monitors":["abc"],"monitor_details":{"abc":{}},"status":"ok","created":"2020-01-01T00:00:00Z"}`)
		case r.Method == "PUT" && r.URL.Path == "/notifications/default":
			mu.Lock()
			putCount++
			putBodies = append(putBodies, string(body))
			mu.Unlock()
			w.WriteHeader(400)
			fmtWrite(w, `{"error":"ambiguous label"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "slack", "--no-browser", "--add-to", "default")
	if err != nil && code == 0 {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit 1 when add-to is ambiguous, got %d\n%s", code, output)
	}
	if putCount != 1 {
		t.Fatalf("expected exactly one PUT with the label (no service:pk retry), got %d", putCount)
	}

	var first map[string]interface{}
	if err := json.Unmarshal([]byte(putBodies[0]), &first); err != nil {
		t.Fatalf("PUT not JSON: %s", putBodies[0])
	}
	firstSlack := toStringSlice(first["notifications"].(map[string]interface{})["slack"])
	if !containsString(firstSlack, "Workspace") {
		t.Errorf("PUT should append label Workspace, got %#v", firstSlack)
	}
	for _, readonly := range []string{"monitors", "monitor_details", "status", "created"} {
		if _, ok := first[readonly]; ok {
			t.Errorf("PUT must not send read-only field %s: %s", readonly, putBodies[0])
		}
	}
	if !strings.Contains(strings.ToLower(output), "ambiguous") {
		t.Errorf("expected ambiguous-label error to be surfaced, got:\n%s", output)
	}
	if strings.Contains(output, "Added") {
		t.Errorf("must not print Added after an ambiguous-label error:\n%s", output)
	}
}

func TestConnect_Telegram_MatchByLabelDiff(t *testing.T) {
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
				fmtWrite(w, `{"integrations":[{"service":"telegram","name":"Existing","label":"Existing"}]}`)
				return
			}
			fmtWrite(w, `{"integrations":[{"service":"telegram","name":"Existing","label":"Existing"},{"service":"telegram","name":"New Bot","label":"New Bot"}]}`)
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
	if !strings.Contains(output, "New Bot") {
		t.Errorf("expected new integration from label-diff, got:\n%s", output)
	}
	if strings.Contains(output, "Existing") && !strings.Contains(output, "New Bot") {
		t.Errorf("matched the pre-existing integration instead of the new one:\n%s", output)
	}
}

func TestConnect_Telegram_MatchByName(t *testing.T) {
	var mu sync.Mutex
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "GET" && r.URL.Path == "/integrations":
			mu.Lock()
			listCalls++
			n := listCalls
			mu.Unlock()
			w.WriteHeader(200)
			// Pre-list and first poll still only have the existing row.
			if n <= 2 {
				fmtWrite(w, `{"integrations":[{"service":"telegram","name":"Existing","label":"Existing"}]}`)
				return
			}
			fmtWrite(w, `{"integrations":[{"service":"telegram","name":"Existing","label":"Existing"},{"service":"telegram","name":"On-call bot","label":"On-call bot"}]}`)
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
	if listCalls < 3 {
		t.Errorf("expected to keep polling past the pre-existing same-name row, listCalls=%d", listCalls)
	}
	if !strings.Contains(output, "On-call bot") {
		t.Errorf("expected new label that matches --name, got:\n%s", output)
	}
	if strings.Contains(output, "Existing") && !strings.Contains(output, "On-call bot") {
		t.Errorf("matched the pre-existing row instead of the --name label:\n%s", output)
	}
}

func TestConnect_FormatJSON_ParseableOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations/connect":
			w.WriteHeader(200)
			fmtWrite(w, `{"authorize_url":"https://slack.example/oauth","token":"tok_json","expires_at":"2099-01-01T00:00:00Z","poll_interval":1}`)
		case r.Method == "GET" && r.URL.Path == "/integrations/connect/tok_json":
			w.WriteHeader(200)
			fmtWrite(w, `{"status":"complete","service":"slack","label":"Workspace"}`)
		case r.Method == "GET" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"key":"default","name":"Default","notifications":{"slack":[]}}`)
		case r.Method == "PUT" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			w.Write(mustRead(r))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "slack", "--no-browser", "--format", "json", "--add-to", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) {
		t.Fatalf("expected stdout to be parseable JSON only, got:\n%s", output)
	}
	if strings.Contains(output, "https://slack.example/oauth") {
		t.Errorf("authorize URL must not appear on stdout in --format json:\n%s", output)
	}
	if strings.Contains(output, "Added") {
		t.Errorf("Added confirmation must not appear on stdout in --format json:\n%s", output)
	}
	if !strings.Contains(trimmed, "Workspace") {
		t.Errorf("expected complete payload label in JSON stdout, got:\n%s", output)
	}
}

func TestConnect_AddTo_WebhookUsesPluralKey(t *testing.T) {
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"webhook","name":"Hook","label":"Hook"}`)
		case r.Method == "GET" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"key":"default","name":"Default","notifications":{"webhooks":["https://old.example"]},"monitors":["m1"],"status":"ok","created":"2020-01-01T00:00:00Z"}`)
		case r.Method == "PUT" && r.URL.Path == "/notifications/default":
			putBody = string(body)
			w.WriteHeader(200)
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "webhook", "--name", "Hook", "--field", "key=https://example.com/hook", "--add-to", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(putBody), &payload); err != nil {
		t.Fatalf("PUT body is not JSON: %s", putBody)
	}
	notifications, _ := payload["notifications"].(map[string]interface{})
	if notifications == nil {
		t.Fatalf("PUT missing notifications: %s", putBody)
	}
	if _, ok := notifications["webhook"]; ok {
		t.Errorf("must not write singular notifications.webhook: %s", putBody)
	}
	hooks := toStringSlice(notifications["webhooks"])
	if !containsString(hooks, "Hook") {
		t.Errorf("expected notifications.webhooks to include Hook, got %#v", hooks)
	}
	for _, readonly := range []string{"monitors", "status", "created"} {
		if _, ok := payload[readonly]; ok {
			t.Errorf("PUT must not send read-only field %s: %s", readonly, putBody)
		}
	}
	if !strings.Contains(output, "Added") {
		t.Errorf("expected Added only after verified PUT, got:\n%s", output)
	}
}

func TestConnect_AddTo_DedupeSkipsExisting(t *testing.T) {
	putCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"webhook","name":"Hook","label":"Hook"}`)
		case r.Method == "GET" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"key":"default","name":"Default","notifications":{"webhooks":["Hook"]}}`)
		case r.Method == "PUT" && r.URL.Path == "/notifications/default":
			putCount++
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "webhook", "--name", "Hook", "--field", "key=https://example.com/hook", "--add-to", "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if putCount != 0 {
		t.Fatalf("expected no PUT when the entry already exists, got %d", putCount)
	}
}

func TestConnect_AddTo_UnverifiedDoesNotPrintAdded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"webhook","name":"Hook","label":"Hook"}`)
		case r.Method == "GET" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"key":"default","name":"Default","notifications":{"webhooks":[]}}`)
		case r.Method == "PUT" && r.URL.Path == "/notifications/default":
			w.WriteHeader(200)
			fmtWrite(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("connect", "webhook", "--name", "Hook", "--field", "key=https://example.com/hook", "--add-to", "default")
	if err != nil && code == 0 {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 1 {
		t.Fatalf("expected exit 1 when add-to is unverified, got %d\n%s", code, output)
	}
	if strings.Contains(output, "Added") {
		t.Errorf("must not print Added for an unverified PUT:\n%s", output)
	}
}

func TestConnect_SecretsRedactedFromVerboseAndLog(t *testing.T) {
	const secret = "verbose-secret-api-key-do-not-print"
	logFile := filepath.Join(t.TempDir(), "debug.log")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"opsgenie","name":"On-call","label":"On-call"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	verbose = true
	viper.Set(varLog, logFile)
	output, code, err := executeWithExit("--verbose", "--log", logFile, "connect", "opsgenie", "--name", "On-call", "--field", "api_key="+secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if strings.Contains(output, secret) {
		t.Errorf("secret leaked to stdout with --verbose:\n%s", output)
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("expected debug log file: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Errorf("secret leaked to --log file:\n%s", data)
	}
	if !strings.Contains(string(data), `"label":"API Key"`) {
		t.Errorf("catalogue fields metadata must remain visible as \"label\":\"API Key\" under --verbose/--log, got:\n%s", data)
	}
}

func TestParseCatalogueFields_ServerStringShape(t *testing.T) {
	fields := parseCatalogueFields(json.RawMessage(`{"key":"URL","username":"Username (optional)","password":"Password (optional)"}`))
	byKey := map[string]catalogueField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	if len(byKey) != 3 {
		t.Fatalf("expected 3 fields, got %#v", fields)
	}
	if f := byKey["key"]; !f.Required || !f.Secret || f.Label != "URL" {
		t.Errorf("key should be required and secret with label URL, got %#v", f)
	}
	if f := byKey["username"]; f.Required || f.Secret {
		t.Errorf("username should be optional and not secret, got %#v", f)
	}
	if f := byKey["password"]; f.Required || !f.Secret {
		t.Errorf("password should be optional and secret, got %#v", f)
	}
}

func TestConnect_Webhook_OptionalFieldsNotRequired(t *testing.T) {
	var createBody string
	var secretPrompts, linePrompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations/services":
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
		case r.Method == "POST" && r.URL.Path == "/integrations":
			createBody = string(body)
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"webhook","name":"Hook","label":"Hook"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()
	readSecretFn = func(prompt string) (string, error) {
		secretPrompts = append(secretPrompts, prompt)
		return "", nil
	}
	readLineFn = func(prompt string) (string, error) {
		linePrompts = append(linePrompts, prompt)
		return "relay", nil
	}

	output, code, err := executeWithExit("connect", "webhook", "--name", "Hook", "--field", "key=https://example.com/hook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0 when optional fields are left empty, got %d\n%s", code, output)
	}
	if !strings.Contains(strings.Join(linePrompts, "\n"), "Username") {
		t.Errorf("username is not a secret and should use the echoing prompt, got line=%#v secret=%#v", linePrompts, secretPrompts)
	}
	if !strings.Contains(strings.Join(secretPrompts, "\n"), "Password") {
		t.Errorf("password should use the hidden prompt, got secret=%#v", secretPrompts)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(createBody), &payload); err != nil {
		t.Fatalf("create body is not JSON: %s", createBody)
	}
	fields, _ := payload["fields"].(map[string]interface{})
	if fields["key"] != "https://example.com/hook" || fields["username"] != "relay" {
		t.Errorf("expected key and username in fields, got %#v", fields)
	}
	if _, ok := fields["password"]; ok {
		t.Errorf("empty optional password must be omitted, got %#v", fields)
	}
}

func TestRedactRequestFields_NestedSecretsAndCatalogueMetadata(t *testing.T) {
	request := `{"service":"webhook","fields":{"auth":{"token":"nested-token-secret"},"urls":["array-secret-value"],"api_key":"scalar-secret"}}`
	redacted, ok := redactSecretJSON(request)
	if !ok {
		t.Fatal("expected request JSON to be redacted")
	}
	if strings.Contains(redacted, "nested-token-secret") || strings.Contains(redacted, "array-secret-value") || strings.Contains(redacted, "scalar-secret") {
		t.Errorf("nested field secrets leaked: %s", redacted)
	}
	if !strings.Contains(redacted, "[REDACTED]") {
		t.Errorf("expected [REDACTED] placeholders, got %s", redacted)
	}

	catalogue := `{"services":[{"service":"opsgenie","fields":{"api_key":{"label":"API Key","secret":true,"required":true}}}]}`
	kept, ok := redactSecretJSON(catalogue)
	if !ok {
		t.Fatal("expected catalogue JSON to parse")
	}
	if !strings.Contains(kept, `"label":"API Key"`) {
		t.Errorf("catalogue metadata label was redacted: %s", kept)
	}
	if !strings.Contains(kept, `"required":true`) {
		t.Errorf("catalogue metadata required flag was redacted: %s", kept)
	}
}

func mustRead(r *http.Request) []byte {
	body, _ := io.ReadAll(r.Body)
	return body
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
			fmtWrite(w, `{"status":"complete","label":"A"}`)
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

func TestPublicLabel_PrefersLabelThenName(t *testing.T) {
	if got := publicLabel("Workspace", "Other"); got != "Workspace" {
		t.Errorf("prefer label, got %q", got)
	}
	if got := publicLabel("", "Workspace"); got != "Workspace" {
		t.Errorf("fall back to name, got %q", got)
	}

	label, service := extractPublicIdentity([]byte(`{"id":"Workspace","service":"slack","label":"Workspace"}`))
	if label != "Workspace" || service != "slack" {
		t.Errorf("extractPublicIdentity should read label and service, got label=%q service=%q", label, service)
	}
	label, service = extractPublicIdentity([]byte(`{"status":"complete","integration":{"label":"Nested","service":"pagerduty"}}`))
	if label != "Nested" || service != "pagerduty" {
		t.Errorf("extractPublicIdentity should read the nested integration, got label=%q service=%q", label, service)
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
