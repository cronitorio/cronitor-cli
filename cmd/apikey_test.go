package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

const (
	apiKeyTestCustom    = "cust0123456789abcdefghijklmnWXYZ"
	apiKeyTestSDK       = "sdk00123456789abcdefghijklmnSDKK"
	apiKeyTestTelemetry = "tele0123456789abcdefghijklmnTELE"
	apiKeyTestCreated   = "new00123456789abcdefghijklmnNEWK"
)

func TestAPIKeyCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "delete"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range apiKeyCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in apikey command", name)
		}
	}
}

func TestAPIKeyPersistentFlags(t *testing.T) {
	flags := []string{"format", "output"}

	for _, flag := range flags {
		if apiKeyCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in apikey command", flag)
		}
	}
}

func TestAPIKeySubcommandFlags(t *testing.T) {
	cases := map[string][]string{
		"list":   {"page", "search"},
		"create": {"name", "kind", "scope"},
		"update": {"name", "scope"},
		"delete": {"yes"},
	}
	for name, flags := range cases {
		cmd, _, err := apiKeyCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		for _, flag := range flags {
			if cmd.Flags().Lookup(flag) == nil {
				t.Errorf("Expected flag '--%s' not found in apikey %s command", flag, name)
			}
		}
	}
}

func TestAPIKeyCommandAliases(t *testing.T) {
	for _, want := range []string{"apikeys", "keys"} {
		found := false
		for _, alias := range apiKeyCmd.Aliases {
			if alias == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected alias '%s' not found", want)
		}
	}
}

func TestMaskAPIKey(t *testing.T) {
	cases := map[string]string{
		apiKeyTestCustom: "cust…WXYZ",
		"short":          "…",
		"":               "",
	}
	for in, want := range cases {
		if got := maskAPIKey(in); got != want {
			t.Errorf("maskAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAPIKeyList_TableMasksKeys(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/keys", 200, testutil.LoadFixture("apikeys_list.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "list")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if req := mock.LastRequest(); req.Method != "GET" || req.Path != "/keys" {
		t.Errorf("got %s %s, want GET /keys", req.Method, req.Path)
	}
	for _, want := range []string{"NAME", "KIND", "SCOPES", "KEY", "LAST USED",
		"Deploy bot", "telemetry", "monitor:read, monitor:write", "never",
		"cust…WXYZ", "sdk0…SDKK", "tele…TELE", "Showing page 1 of 2"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in table:\n%s", want, output)
		}
	}
	for _, key := range []string{apiKeyTestCustom, apiKeyTestSDK, apiKeyTestTelemetry} {
		if strings.Contains(output, key) {
			t.Errorf("table leaked full key %s:\n%s", key, output)
		}
	}
}

func TestAPIKeyList_PageAndSearch(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/keys", 200, testutil.LoadFixture("apikeys_list.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "list", "--page", "2", "--search", "deploy")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	q := mock.LastRequest().QueryParams
	if q.Get("page") != "2" || q.Get("search") != "deploy" {
		t.Errorf("query = %v, want page=2 search=deploy", q)
	}
}

func TestAPIKeyList_JSONIsUnmasked(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	fixture := testutil.LoadFixture("apikeys_list.json")
	mock.On("GET", "/keys", 200, fixture)
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "list", "--format", "json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if strings.TrimSpace(output) != strings.TrimSpace(FormatJSON([]byte(fixture))) {
		t.Errorf("expected the raw response as JSON, got:\n%s", output)
	}
}

func TestAPIKeyList_NoAccountAccessShowsDetail(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/keys", 403, testutil.LoadFixture("error_responses/403_no_account_access.json"))
	withAccountTest(t, mock)

	output, code, _ := executeWithExit("apikey", "list")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "Run: cronitor auth login --admin") {
		t.Errorf("expected the server detail, got:\n%s", output)
	}
}

func TestAPIKeyGet_JSON(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	fixture := testutil.LoadFixture("apikey_get.json")
	mock.On("GET", "/keys/"+apiKeyTestCustom, 200, fixture)
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "get", apiKeyTestCustom)
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if req := mock.LastRequest(); req.Method != "GET" || req.Path != "/keys/"+apiKeyTestCustom {
		t.Errorf("got %s %s", req.Method, req.Path)
	}
	if strings.TrimSpace(output) != strings.TrimSpace(FormatJSON([]byte(fixture))) {
		t.Errorf("expected the key as JSON, got:\n%s", output)
	}
}

