package cmd

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/spf13/viper"
)

func TestTeamCommandStructure(t *testing.T) {
	subcommands := []string{"list", "invite", "role", "remove"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range teamCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in team command", name)
		}
	}
}

func TestTeamPersistentFlags(t *testing.T) {
	flags := []string{"format", "output"}

	for _, flag := range flags {
		if teamCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in team command", flag)
		}
	}
}

func TestTeamInviteCommandFlags(t *testing.T) {
	flags := []string{"name", "role"}

	for _, flag := range flags {
		if teamInviteCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in team invite command", flag)
		}
	}
}

func TestTeamRemoveCommandFlags(t *testing.T) {
	if teamRemoveCmd.Flags().Lookup("yes") == nil {
		t.Error("Expected flag '--yes' not found in team remove command")
	}
}

func TestTeamCommandAliases(t *testing.T) {
	found := false
	for _, alias := range teamCmd.Aliases {
		if alias == "teams" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'teams' not found")
	}
}

// withAccountTest points the CLI at mock and stubs the prompt hooks. By
// default stdin is an interactive terminal and every prompt is answered "n".
func withAccountTest(t *testing.T, mock *testutil.MockAPI) {
	t.Helper()
	cleanup := setupIntegrationTest(mock.Server.URL)
	resetTeamFlags()
	resetAPIKeyFlags()
	oldLine := readLineFn
	oldTerminal := stdinIsTerminalFn
	readLineFn = func(string) (string, error) { return "n", nil }
	stdinIsTerminalFn = func() bool { return true }
	resetSecretRedaction()
	t.Cleanup(func() {
		cleanup()
		readLineFn = oldLine
		stdinIsTerminalFn = oldTerminal
		verbose = false
		viper.Set(varLog, "")
		resetSecretRedaction()
		resetTeamFlags()
		resetAPIKeyFlags()
		RootCmd.SetArgs([]string{"--help"})
	})
}

func requestsWithMethod(mock *testutil.MockAPI, method string) []testutil.RecordedRequest {
	var out []testutil.RecordedRequest
	for _, req := range mock.Requests {
		if req.Method == method {
			out = append(out, req)
		}
	}
	return out
}

func assertJSONBody(t *testing.T, body string, want map[string]interface{}) {
	t.Helper()
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("request body is not JSON: %s", body)
	}
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(got)
	if string(wantJSON) != string(gotJSON) {
		t.Errorf("request body = %s, want %s", gotJSON, wantJSON)
	}
}

func TestTeamList_Table(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/team", 200, testutil.LoadFixture("team_list.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("team", "list")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if req := mock.LastRequest(); req.Method != "GET" || req.Path != "/team" {
		t.Errorf("got %s %s, want GET /team", req.Method, req.Path)
	}
	for _, want := range []string{"ID", "NAME", "EMAIL", "ROLE", "STATE", "OWNER",
		"Ada Lovelace", "ada@example.com", "member@example.com", "alan+invite@example.com", "readonly", "pending"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in table:\n%s", want, output)
		}
	}
}

func TestTeamList_JSON(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	fixture := testutil.LoadFixture("team_list.json")
	mock.On("GET", "/team", 200, fixture)
	withAccountTest(t, mock)

	output, code, err := executeWithExit("team", "list", "--format", "json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if strings.TrimSpace(output) != strings.TrimSpace(FormatJSON([]byte(fixture))) {
		t.Errorf("expected the raw response as JSON, got:\n%s", output)
	}
}

func TestTeamList_NoAccountAccessShowsDetail(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("GET", "/team", 403, testutil.LoadFixture("error_responses/403_no_account_access.json"))
	withAccountTest(t, mock)

	output, code, _ := executeWithExit("team", "list")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "This API key has no account access. Run: cronitor auth login --admin") {
		t.Errorf("expected the server detail, got:\n%s", output)
	}
	if strings.Contains(output, "detail:") {
		t.Errorf("detail should be printed as a message, not a field error:\n%s", output)
	}
}

