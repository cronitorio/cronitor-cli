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
			fmtWrite(w, `{"integrations":[{"id":"slack:12","service":"slack","service_name":"Slack","method":"oauth","name":"Workspace","label":"Workspace","available":true,"created":"2026-01-01T00:00:00Z"}]}`)
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
	for _, header := range []string{"ID", "SERVICE", "NAME", "METHOD", "AVAILABLE"} {
		if !strings.Contains(output, header) {
			t.Errorf("expected table header %q, got:\n%s", header, output)
		}
	}
	if !strings.Contains(output, "slack:12") || !strings.Contains(output, "Workspace") {
		t.Errorf("expected integration row, got:\n%s", output)
	}
}

func TestIntegration_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations/discord:44" {
			w.WriteHeader(200)
			fmtWrite(w, `{"id":"discord:44","service":"discord","service_name":"Discord","method":"api_key","name":"Alerts","label":"Alerts","available":true}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "get", "discord:44")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) {
		t.Errorf("expected JSON output, got:\n%s", output)
	}
	if !strings.Contains(output, "discord:44") {
		t.Errorf("expected id in output, got:\n%s", output)
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
			fmtWrite(w, `{"id":"discord:44","service":"discord","name":"Alerts","label":"Alerts"}`)
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

func TestIntegration_Delete_Force(t *testing.T) {
	var force string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/integrations/discord:44" {
			force = r.URL.Query().Get("force")
			w.WriteHeader(204)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cleanup := withConnectTest(t, server.URL)
	defer cleanup()

	output, code, err := executeWithExit("integration", "delete", "discord:44", "--force")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\n%s", code, output)
	}
	if force != "1" {
		t.Errorf("expected force=1 query param, got %q", force)
	}
	if !strings.Contains(output, "discord:44") {
		t.Errorf("expected deleted id in output, got:\n%s", output)
	}
}

func TestIntegration_ListJSONOutputToFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/integrations" {
			w.WriteHeader(200)
			fmtWrite(w, `{"integrations":[{"id":"slack:12","service":"slack","label":"Workspace"}]}`)
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
	if !strings.Contains(string(data), "slack:12") {
		t.Errorf("expected file to contain slack:12, got %s", data)
	}
	if strings.Contains(output, `"slack:12"`) {
		t.Error("expected stdout not to contain JSON when writing to a file")
	}
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
