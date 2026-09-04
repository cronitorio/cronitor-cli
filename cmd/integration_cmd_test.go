package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "services", "create", "delete"}
	for _, name := range subcommands {
		found := false
		for _, cmd := range integrationCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand %q not found in integration command", name)
		}
	}
}

func TestIntegrationPersistentFlags(t *testing.T) {
	for _, flag := range []string{"page", "page-size", "format", "output"} {
		if integrationCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag --%s not found", flag)
		}
	}
}

func TestIntegrationCreateCommandFlags(t *testing.T) {
	for _, flag := range []string{"service", "name", "field", "identifier", "data", "file"} {
		if integrationCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag --%s not found on integration create", flag)
		}
	}
}

func TestIntegrationDeleteCommandFlags(t *testing.T) {
	if integrationDeleteCmd.Flags().Lookup("force") == nil {
		t.Error("Expected flag --force not found on integration delete")
	}
	if integrationDeleteCmd.Flags().Lookup("service") == nil {
		t.Error("Expected flag --service not found on integration delete")
	}
	if integrationGetCmd.Flags().Lookup("service") == nil {
		t.Error("Expected flag --service not found on integration get")
	}
}

func TestIntegrationCommandAliases(t *testing.T) {
	found := false
	for _, alias := range integrationCmd.Aliases {
		if alias == "integrations" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'integrations' not found")
	}
}

func TestConnectCommandFlags(t *testing.T) {
	for _, flag := range []string{"name", "field", "no-browser", "timeout", "add-to", "format"} {
		if connectCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag --%s not found on connect", flag)
		}
	}
}

func TestIntegration_ServicesTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations/services" {
			w.WriteHeader(200)
			fmtWrite(w, catalogueJSON())
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "services")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	for _, header := range []string{"SERVICE", "METHOD", "AVAILABLE"} {
		if !strings.Contains(output, header) {
			t.Errorf("expected table header %q, got:\n%s", header, output)
		}
	}
	if !strings.Contains(output, "microsoft-teams") && !strings.Contains(output, "slack") {
		t.Errorf("expected catalogue keys in services table, got:\n%s", output)
	}
}

func TestIntegration_ListTableAndFilters(t *testing.T) {
	var gotService, gotPage, gotPageSize string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations" {
			gotService = r.URL.Query().Get("service")
			gotPage = r.URL.Query().Get("page")
			gotPageSize = r.URL.Query().Get("pageSize")
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"service":"slack","service_name":"Slack","method":"oauth","name":"Workspace","label":"Workspace","available":true,"created":"2026-01-01T00:00:00Z"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "list", "--service", "slack", "--page", "2", "--page-size", "25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if gotService != "slack" || gotPage != "2" || gotPageSize != "25" {
		t.Errorf("expected query service=slack page=2 pageSize=25, got service=%q page=%q pageSize=%q", gotService, gotPage, gotPageSize)
	}
	for _, header := range []string{"LABEL", "SERVICE", "METHOD", "AVAILABLE"} {
		if !strings.Contains(output, header) {
			t.Errorf("expected table header %q, got:\n%s", header, output)
		}
	}
	if strings.Contains(output, " ID ") || strings.Contains(output, "ID  ") || strings.HasPrefix(strings.TrimSpace(output), "ID") {
		t.Errorf("list table must not use an ID column, got:\n%s", output)
	}
	if !strings.Contains(output, "Workspace") || !strings.Contains(output, "slack") {
		t.Errorf("expected integration row, got:\n%s", output)
	}
	if strings.Contains(output, "slack:") {
		t.Errorf("must not print composite pk ids, got:\n%s", output)
	}
}

func TestIntegration_Get(t *testing.T) {
	var gotService, gotLabel, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations" {
			gotPath = r.URL.Path
			gotService = r.URL.Query().Get("service")
			gotLabel = r.URL.Query().Get("label")
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"id":"Alerts","service":"slack","service_name":"Slack","method":"oauth","name":"Alerts","label":"Alerts","available":true}],"page":1,"page_size":50,"total_integration_count":1}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "get", "Alerts", "--service", "slack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if gotPath != "/integrations" || gotService != "slack" || gotLabel != "Alerts" {
		t.Errorf("expected GET /integrations?service=slack&label=Alerts, got path=%q service=%q label=%q", gotPath, gotService, gotLabel)
	}
	trimmed := strings.TrimSpace(output)
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		t.Fatalf("expected a single JSON object on stdout, got:\n%s", output)
	}
	if _, isEnvelope := obj["integrations"]; isEnvelope {
		t.Errorf("expected the integration object, not the list envelope:\n%s", output)
	}
	if obj["label"] != "Alerts" || obj["service"] != "slack" {
		t.Errorf("expected label Alerts and service slack, got:\n%s", output)
	}
}

func TestIntegration_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations" {
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[],"page":1,"page_size":50,"total_integration_count":0}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, _ := executeWithExit("integration", "get", "Missing")
	if code != 1 {
		t.Fatalf("expected exit 1 for an empty list, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "not found") {
		t.Errorf("expected not-found message, got:\n%s", output)
	}
}

func TestIntegration_Get_AmbiguousWithoutService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations" {
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"service":"slack","label":"Alerts"},{"service":"discord","label":"Alerts"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, _ := executeWithExit("integration", "get", "Alerts")
	if code != 1 {
		t.Fatalf("expected exit 1 for an ambiguous label, got %d\n%s", code, output)
	}
	for _, want := range []string{"slack", "discord", "--service"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in ambiguity message, got:\n%s", want, output)
		}
	}
}