func TestAPIKeyCreate_CustomPrintsKeyOnce(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("POST", "/keys", 201, testutil.LoadFixture("apikey_create.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "create", "--name", "Deploy bot", "--scope", "monitor:read", "--scope", "monitor:write")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	req := mock.LastRequest()
	if req.Method != "POST" || req.Path != "/keys" {
		t.Errorf("got %s %s, want POST /keys", req.Method, req.Path)
	}
	assertJSONBody(t, req.Body, map[string]interface{}{
		"name": "Deploy bot", "kind": "custom", "scopes": []string{"monitor:read", "monitor:write"},
	})
	if strings.Count(output, apiKeyTestCreated) != 1 {
		t.Errorf("expected the full key exactly once:\n%s", output)
	}
	if !strings.Contains(output, "only time") {
		t.Errorf("expected a note that the full key is shown only this time:\n%s", output)
	}
}

func TestAPIKeyCreate_TelemetrySendsEmptyScopes(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("POST", "/keys", 201, testutil.LoadFixture("apikey_create.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "create", "--name", "Telemetry", "--kind", "telemetry")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertJSONBody(t, mock.LastRequest().Body, map[string]interface{}{
		"name": "Telemetry", "kind": "telemetry", "scopes": []string{},
	})
}

func TestAPIKeyCreate_JSONFormat(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	fixture := testutil.LoadFixture("apikey_create.json")
	mock.On("POST", "/keys", 201, fixture)
	withAccountTest(t, mock)

	output, code, err := executeWithExit("apikey", "create", "--name", "Deploy bot", "--scope", "monitor:read", "--format", "json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	trimmed := strings.TrimSpace(output)
	if !json.Valid([]byte(trimmed)) || !strings.Contains(trimmed, apiKeyTestCreated) {
		t.Errorf("expected only the created key as JSON, got:\n%s", output)
	}
}

func TestAPIKeyCreate_RejectsBadInputWithoutRequest(t *testing.T) {
	cases := map[string][]string{
		"custom without scope": {"apikey", "create", "--name", "Deploy bot"},
		"invalid kind":         {"apikey", "create", "--name", "Deploy bot", "--kind", "admin", "--scope", "monitor:read"},
		"missing name":         {"apikey", "create", "--scope", "monitor:read"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			mock := testutil.NewMockAPI()
			defer mock.Close()
			withAccountTest(t, mock)

			output, code, _ := executeWithExit(args...)
			if code != 1 {
				t.Fatalf("expected exit 1, got %d\n%s", code, output)
			}
			if mock.RequestCount() != 0 {
				t.Errorf("expected no request, got %d", mock.RequestCount())
			}
		})
	}
}

func TestAPIKeyUpdate_SendsOnlyGivenFields(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want map[string]interface{}
	}{
		{"name only", []string{"--name", "Renamed"}, map[string]interface{}{"name": "Renamed"}},
		{"scopes only", []string{"--scope", "monitor:read", "--scope", "issue:read"}, map[string]interface{}{"scopes": []string{"monitor:read", "issue:read"}}},
		{"both", []string{"--name", "Renamed", "--scope", "site:read"}, map[string]interface{}{"name": "Renamed", "scopes": []string{"site:read"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := testutil.NewMockAPI()
			defer mock.Close()
			mock.On("PATCH", "/keys/"+apiKeyTestCustom, 200, testutil.LoadFixture("apikey_get.json"))
			withAccountTest(t, mock)

			args := append([]string{"apikey", "update", apiKeyTestCustom}, tc.args...)
			output, code, err := executeWithExit(args...)
			if err != nil || code != 0 {
				t.Fatalf("exit %d err %v\n%s", code, err, output)
			}
			req := mock.LastRequest()
			if req.Method != "PATCH" || req.Path != "/keys/"+apiKeyTestCustom {
				t.Errorf("got %s %s", req.Method, req.Path)
			}
			assertJSONBody(t, req.Body, tc.want)
			if strings.Contains(output, apiKeyTestCustom) {
				t.Errorf("update output should mask the key:\n%s", output)
			}
		})
	}
}

func TestAPIKeyUpdate_NothingToUpdate(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withAccountTest(t, mock)

	output, code, _ := executeWithExit("apikey", "update", apiKeyTestCustom)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, output)
	}
	if mock.RequestCount() != 0 {
		t.Errorf("expected no request, got %d", mock.RequestCount())
	}
}