func TestTeamInvite_DefaultRoleUser(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("POST", "/team", 201, testutil.LoadFixture("team_member.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("team", "invite", "jane@example.com", "--name", "Jane Doe")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	req := mock.LastRequest()
	if req.Method != "POST" || req.Path != "/team" {
		t.Errorf("got %s %s, want POST /team", req.Method, req.Path)
	}
	assertJSONBody(t, req.Body, map[string]interface{}{"email": "jane@example.com", "name": "Jane Doe", "role": "user"})
	if !strings.Contains(output, "jane@example.com") {
		t.Errorf("expected the invited email in output:\n%s", output)
	}
}

func TestTeamInvite_Role(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("POST", "/team", 201, testutil.LoadFixture("team_member.json"))
	withAccountTest(t, mock)

	output, code, err := executeWithExit("team", "invite", "ops@example.com", "--name", "Ops", "--role", "readonly", "--format", "json")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	assertJSONBody(t, mock.LastRequest().Body, map[string]interface{}{"email": "ops@example.com", "name": "Ops", "role": "readonly"})
	if !strings.Contains(output, `"state": "pending"`) {
		t.Errorf("expected the member JSON with --format json:\n%s", output)
	}
}

func TestTeamInvite_RejectsBadInputWithoutRequest(t *testing.T) {
	cases := map[string][]string{
		"invalid role": {"team", "invite", "jane@example.com", "--name", "Jane", "--role", "owner"},
		"missing name": {"team", "invite", "jane@example.com"},
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

func TestTeamRole_PutsRole(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("PUT", "/team/member@example.com", 200, `{"id":"member@example.com","name":"Grace Hopper","email":"member@example.com","role":"admin","state":"active","is_owner":false}`)
	withAccountTest(t, mock)

	output, code, err := executeWithExit("team", "role", "member@example.com", "admin")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	req := mock.LastRequest()
	if req.Method != "PUT" || req.Path != "/team/member@example.com" {
		t.Errorf("got %s %s, want PUT /team/member@example.com", req.Method, req.Path)
	}
	assertJSONBody(t, req.Body, map[string]interface{}{"role": "admin"})
	if !strings.Contains(output, "admin") {
		t.Errorf("expected the new role in output:\n%s", output)
	}
}

func TestTeamRole_RejectsBadInputWithoutRequest(t *testing.T) {
	cases := map[string][]string{
		"invalid role": {"team", "role", "member@example.com", "owner"},
		"numeric id":   {"team", "role", "42", "admin"},
		"path id":      {"team", "role", "../keys", "admin"},
		"empty id":     {"team", "role", "", "admin"},
		"display name": {"team", "role", "Jane <jane@example.com>", "admin"},
		"padded email": {"team", "role", " member@example.com", "admin"},
		"missing at":   {"team", "role", "not-an-email", "admin"},
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

func TestTeamRole_ForbiddenShowsDetail(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("PUT", "/team/member@example.com", 403, `{"detail":"You do not have permission to perform this action."}`)
	withAccountTest(t, mock)

	output, code, _ := executeWithExit("team", "role", "member@example.com", "admin")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d\n%s", code, output)
	}
	if !strings.Contains(output, "You do not have permission to perform this action.") {
		t.Errorf("expected the server detail, got:\n%s", output)
	}
}

func TestTeamRemove_YesSkipsPrompt(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("DELETE", "/team/member@example.com", 204, "")
	withAccountTest(t, mock)
	readLineFn = func(string) (string, error) {
		t.Error("--yes must not prompt")
		return "n", nil
	}

	output, code, err := executeWithExit("team", "remove", "member@example.com", "--yes")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if deletes := requestsWithMethod(mock, "DELETE"); len(deletes) != 1 || deletes[0].Path != "/team/member@example.com" {
		t.Errorf("expected one DELETE /team/member@example.com, got %+v", deletes)
	}
}

func TestTeamRemove_ConfirmYes(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("DELETE", "/team/member@example.com", 204, "")
	withAccountTest(t, mock)
	var prompt string
	readLineFn = func(p string) (string, error) { prompt = p; return "y", nil }

	output, code, err := executeWithExit("team", "remove", "member@example.com")
	if err != nil || code != 0 {
		t.Fatalf("exit %d err %v\n%s", code, err, output)
	}
	if !strings.Contains(prompt, "member@example.com") || !strings.HasSuffix(prompt, "[y/N] ") {
		t.Errorf("prompt = %q", prompt)
	}
	if deletes := requestsWithMethod(mock, "DELETE"); len(deletes) != 1 {
		t.Errorf("expected one DELETE, got %d", len(deletes))
	}
}

func TestTeamRemove_ConfirmNoSendsNothing(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withAccountTest(t, mock)
	readLineFn = func(string) (string, error) { return "n", nil }

	output, code, _ := executeWithExit("team", "remove", "member@example.com")
	if code != 1 {
		t.Fatalf("expected exit 1 when cancelled, got %d\n%s", code, output)
	}
	if deletes := requestsWithMethod(mock, "DELETE"); len(deletes) != 0 {
		t.Errorf("expected no DELETE after answering n, got %+v", deletes)
	}
}

func TestTeamRemove_NonTerminalRefusesWithoutYes(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	withAccountTest(t, mock)
	stdinIsTerminalFn = func() bool { return false }
	readLineFn = func(string) (string, error) {
		t.Error("must not read stdin when it is not a terminal")
		return "y", nil
	}

	output, code, _ := executeWithExit("team", "remove", "member@example.com")
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

func TestAccountMutations_JSONStdoutIsParseable(t *testing.T) {
	member := `{"id":"member@example.com","name":"Grace Hopper","email":"member@example.com","role":"admin","state":"active","is_owner":false}`
	cases := []struct {
		name      string
		route     [3]string // method, path, body
		args      []string
		emptyBody bool // 204: no JSON to print, so stdout stays empty
	}{
		{"team invite", [3]string{"POST", "/team", member}, []string{"team", "invite", "member@example.com", "--name", "Grace"}, false},
		{"team role", [3]string{"PUT", "/team/member@example.com", member}, []string{"team", "role", "member@example.com", "admin"}, false},
		{"team remove", [3]string{"DELETE", "/team/member@example.com", ""}, []string{"team", "remove", "member@example.com", "--yes"}, true},
		{"apikey create", [3]string{"POST", "/keys", testutil.LoadFixture("apikey_create.json")}, []string{"apikey", "create", "--name", "Deploy bot", "--scope", "monitor:read"}, false},
		{"apikey update", [3]string{"PATCH", "/keys/" + apiKeyTestCustom, testutil.LoadFixture("apikey_get.json")}, []string{"apikey", "update", apiKeyTestCustom, "--name", "Renamed"}, false},
		{"apikey delete", [3]string{"DELETE", "/keys/" + apiKeyTestCustom, ""}, []string{"apikey", "delete", apiKeyTestCustom, "--yes"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := testutil.NewMockAPI()
			defer mock.Close()
			status := 200
			if tc.emptyBody {
				status = 204
			}
			mock.On(tc.route[0], tc.route[1], status, tc.route[2])
			withAccountTest(t, mock)

			args := append(append([]string{}, tc.args...), "--format", "json")
			stdout, stderr, code, err := executeAuth(args...)
			if err != nil || code != 0 {
				t.Fatalf("exit %d err %v\n%s%s", code, err, stdout, stderr)
			}
			if tc.emptyBody {
				if strings.TrimSpace(stdout) != "" {
					t.Errorf("expected empty stdout, got:\n%s", stdout)
				}
			} else if !json.Valid([]byte(stdout)) {
				t.Errorf("stdout is not JSON:\n%s", stdout)
			}
			if !strings.Contains(stderr, "✓") {
				t.Errorf("expected the status line on stderr, got:\n%s", stderr)
			}
		})
	}
}

func TestValidateTeamMemberID(t *testing.T) {
	for _, id := range []string{
		"",
		"42",
		"0",
		"-1",
		"../keys",
		"not-an-email",
		"user@",
		"@example.com",
		" member@example.com",
		"Jane <jane@example.com>",
	} {
		if err := validateTeamMemberID(id); err == nil {
			t.Errorf("validateTeamMemberID(%q) accepted an invalid id", id)
		}
	}
	for _, id := range []string{
		"member@example.com",
		"user+tag@example.com",
		"dev!ops@example.com",
		"dev?ops@example.com",
		"o'brien@example.com",
	} {
		if err := validateTeamMemberID(id); err != nil {
			t.Errorf("validateTeamMemberID(%q) = %v", id, err)
		}
	}
}

// TestTeamList_EmailIDContract locks the list shape from cronitor#467
// (feat/cli-account-access): id is the member email. The server tests use
// id "member@example.com". Decoding runs before the format branch, so both
// table and json must accept that body.
func TestTeamList_EmailIDContract(t *testing.T) {
	fixture := testutil.LoadFixture("team_list.json")
	var decoded struct {
		Data []teamMember `json:"data"`
	}
	if err := json.Unmarshal([]byte(fixture), &decoded); err != nil {
		t.Fatalf("contract fixture does not decode: %v", err)
	}
	if len(decoded.Data) == 0 {
		t.Fatal("contract fixture has no members")
	}
	sawServerExample := false
	for _, m := range decoded.Data {
		if m.ID == "" || !strings.Contains(m.ID, "@") {
			t.Errorf("id %q is not an email", m.ID)
		}
		if m.ID != m.Email {
			t.Errorf("id %q and email %q differ", m.ID, m.Email)
		}
		if m.ID == "member@example.com" {
			sawServerExample = true
		}
	}
	if !sawServerExample {
		t.Fatal(`contract fixture is missing id "member@example.com"`)
	}

	t.Run("table", func(t *testing.T) {
		mock := testutil.NewMockAPI()
		defer mock.Close()
		mock.On("GET", "/team", 200, fixture)
		withAccountTest(t, mock)

		output, code, err := executeWithExit("team", "list")
		if err != nil || code != 0 {
			t.Fatalf("exit %d err %v\n%s", code, err, output)
		}
		for _, m := range decoded.Data {
			if !strings.Contains(output, m.ID) {
				t.Errorf("table missing id %q:\n%s", m.ID, output)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		mock := testutil.NewMockAPI()
		defer mock.Close()
		mock.On("GET", "/team", 200, fixture)
		withAccountTest(t, mock)

		output, code, err := executeWithExit("team", "list", "--format", "json")
		if err != nil || code != 0 {
			t.Fatalf("exit %d err %v\n%s", code, err, output)
		}
		if strings.TrimSpace(output) != strings.TrimSpace(FormatJSON([]byte(fixture))) {
			t.Errorf("expected the raw response as JSON, got:\n%s", output)
		}
		if !strings.Contains(output, `"id": "member@example.com"`) {
			t.Errorf("json output lost the email id:\n%s", output)
		}
	})
}

// djangoTeamEmail resolves path("team/<email>") the way Django does.
// The WSGI server percent-decodes the request target with unquote, which
// keeps '+', and the str converter captures one segment ([^/]+).
// Query decoding (unquote_plus) would turn '+' into a space and miss.
func djangoTeamEmail(requestURI string) (string, bool) {
	rawPath, _, _ := strings.Cut(requestURI, "?")
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", false
	}
	email, ok := strings.CutPrefix(decoded, "/team/")
	if !ok || email == "" || strings.Contains(email, "/") {
		return "", false
	}
	return email, true
}

func TestTeamMutation_EncodedEmailRoundTrip(t *testing.T) {
	// '+' stays literal under PathEscape. '!', '?', apostrophe, and non-ASCII
	// are percent-encoded and come back through Django's unquote.
	emails := []string{
		"user+tag@example.com",
		"dev!ops@example.com",
		"dev?ops@example.com",
		"o'brien@example.com",
		"üser@example.com",
	}
	for _, email := range emails {
		t.Run(email, func(t *testing.T) {
			member := `{"id":"` + email + `","name":"Ops","email":"` + email + `","role":"admin","state":"active","is_owner":false}`
			cases := []struct {
				method string
				args   []string
			}{
				{"PUT", []string{"team", "role", email, "readonly"}},
				{"DELETE", []string{"team", "remove", email, "--yes"}},
			}
			for _, tc := range cases {
				t.Run(tc.method, func(t *testing.T) {
					mock := testutil.NewMockAPI()
					defer mock.Close()
					status := 200
					body := member
					if tc.method == "DELETE" {
						status = 204
						body = ""
					}
					mock.On(tc.method, "/team/"+email, status, body)
					withAccountTest(t, mock)

					output, code, err := executeWithExit(tc.args...)
					if err != nil || code != 0 {
						t.Fatalf("exit %d err %v\n%s", code, err, output)
					}
					req := mock.LastRequest()
					wantURI := "/team/" + url.PathEscape(email)
					if req.RequestURI != wantURI {
						t.Errorf("request target = %q, want PathEscape %q", req.RequestURI, wantURI)
					}
					if strings.Contains(email, "+") && strings.Contains(req.RequestURI, "%2B") {
						t.Errorf("PathEscape must leave '+' literal, request target %q", req.RequestURI)
					}
					got, ok := djangoTeamEmail(req.RequestURI)
					if !ok || got != email {
						t.Errorf("Django resolved %q to %q (ok=%v)", req.RequestURI, got, ok)
					}
				})
			}
		})
	}
}