func TestIntegration_Create_FlagsAndFile(t *testing.T) {
	const secret = "create-secret-value-do-not-print"
	var createBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method == "POST" && r.URL.Path == "/integrations" {
			createBody = string(body)
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"discord","name":"Alerts","label":"Alerts"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "create", "--service", "discord", "--name", "Alerts", "--field", "url="+secret, "--identifier", "hook-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if strings.Contains(output, secret) {
		t.Errorf("secret leaked to stdout:\n%s", output)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(createBody), &payload); err != nil {
		t.Fatalf("create body is not JSON: %s", createBody)
	}
	if payload["service"] != "discord" || payload["name"] != "Alerts" || payload["identifier"] != "hook-1" {
		t.Errorf("unexpected create payload: %#v", payload)
	}
	fields, _ := payload["fields"].(map[string]interface{})
	if fields["url"] != secret {
		t.Errorf("expected fields.url, got %#v", payload["fields"])
	}

	tmp := filepath.Join(t.TempDir(), "integration.json")
	fileBody := `{"service":"webhook","name":"Hook","fields":{"url":"https://example.com"}}`
	if err := os.WriteFile(tmp, []byte(fileBody), 0644); err != nil {
		t.Fatal(err)
	}
	createBody = ""
	resetIntegrationFlags()
	output, code, err = executeWithExit("integration", "create", "--file", tmp)
	if err != nil {
		t.Fatalf("file create unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("file create expected exit 0, got %d\n%s", code, output)
	}
	if createBody != fileBody && !strings.Contains(createBody, `"service":"webhook"`) {
		t.Errorf("expected file JSON to be posted, got %s", createBody)
	}
}

func TestIntegration_Create_FormatJSON_ParseableOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/integrations" {
			w.WriteHeader(201)
			fmtWrite(w, `{"service":"discord","name":"Alerts","label":"Alerts"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "create", "--service", "discord", "--name", "Alerts", "--field", "url=https://example.com/hook", "--format", "json")
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
	if strings.Contains(output, "Created") {
		t.Errorf("Created confirmation must not appear on stdout in --format json:\n%s", output)
	}
	if !strings.Contains(trimmed, "Alerts") {
		t.Errorf("expected create payload label in JSON stdout, got:\n%s", output)
	}
	if strings.Contains(trimmed, "discord:") {
		t.Errorf("JSON stdout must not teach composite pk ids, got:\n%s", output)
	}
}

func TestIntegration_Delete_Force(t *testing.T) {
	var gotPath, force, service, label string
	listCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations":
			listCalls++
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"service":"slack","label":"Alerts"}]}`)
		case r.Method == "DELETE" && r.URL.Path == "/integrations":
			gotPath = r.URL.Path
			force = r.URL.Query().Get("force")
			service = r.URL.Query().Get("service")
			label = r.URL.Query().Get("label")
			w.WriteHeader(200)
			fmtWrite(w, `{"deleted":true,"detached_from":{"notification_lists":[],"monitors":[]}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "delete", "Alerts", "--service", "slack", "--force")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if gotPath != "/integrations" || service != "slack" || label != "Alerts" || force != "1" {
		t.Errorf("expected DELETE /integrations?service=slack&label=Alerts&force=1, got path=%q service=%q label=%q force=%q", gotPath, service, label, force)
	}
	if listCalls != 0 {
		t.Errorf("expected no lookup when --service is given, got %d list calls", listCalls)
	}
	if !strings.Contains(output, "Alerts") {
		t.Errorf("expected deleted label in output, got:\n%s", output)
	}
}

func TestIntegration_Delete_ResolvesServiceFromLabel(t *testing.T) {
	var service, label string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/integrations":
			if r.URL.Query().Get("label") != "Alerts" {
				t.Errorf("expected lookup by label, got %q", r.URL.RawQuery)
			}
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"service":"discord","label":"Alerts"}]}`)
		case r.Method == "DELETE" && r.URL.Path == "/integrations":
			service = r.URL.Query().Get("service")
			label = r.URL.Query().Get("label")
			w.WriteHeader(200)
			fmtWrite(w, `{"deleted":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "delete", "Alerts")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if service != "discord" || label != "Alerts" {
		t.Errorf("expected DELETE with service resolved from the label, got service=%q label=%q", service, label)
	}
}

func TestIntegration_Delete_InUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/integrations" {
			w.WriteHeader(409)
			fmtWrite(w, `{"error":"in_use","notification_lists":["oncall"],"monitors":["important-job"]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, _ := executeWithExit("integration", "delete", "Alerts", "--service", "discord")
	if code != 1 {
		t.Fatalf("expected exit 1 when in use, got %d\n%s", code, output)
	}
	for _, want := range []string{"oncall", "important-job", "--force"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in in-use message, got:\n%s", want, output)
		}
	}
}

func TestIntegration_ListJSONOutputToFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations" {
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"service":"slack","label":"Workspace"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	outFile := filepath.Join(t.TempDir(), "integrations.json")
	output, code, err := executeWithExit("integration", "list", "--format", "json", "--output", outFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected output file: %v", err)
	}
	if !json.Valid(bytesTrim(data)) {
		t.Errorf("expected valid JSON in file, got %s", data)
	}
	if !strings.Contains(string(data), "Workspace") {
		t.Errorf("expected file to contain Workspace, got %s", data)
	}
	if strings.Contains(string(data), "slack:") {
		t.Errorf("file must not teach composite pk ids, got %s", data)
	}
	if strings.Contains(output, `"Workspace"`) {
		t.Error("expected stdout not to contain JSON when writing to a file")
	}
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