func TestAPIKeyDelete_YesSkipsPrompt(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("DELETE", "/keys/"+apiKeyTestCustom, 204, "")
	withAccountTest(t, mock)
	readLineFn = func(string) (string, error) {
		t.Error("--yes must not prompt")
		return "n", nil
	}

	output, code, err := executeWithExit("apikey", "delete", apiKeyTestCustom, "--yes")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if deletes := requestsWithMethod(mock, "DELETE"); len(deletes) != 1 || deletes[0].Path != "/keys/"+apiKeyTestCustom {
		t.Errorf("expected one DELETE /keys/<key>, got %+v", deletes)
	}
}

func TestAPIKeyDelete_ConfirmNoSendsNothing(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withAccountTest(t, mock)
	var prompt string
	readLineFn = func(p string) (string, error) { prompt = p; return "n", nil }

	output, code, _ := executeWithExit("apikey", "delete", apiKeyTestCustom)
	if code != 1 {
		t.Fatalf("expected exit 1 when cancelled, got %d\n%s", code, output)
	}
	if !strings.Contains(prompt, "cust…WXYZ") || !strings.HasSuffix(prompt, "[y/N] ") {
		t.Errorf("prompt = %q", prompt)
	}
	if deletes := requestsWithMethod(mock, "DELETE"); len(deletes) != 0 {
		t.Errorf("expected no DELETE after answering n, got %+v", deletes)
	}
}

func TestAPIKeyDelete_NonTerminalRefusesWithoutYes(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withAccountTest(t, mock)
	stdinIsTerminalFn = func() bool { return false }

	output, code, _ := executeWithExit("apikey", "delete", apiKeyTestCustom)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "--yes") {
		t.Errorf("expected a hint to pass --yes:\n%s", output)
	}
	if mock.RequestCount() != 0 {
		t.Errorf("expected no request, got %d", mock.RequestCount())
	}
}

func TestAPIKeyDelete_LastKeyShowsDetail(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("DELETE", "/keys/"+apiKeyTestTelemetry, 400, `{"detail":"You cannot delete your last telemetry key."}`)
	withAccountTest(t, mock)

	output, code, _ := executeWithExit("apikey", "delete", apiKeyTestTelemetry, "--yes")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "You cannot delete your last telemetry key.") {
		t.Errorf("expected the server detail, got:\n%s", output)
	}
}

func assertNoAPIKeys(t *testing.T, where, text string) {
	t.Helper()
	for _, key := range []string{apiKeyTestCustom, apiKeyTestSDK, apiKeyTestTelemetry, apiKeyTestCreated} {
		if strings.Contains(text, key) {
			t.Errorf("%s leaked full key %s:\n%s", where, key, text)
		}
	}
}

func TestAPIKey_VerboseAndLogFileRedactKeys(t *testing.T) {
	cases := []struct {
		name  string
		route [3]string // method, path, fixture
		args  []string
	}{
		{"list", [3]string{"GET", "/keys", "apikeys_list.json"}, []string{"apikey", "list"}},
		{"get", [3]string{"GET", "/keys/" + apiKeyTestCustom, "apikey_get.json"}, []string{"apikey", "get", apiKeyTestCustom}},
		{"create", [3]string{"POST", "/keys", "apikey_create.json"}, []string{"apikey", "create", "--name", "Deploy bot", "--scope", "monitor:read"}},
		{"update", [3]string{"PATCH", "/keys/" + apiKeyTestCustom, "apikey_get.json"}, []string{"apikey", "update", apiKeyTestCustom, "--name", "Renamed"}},
		{"delete", [3]string{"DELETE", "/keys/" + apiKeyTestCustom, ""}, []string{"apikey", "delete", apiKeyTestCustom, "--yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := testutil.NewMockAPI()
			defer mock.Close()
			body := ""
			if tc.route[2] != "" {
				body = testutil.LoadFixture(tc.route[2])
			}
			mock.On(tc.route[0], tc.route[1], 200, body)
			withAccountTest(t, mock)

			dir := t.TempDir()
			logFile := filepath.Join(dir, "debug.log")
			viper.Set(varLog, logFile)
			args := append(append([]string{}, tc.args...), "--verbose")
			stdout, stderr, code, err := executeAuth(args...)
			if err != nil || code != 0 {
				t.Fatalf("exit %d err %v\n%s%s", code, err, stdout, stderr)
			}
			// get and create print a full key on purpose; only the verbose
			// log lines must not.
			var logLines []string
			for _, line := range strings.Split(stdout, "\n") {
				for _, prefix := range []string{"API Request:", "Request Body:", "Response Status:", "Response Body:"} {
					if strings.HasPrefix(line, prefix) {
						logLines = append(logLines, line)
					}
				}
			}
			if len(logLines) < 3 {
				t.Fatalf("expected verbose request logs on stdout:\n%s", stdout)
			}
			assertNoAPIKeys(t, "verbose output", strings.Join(logLines, "\n"))

			logged, err := os.ReadFile(logFile)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(logged), "Response Body:") {
				t.Fatalf("expected response logging in the log file:\n%s", logged)
			}
			assertNoAPIKeys(t, "log file", string(logged))
		})
	}
}

func TestAPIKey_NetworkErrorRedactsKey(t *testing.T) {
	mock := testutil.NewMockAPI()
	withAccountTest(t, mock)
	mock.Close()
	// Nothing listens here any more, so the request fails with an error
	// that quotes the URL, which ends in the key.
	lib.BaseURLOverride = mock.Server.URL

	for _, args := range [][]string{
		{"apikey", "get", apiKeyTestCustom},
		{"apikey", "update", apiKeyTestCustom, "--name", "Renamed"},
		{"apikey", "delete", apiKeyTestCustom, "--yes"},
	} {
		resetAPIKeyFlags()
		stdout, stderr, code, _ := executeAuth(args...)
		if code != 1 {
			t.Fatalf("%v: expected exit 1, got %d\n%s%s", args, code, stdout, stderr)
		}
		if !strings.Contains(stdout, "Failed to") {
			t.Errorf("%v: expected a network error, got:\n%s", args, stdout)
		}
		assertNoAPIKeys(t, strings.Join(args[:2], " ")+" error", stdout+stderr)
	}
}

func TestAPIKey_OutputFileIsOwnerOnly(t *testing.T) {
	cases := []struct {
		name  string
		route [3]string
		args  []string
	}{
		{"list", [3]string{"GET", "/keys", "apikeys_list.json"}, []string{"apikey", "list", "--format", "json"}},
		{"get", [3]string{"GET", "/keys/" + apiKeyTestCustom, "apikey_get.json"}, []string{"apikey", "get", apiKeyTestCustom}},
		{"create", [3]string{"POST", "/keys", "apikey_create.json"}, []string{"apikey", "create", "--name", "Deploy bot", "--scope", "monitor:read", "--format", "json"}},
	}
	for _, tc := range cases {
		for _, existing := range []bool{false, true} {
			name := tc.name + "/new file"
			if existing {
				name = tc.name + "/existing 0644 file"
			}
			t.Run(name, func(t *testing.T) {
				mock := testutil.NewMockAPI()
				defer mock.Close()
				mock.On(tc.route[0], tc.route[1], 200, testutil.LoadFixture(tc.route[2]))
				withAccountTest(t, mock)

				out := filepath.Join(t.TempDir(), "keys.json")
				if existing {
					if err := os.WriteFile(out, []byte("old content that is longer than nothing\n"), 0644); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(out, 0644); err != nil {
						t.Fatal(err)
					}
				}

				args := append(append([]string{}, tc.args...), "--output", out)
				output, code, err := executeWithExit(args...)
				if err != nil || code != 0 {
					t.Fatalf("exit %d err %v\n%s", code, err, output)
				}
				data, err := os.ReadFile(out)
				if err != nil {
					t.Fatal(err)
				}
				if !json.Valid(data) {
					t.Errorf("expected only the JSON in %s, got:\n%s", out, data)
				}
				if runtime.GOOS == "windows" {
					return
				}
				info, err := os.Stat(out)
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != 0600 {
					t.Errorf("mode %o, want 0600", info.Mode().Perm())
				}
			})
		}
	}
}
