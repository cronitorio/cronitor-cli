package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

const (
	authTestAuthCode    = "AUTHORIZATION_CODE_SECRET_do_not_print"
	authTestAccessToken = "WORKOS_ACCESS_TOKEN_SECRET_do_not_print"
	authTestRefreshTok  = "WORKOS_REFRESH_TOKEN_SECRET_do_not_print"
	authTestMachineKey  = "cronitor_machine_key_SECRET_do_not_print"
	authTestClientID    = "client_test_cronitor_cli"
	authTestCredName    = "cli-testhost"
)

type authFake struct {
	mu sync.Mutex

	tokenCalls  int
	tokenScript []string

	createStatus int
	currentBody  string
	currentCode  int
	deleteCode   int

	requests []recordedRequest
}

func newAuthFake() *authFake {
	return &authFake{
		tokenScript:  []string{"ok"},
		createStatus: 201,
		currentCode:  200,
		deleteCode:   204,
		currentBody:  `{"name":"cli-testhost","kind":"machine","scopes":["monitors:read","telemetry:write"],"organization":"Acme"}`,
	}
}

func (f *authFake) deleteUsers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var users []string
	for _, req := range f.requests {
		if req.Method == "DELETE" && req.Path == "/api/cli/machine-credentials/current" {
			users = append(users, req.AuthUser)
		}
	}
	return users
}

func (f *authFake) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	user, _, _ := r.BasicAuth()
	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(body), AuthUser: user})
	path := r.URL.Path
	method := r.Method
	authz := r.Header.Get("Authorization")
	f.mu.Unlock()

	switch {
	case method == "POST" && path == "/oauth2/token":
		f.mu.Lock()
		idx := f.tokenCalls
		f.tokenCalls++
		script := "ok"
		if idx < len(f.tokenScript) {
			script = f.tokenScript[idx]
		} else if len(f.tokenScript) > 0 {
			script = f.tokenScript[len(f.tokenScript)-1]
		}
		f.mu.Unlock()
		switch script {
		case "denied":
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"access_denied"}`)
		default:
			fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q,"token_type":"Bearer","expires_in":300}`, authTestAccessToken, authTestRefreshTok)
		}
	case method == "POST" && path == "/api/cli/machine-credentials":
		if authz != "Bearer "+authTestAccessToken {
			w.WriteHeader(401)
			io.WriteString(w, `{"error":"unauthorized"}`)
			return
		}
		status := f.createStatus
		if status == 0 {
			status = 201
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"key":%q,"name":%q,"kind":"machine","scopes":["monitors:read","telemetry:write"],"organization":"Acme"}`, authTestMachineKey, authTestCredName)
	case method == "GET" && path == "/api/cli/machine-credentials/current":
		if !strings.HasPrefix(authz, "Basic ") {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(f.currentCode)
		if f.currentBody != "" {
			io.WriteString(w, f.currentBody)
		}
	case method == "DELETE" && path == "/api/cli/machine-credentials/current":
		w.WriteHeader(f.deleteCode)
	default:
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"not found"}`)
	}
}

func withAuthTest(t *testing.T, fake *authFake) (serverURL string, cleanup func()) {
	t.Helper()
	if fake == nil {
		fake = newAuthFake()
	}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))

	oldBase := lib.BaseURLOverride
	oldAuthKit := lib.WorkOSAuthKitURLOverride
	oldClient := lib.WorkOSClientIDOverride
	oldPing := lib.PingHostOverride
	oldExit := exitFn
	oldSleep := sleepFn
	oldCallback := readCallbackFn
	oldOpen := openBrowserFn
	oldLine := readLineFn
	oldNow := nowFn
	oldVerbose := verbose
	oldAPIKey := viper.GetString(varApiKey)
	oldManaged := viper.GetBool(varAuthManaged)
	oldName := viper.GetString(varMachineCredentialName)
	oldConfig := viper.GetString(varConfig)
	oldHostname := viper.GetString(varHostname)
	oldPingKey := viper.GetString(varPingApiKey)

	cfg := filepath.Join(t.TempDir(), "cronitor.json")
	lib.BaseURLOverride = server.URL + "/api"
	lib.WorkOSAuthKitURLOverride = server.URL
	lib.WorkOSClientIDOverride = authTestClientID
	lib.PingHostOverride = ""
	sleepFn = func(time.Duration) {}
	openBrowserFn = completeTestBrowserLogin
	readCallbackFn = func(ctx context.Context, raw string) (string, error) { return testCallback(raw), nil }
	readLineFn = func(string) (string, error) { return "y", nil }
	nowFn = time.Now
	verbose = false
	resetSecretRedaction()
	resetAuthFlags()
	resetAPIKeyFlag()
	authYes = true
	if _, ok := os.LookupEnv(varApiKey); ok {
		t.Setenv(varApiKey, "")
	}
	viper.Set(varConfig, cfg)
	viper.Set(varApiKey, "")
	viper.Set(varAuthManaged, false)
	viper.Set(varMachineCredentialName, "")
	viper.Set(varHostname, "testhost")
	viper.Set(varPingApiKey, "")
	viper.Set(varLog, "")

	return server.URL, func() {
		server.Close()
		lib.BaseURLOverride = oldBase
		lib.WorkOSAuthKitURLOverride = oldAuthKit
		lib.WorkOSClientIDOverride = oldClient
		lib.PingHostOverride = oldPing
		exitFn = oldExit
		sleepFn = oldSleep
		readCallbackFn = oldCallback
		openBrowserFn = oldOpen
		readLineFn = oldLine
		nowFn = oldNow
		verbose = oldVerbose
		resetSecretRedaction()
		resetAuthFlags()
		resetAPIKeyFlag()
		viper.Set(varApiKey, oldAPIKey)
		viper.Set(varAuthManaged, oldManaged)
		viper.Set(varMachineCredentialName, oldName)
		viper.Set(varConfig, oldConfig)
		viper.Set(varHostname, oldHostname)
		viper.Set(varPingApiKey, oldPingKey)
		viper.Set(varLog, "")
		RootCmd.SetArgs([]string{"--help"})
	}
}

func executeAuth(args ...string) (stdout, stderr string, code int, err error) {
	oldExit := exitFn
	code = 0
	exitFn = func(c int) { panic(exitSentinel(c)) }
	defer func() { exitFn = oldExit }()

	resetCobraHelpFlags(RootCmd)
	RootCmd.SetArgs(args)
	var execErr error
	stdout, stderr = testutil.CaptureStdoutStderr(func() {
		defer func() {
			if rec := recover(); rec != nil {
				if c, ok := rec.(exitSentinel); ok {
					code = int(c)
					return
				}
				panic(rec)
			}
		}()
		execErr = RootCmd.Execute()
	})
	err = execErr
	return stdout, stderr, code, err
}

func assertNoSecrets(t *testing.T, blobs ...string) {
	t.Helper()
	combined := strings.Join(blobs, "\n")
	for _, secret := range []string{authTestAuthCode, authTestAccessToken, authTestRefreshTok, authTestMachineKey} {
		if strings.Contains(combined, secret) {
			t.Errorf("secret %q leaked:\n%s", secret, combined)
		}
	}
}

func readAuthConfig(t *testing.T) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAuthLogin_InstallsMachineCredential(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	var opened []string
	openBrowserFn = func(u string) { opened = append(opened, u); completeTestBrowserLogin(u) }

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, "/oauth2/authorize?") {
		t.Errorf("expected authorization URI:\n%s", stdout)
	}
	if !strings.Contains(stdout, authTestCredName) {
		t.Errorf("expected credential name:\n%s", stdout)
	}
	if len(opened) != 1 || !strings.Contains(opened[0], "code_challenge_method=S256") {
		t.Errorf("browser: %#v", opened)
	}

	cfg := readAuthConfig(t)
	if cfg["CRONITOR_API_KEY"] != authTestMachineKey {
		t.Errorf("stored key: %#v", cfg["CRONITOR_API_KEY"])
	}
	if cfg["CRONITOR_AUTH_MANAGED"] != true {
		t.Errorf("auth managed: %#v", cfg["CRONITOR_AUTH_MANAGED"])
	}
	if cfg["CRONITOR_MACHINE_CREDENTIAL_NAME"] != authTestCredName {
		t.Errorf("name: %#v", cfg["CRONITOR_MACHINE_CREDENTIAL_NAME"])
	}
	raw, _ := os.ReadFile(configFilePath())
	for _, leak := range []string{authTestAuthCode, authTestAccessToken, authTestRefreshTok} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("config persisted WorkOS secret %s", leak)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(configFilePath())
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("mode %o, want 0600", info.Mode().Perm())
		}
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	var sawCreate bool
	for _, req := range fake.requests {
		if req.Method == "POST" && req.Path == "/api/cli/machine-credentials" {
			sawCreate = true
			var payload map[string]string
			if json.Unmarshal([]byte(req.Body), &payload) != nil || payload["hostname"] != "testhost" || len(payload) != 1 {
				t.Errorf("create body: %s", req.Body)
			}
		}
	}
	if !sawCreate {
		t.Fatal("did not POST machine-credentials")
	}
}

func TestSignupCommandSharesAuthLoginRunE(t *testing.T) {
	cmd, args, err := RootCmd.Find([]string{"signup"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != signupCmd {
		t.Fatalf("Find(signup) = %q, want signupCmd", cmd.CommandPath())
	}
	if len(args) != 0 {
		t.Fatalf("Find leftover args: %v", args)
	}
	if signupCmd.RunE == nil || authLoginCmd.RunE == nil {
		t.Fatal("signup and auth login must use RunE")
	}
	if reflect.ValueOf(signupCmd.RunE).Pointer() != reflect.ValueOf(authLoginCmd.RunE).Pointer() {
		t.Fatal("signup must share auth login RunE")
	}
	alias, aliasArgs, err := RootCmd.Find([]string{"auth", "signup"})
	if err != nil {
		t.Fatal(err)
	}
	if alias != authLoginCmd {
		t.Fatalf("Find(auth signup) = %q, want auth login", alias.CommandPath())
	}
	if len(aliasArgs) != 0 {
		t.Fatalf("auth signup leftover args: %v", aliasArgs)
	}
	for _, name := range []string{"yes", "no-browser", "timeout"} {
		if signupCmd.Flags().Lookup(name) == nil {
			t.Errorf("signup missing login flag --%s", name)
		}
	}
}

func TestSignup_HelpNotesAuthLoginAlias(t *testing.T) {
	if !strings.Contains(strings.ToLower(signupCmd.Short), "alias") {
		t.Errorf("signup Short should say it is an alias: %s", signupCmd.Short)
	}
	if !strings.Contains(signupCmd.Long, "alias for auth login") {
		t.Errorf("signup Long should note it is an alias for auth login:\n%s", signupCmd.Long)
	}
	if !strings.Contains(authLoginCmd.Long, "signup is an alias") {
		t.Errorf("auth login Long should mention the signup alias:\n%s", authLoginCmd.Long)
	}

	stdout, stderr, code, err := executeAuth("signup", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	help := stdout + stderr
	if !strings.Contains(help, "alias") || !strings.Contains(help, "auth login") {
		t.Errorf("signup help should note it is an alias for auth login:\n%s", help)
	}
	if strings.Contains(help, "Full Name") || (strings.Contains(help, "email") && strings.Contains(help, "password")) {
		t.Errorf("signup help still describes the old TUI sign-up path:\n%s", help)
	}
}

func TestSignup_InstallsMachineCredentialViaAuthLogin(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	var opened []string
	openBrowserFn = func(u string) { opened = append(opened, u); completeTestBrowserLogin(u) }

	stdout, stderr, code, err := executeAuth("signup", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, "/oauth2/authorize?") {
		t.Errorf("expected authorization URI:\n%s", stdout)
	}

	cfg := readAuthConfig(t)
	if cfg["CRONITOR_API_KEY"] != authTestMachineKey {
		t.Errorf("stored key: %#v", cfg["CRONITOR_API_KEY"])
	}
	if cfg["CRONITOR_AUTH_MANAGED"] != true {
		t.Errorf("auth managed: %#v", cfg["CRONITOR_AUTH_MANAGED"])
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	var sawToken, sawCreate, sawLegacySignup bool
	for _, req := range fake.requests {
		switch {
		case req.Method == "POST" && req.Path == "/oauth2/token":
			sawToken = true
		case req.Method == "POST" && req.Path == "/api/cli/machine-credentials":
			sawCreate = true
		case strings.Contains(req.Path, "sign-up") || strings.Contains(req.Path, "signup"):
			sawLegacySignup = true
			t.Errorf("signup command hit old signup path: %s %s", req.Method, req.Path)
		}
	}
	if !sawToken {
		t.Fatal("signup did not exchange authorization code")
	}
	if !sawCreate {
		t.Fatal("signup did not POST machine-credentials")
	}
	if sawLegacySignup {
		t.Fatal("signup used the legacy website sign-up key mint")
	}
	if len(opened) != 1 {
		t.Errorf("browser: %#v", opened)
	}
}

func TestAuthLogin_PromptRequiredToReplace(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	if err := os.WriteFile(configFilePath(), []byte(`{"CRONITOR_API_KEY":"existing-key-not-from-auth","CRONITOR_HOSTNAME":"keep-me"}`), 0600); err != nil {
		t.Fatal(err)
	}
	viper.Set(varApiKey, "existing-key-not-from-auth")
	authYes = false
	readLineFn = func(string) (string, error) { return "n", nil }

	stdout, stderr, code, _ := executeAuth("auth", "login")
	if code == 0 {
		t.Fatal("expected cancel")
	}
	assertNoSecrets(t, stdout, stderr)
	cfg := readAuthConfig(t)
	if cfg["CRONITOR_API_KEY"] != "existing-key-not-from-auth" {
		t.Errorf("replaced without consent: %#v", cfg)
	}
	if cfg["CRONITOR_HOSTNAME"] != "keep-me" {
		t.Errorf("lost hostname: %#v", cfg)
	}
}

func TestAuthLogin_BrowserFallbackStillShowsAuthorizationURL(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	openBrowserFn = func(raw string) {
		fmt.Println("Failed to open browser: no display")
		completeTestBrowserLogin(raw)
	}

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, "code_challenge_method=S256") || !strings.Contains(stdout, "/oauth2/authorize?") {
		t.Errorf("fallback must still show authorization URI:\n%s", stdout)
	}
}

func TestAuthLogin_NoBrowserFlag(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	opened := 0
	openBrowserFn = func(string) { opened++ }

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes", "--no-browser")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	if opened != 0 {
		t.Fatalf("opened browser %d times", opened)
	}
	assertNoSecrets(t, stdout, stderr)
}

func TestAuthLogin_VerboseDebugDoesNotLeakSecrets(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	logPath := filepath.Join(t.TempDir(), "debug.log")
	viper.Set(varLog, logPath)
	verbose = true

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes", "--verbose")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	logData, _ := os.ReadFile(logPath)
	assertNoSecrets(t, stdout, stderr, string(logData))
}

func TestAuthLogin_PreservesExistingSettings(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	if err := os.WriteFile(configFilePath(), []byte(`{"CRONITOR_HOSTNAME":"keep-host","CRONITOR_ENV":"keep-env"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if _, _, code, err := executeAuth("auth", "login", "--yes"); err != nil || code != 0 {
		t.Fatalf("login failed code=%d err=%v", code, err)
	}
	cfg := readAuthConfig(t)
	if cfg["CRONITOR_HOSTNAME"] != "keep-host" || cfg["CRONITOR_ENV"] != "keep-env" {
		t.Errorf("lost settings: %#v", cfg)
	}
}

func TestAuthStatus_ShowsMetadataNeverKey(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	if _, _, code, err := executeAuth("auth", "login", "--yes"); err != nil || code != 0 {
		t.Fatalf("login failed")
	}
	fake.currentBody = `{"key":"` + authTestMachineKey + `","name":"cli-testhost","kind":"machine","scopes":["telemetry:write"],"organization":"Acme"}`

	stdout, stderr, code, err := executeAuth("auth", "status")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, "cli-testhost") || !strings.Contains(stdout, "Acme") {
		t.Errorf("status: %s", stdout)
	}
}

func TestAuthStatus_NotLoggedIn(t *testing.T) {
	_, cleanup := withAuthTest(t, newAuthFake())
	defer cleanup()

	stdout, stderr, code, _ := executeAuth("auth", "status")
	if code == 0 {
		t.Fatal("expected not logged in")
	}
	if !strings.Contains(stdout+stderr, "Not logged in") {
		t.Errorf("status: %s %s", stdout, stderr)
	}
}

func TestAuthLogout_RevokesAndClears(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	if err := os.WriteFile(configFilePath(), []byte(`{"CRONITOR_HOSTNAME":"keep-host","CRONITOR_ENV":"keep-env"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, code, err := executeAuth("auth", "login", "--yes"); err != nil || code != 0 {
		t.Fatal("login failed")
	}

	stdout, stderr, code, err := executeAuth("auth", "logout", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)

	cfg := readAuthConfig(t)
	if _, ok := cfg["CRONITOR_API_KEY"]; ok {
		t.Errorf("key still present: %#v", cfg)
	}
	if cfg["CRONITOR_HOSTNAME"] != "keep-host" {
		t.Errorf("lost hostname: %#v", cfg)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	var sawDelete bool
	for _, req := range fake.requests {
		if req.Method == "DELETE" && req.Path == "/api/cli/machine-credentials/current" {
			sawDelete = true
		}
	}
	if !sawDelete {
		t.Fatal("expected DELETE current")
	}
}

func TestAuthLogout_UnmanagedRequiresForce(t *testing.T) {
	_, cleanup := withAuthTest(t, newAuthFake())
	defer cleanup()

	viper.Set(varApiKey, "manual-key-not-from-auth")
	if err := os.WriteFile(configFilePath(), []byte(`{"CRONITOR_API_KEY":"manual-key-not-from-auth"}`), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes")
	if code == 0 {
		t.Fatal("expected failure without --force")
	}
	if !strings.Contains(stdout+stderr, "not installed by auth login") {
		t.Errorf("message: %s %s", stdout, stderr)
	}
}

const (
	authStoredKey = authTestMachineKey
	authEnvKey    = "cronitor_env_override_SECRET_eeee"
	authFlagKey   = "cronitor_flag_override_SECRET_ffff"
)

func writeManagedAuthConfig(t *testing.T, key, name string) {
	t.Helper()
	raw, err := json.Marshal(map[string]interface{}{
		"CRONITOR_API_KEY":                 key,
		"CRONITOR_AUTH_MANAGED":            true,
		"CRONITOR_MACHINE_CREDENTIAL_NAME": name,
		"CRONITOR_HOSTNAME":                "keep-host",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFilePath(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	rememberSecret(key)
}

func assertManagedKeyPreserved(t *testing.T, key string) {
	t.Helper()
	cfg := readAuthConfig(t)
	if cfg["CRONITOR_API_KEY"] != key {
		t.Fatalf("stored key changed: %#v", cfg["CRONITOR_API_KEY"])
	}
	if cfg["CRONITOR_AUTH_MANAGED"] != true {
		t.Fatalf("managed metadata cleared: %#v", cfg)
	}
}

func TestAuthLogout_EnvOverrideDoesNotRevokeWrongKey(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	// viper merge would prefer the env key; logout must not follow it.
	viper.Set(varApiKey, authEnvKey)
	viper.Set(varAuthManaged, true)
	viper.Set(varMachineCredentialName, authTestCredName)
	t.Setenv(varApiKey, authEnvKey)
	rememberSecret(authEnvKey)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes")
	if code == 0 {
		t.Fatalf("expected refusal, got success: %s %s", stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("promised logout despite env override: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "CRONITOR_API_KEY") || !strings.Contains(stdout+stderr, "does not match") {
		t.Errorf("expected override refusal: %s %s", stdout, stderr)
	}
	assertManagedKeyPreserved(t, authStoredKey)
	if users := fake.deleteUsers(); len(users) != 0 {
		t.Fatalf("DELETE under env override: %#v", users)
	}
}

func TestAuthLogout_FlagOverrideDoesNotRevokeWrongKey(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	viper.Set(varApiKey, authFlagKey)
	rememberSecret(authFlagKey)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes", "--api-key", authFlagKey)
	if code == 0 {
		t.Fatalf("expected refusal, got success: %s %s", stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("promised logout despite flag override: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "--api-key") || !strings.Contains(stdout+stderr, "does not match") {
		t.Errorf("expected flag refusal: %s %s", stdout, stderr)
	}
	assertManagedKeyPreserved(t, authStoredKey)
	if users := fake.deleteUsers(); len(users) != 0 {
		t.Fatalf("DELETE under flag override: %#v", users)
	}
}

func TestAuthLogout_MatchingEnvOverrideStillRevokesStoredKey(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	t.Setenv(varApiKey, authStoredKey)
	viper.Set(varApiKey, authStoredKey)
	viper.Set(varAuthManaged, true)

	stdout, stderr, code, err := executeAuth("auth", "logout", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Logged out") {
		t.Errorf("expected confirmed logout: %s %s", stdout, stderr)
	}
	cfg := readAuthConfig(t)
	if _, ok := cfg["CRONITOR_API_KEY"]; ok {
		t.Errorf("key still present: %#v", cfg)
	}
	if users := fake.deleteUsers(); len(users) != 1 || users[0] != authStoredKey {
		t.Fatalf("DELETE users %#v", users)
	}
}

func TestAuthLogout_403DoesNotClearLocalOrPromiseSuccess(t *testing.T) {
	fake := newAuthFake()
	fake.deleteCode = 403
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	viper.Set(varApiKey, authStoredKey)
	viper.Set(varAuthManaged, true)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes")
	if code == 0 {
		t.Fatal("expected failure on 403")
	}
	assertNoSecrets(t, stdout, stderr)
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("promised logout on 403: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "403") || !strings.Contains(stdout+stderr, "Local credential was not removed") {
		t.Errorf("expected unconfirmed revoke message: %s %s", stdout, stderr)
	}
	assertManagedKeyPreserved(t, authStoredKey)
}

func TestAuthLogout_404DoesNotClearLocalOrPromiseSuccess(t *testing.T) {
	fake := newAuthFake()
	fake.deleteCode = 404
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	viper.Set(varApiKey, authStoredKey)
	viper.Set(varAuthManaged, true)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes")
	if code == 0 {
		t.Fatal("expected failure on 404")
	}
	assertNoSecrets(t, stdout, stderr)
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("promised logout on 404: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "404") || !strings.Contains(stdout+stderr, "Local credential was not removed") {
		t.Errorf("expected unconfirmed revoke message: %s %s", stdout, stderr)
	}
	assertManagedKeyPreserved(t, authStoredKey)
}

func TestAuthLogout_401ClearsLocalWithoutClaimingRemoteRevoke(t *testing.T) {
	fake := newAuthFake()
	fake.deleteCode = 401
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	viper.Set(varApiKey, authStoredKey)
	viper.Set(varAuthManaged, true)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes")
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("must not claim remote logout on 401: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "no longer valid remotely") {
		t.Errorf("expected already-invalid message: %s %s", stdout, stderr)
	}
	cfg := readAuthConfig(t)
	if _, ok := cfg["CRONITOR_API_KEY"]; ok {
		t.Errorf("401 should clear a locally stored invalid key: %#v", cfg)
	}
}

func TestAuthLogout_ViperSetWithoutEnvStillRevokesStoredKey(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	// Ordinary SDK/viper override of the key, no flag and no process env.
	viper.Set(varApiKey, authEnvKey)
	viper.Set(varAuthManaged, true)
	rememberSecret(authEnvKey)

	stdout, stderr, code, err := executeAuth("auth", "logout", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Logged out") {
		t.Errorf("expected confirmed logout of the stored key: %s %s", stdout, stderr)
	}
	if users := fake.deleteUsers(); len(users) != 1 || users[0] != authStoredKey {
		t.Fatalf("DELETE must use the config key, not the viper override: %#v", users)
	}
	cfg := readAuthConfig(t)
	if _, ok := cfg["CRONITOR_API_KEY"]; ok {
		t.Errorf("stored key should be cleared: %#v", cfg)
	}
}

func TestAuthLogout_ForceDoesNotBypassEnvOverrideRefusal(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	t.Setenv(varApiKey, authEnvKey)
	viper.Set(varApiKey, authEnvKey)
	rememberSecret(authEnvKey)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes", "--force")
	if code == 0 {
		t.Fatalf("expected refusal, got success: %s %s", stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("force must not promise logout on override mismatch: %s %s", stdout, stderr)
	}
	assertManagedKeyPreserved(t, authStoredKey)
	if users := fake.deleteUsers(); len(users) != 0 {
		t.Fatalf("DELETE under env override with --force: %#v", users)
	}
}

func TestAuthLogout_EmptyEnvDoesNotBlockStoredLogout(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	t.Setenv(varApiKey, "")
	viper.Set(varApiKey, authStoredKey)
	viper.Set(varAuthManaged, true)

	stdout, stderr, code, err := executeAuth("auth", "logout", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	if users := fake.deleteUsers(); len(users) != 1 || users[0] != authStoredKey {
		t.Fatalf("DELETE users %#v", users)
	}
}

func TestAuthLogout_EnvOnlyKeyIsNotLoggedIn(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	t.Setenv(varApiKey, authEnvKey)
	viper.Set(varApiKey, authEnvKey)
	rememberSecret(authEnvKey)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes")
	if code != 0 {
		t.Fatalf("env-only key should be not-logged-in, exit %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "Not logged in") {
		t.Errorf("message: %s %s", stdout, stderr)
	}
	if users := fake.deleteUsers(); len(users) != 0 {
		t.Fatalf("must not DELETE an env-only key: %#v", users)
	}
}

func TestAuthLogout_ReadsCaseInsensitiveConfigKeys(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	raw := `{
		"cronitor_api_key": "` + authStoredKey + `",
		"cronitor_auth_managed": true,
		"cronitor_machine_credential_name": "` + authTestCredName + `"
	}`
	if err := os.WriteFile(configFilePath(), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	rememberSecret(authStoredKey)

	stdout, stderr, code, err := executeAuth("auth", "logout", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	if users := fake.deleteUsers(); len(users) != 1 || users[0] != authStoredKey {
		t.Fatalf("DELETE users %#v", users)
	}
}

func TestAuthLogout_ForceRemovesUnmanaged(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	if err := os.WriteFile(configFilePath(), []byte(`{"CRONITOR_API_KEY":"manual-key-not-from-auth","CRONITOR_HOSTNAME":"keep-host"}`), 0600); err != nil {
		t.Fatal(err)
	}
	viper.Set(varApiKey, "manual-key-not-from-auth")

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes", "--force")
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Logged out") {
		t.Errorf("expected local logout: %s %s", stdout, stderr)
	}
	if users := fake.deleteUsers(); len(users) != 0 {
		t.Fatalf("unmanaged --force must not DELETE remotely: %#v", users)
	}
	cfg := readAuthConfig(t)
	if _, ok := cfg["CRONITOR_API_KEY"]; ok {
		t.Errorf("key still present: %#v", cfg)
	}
	if cfg["CRONITOR_HOSTNAME"] != "keep-host" {
		t.Errorf("lost hostname: %#v", cfg)
	}
}

func TestAuthLogout_403ForceRemovesLocalOnly(t *testing.T) {
	fake := newAuthFake()
	fake.deleteCode = 403
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	writeManagedAuthConfig(t, authStoredKey, authTestCredName)
	viper.Set(varApiKey, authStoredKey)
	viper.Set(varAuthManaged, true)

	stdout, stderr, code, _ := executeAuth("auth", "logout", "--yes", "--force")
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if strings.Contains(stdout+stderr, "Logged out") {
		t.Fatalf("must not claim remote logout when force-clearing after 403: %s %s", stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "Remote revocation was not confirmed") {
		t.Errorf("expected local-only message: %s %s", stdout, stderr)
	}
	cfg := readAuthConfig(t)
	if _, ok := cfg["CRONITOR_API_KEY"]; ok {
		t.Errorf("force should remove local key: %#v", cfg)
	}
}

func TestAuthHelpListsSubcommands(t *testing.T) {
	_, cleanup := withAuthTest(t, newAuthFake())
	defer cleanup()

	stdout, _, code, err := executeAuth("auth", "--help")
	if err != nil || code != 0 {
		t.Fatalf("help failed code=%d err=%v", code, err)
	}
	for _, want := range []string{"login", "status", "logout"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %s:\n%s", want, stdout)
		}
	}
}

func testCallback(raw string) string {
	u, _ := url.Parse(raw)
	return lib.CLIAuthRedirectURI + "?code=" + authTestAuthCode + "&state=" + u.Query().Get("state")
}
func completeTestBrowserLogin(raw string) {
	resp, err := http.Get(testCallback(raw))
	if err == nil {
		resp.Body.Close()
	}
}

func TestAuthLoginTimeoutPreservesCredential(t *testing.T) {
	_, cleanup := withAuthTest(t, nil)
	defer cleanup()
	os.WriteFile(configFilePath(), []byte(`{"CRONITOR_API_KEY":"existing"}`), 0600)
	openBrowserFn = func(string) {}
	_, _, code, _ := executeAuth("auth", "login", "--yes", "--timeout", "20ms")
	if code == 0 {
		t.Fatal("expected timeout")
	}
	if readAuthConfig(t)["CRONITOR_API_KEY"] != "existing" {
		t.Fatal("replaced credential on timeout")
	}
	// Listener must have been released even when login timed out.
	openBrowserFn = completeTestBrowserLogin
	if _, _, code, _ := executeAuth("auth", "login", "--yes", "--timeout", "5s"); code != 0 {
		t.Fatal("listener not released")
	}
}

func TestAuthLoginRemoteRejectsInvalidCallbackWithoutPersisting(t *testing.T) {
	_, cleanup := withAuthTest(t, nil)
	defer cleanup()
	readCallbackFn = func(context.Context, string) (string, error) {
		return lib.CLIAuthRedirectURI + "?state=wrong&code=" + authTestAuthCode, nil
	}
	stdout, stderr, code, _ := executeAuth("auth", "login", "--yes", "--no-browser")
	if code == 0 {
		t.Fatal("invalid callback accepted")
	}
	assertNoSecrets(t, stdout, stderr)
	if _, err := os.Stat(configFilePath()); !os.IsNotExist(err) {
		t.Fatal("persisted invalid login")
	}
}

func TestAuthLoginInvalidLocalCallbackDoesNotCancelLogin(t *testing.T) {
	_, cleanup := withAuthTest(t, nil)
	defer cleanup()
	openBrowserFn = func(raw string) {
		bad := strings.Replace(testCallback(raw), "state=", "state=wrong", 1)
		resp, err := http.Get(bad)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Error("accepted invalid state")
		}
		completeTestBrowserLogin(raw)
	}
	if _, _, code, _ := executeAuth("auth", "login", "--yes"); code != 0 {
		t.Fatal("invalid state killed login")
	}
}

func TestAuthLoginDenialAndExchangeFailurePreserveExistingCredential(t *testing.T) {
	for _, remote := range []bool{false, true} {
		for _, denied := range []bool{false, true} {
			t.Run(fmt.Sprintf("remote=%v/denied=%v", remote, denied), func(t *testing.T) {
				fake := newAuthFake()
				if !denied {
					fake.tokenScript = []string{"denied"}
				}
				_, cleanup := withAuthTest(t, fake)
				defer cleanup()
				os.WriteFile(configFilePath(), []byte(`{"CRONITOR_API_KEY":"existing"}`), 0600)
				callback := func(raw string) string {
					value := testCallback(raw)
					if denied {
						value = strings.Replace(value, "code="+authTestAuthCode, "error=access_denied&error_description="+authTestAuthCode, 1)
					}
					return value
				}
				readCallbackFn = func(_ context.Context, raw string) (string, error) { return callback(raw), nil }
				openBrowserFn = func(raw string) {
					resp, err := http.Get(callback(raw))
					if err != nil {
						t.Error(err)
						return
					}
					resp.Body.Close()
				}
				args := []string{"auth", "login", "--yes", "--timeout", "1s"}
				if remote {
					args = append(args, "--no-browser")
				}
				stdout, stderr, code, _ := executeAuth(args...)
				if code == 0 {
					t.Fatal("failed login succeeded")
				}
				assertNoSecrets(t, stdout, stderr)
				if readAuthConfig(t)["CRONITOR_API_KEY"] != "existing" {
					t.Fatal("replaced credential")
				}
				fake.mu.Lock()
				defer fake.mu.Unlock()
				for _, r := range fake.requests {
					if r.Path == "/api/cli/machine-credentials" {
						t.Error("attempted credential creation after failure")
					}
				}
			})
		}
	}
}

func TestAuthLoginCreateFailurePreservesExistingCredential(t *testing.T) {
	fake := newAuthFake()
	fake.createStatus = 503
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()
	os.WriteFile(configFilePath(), []byte(`{"CRONITOR_API_KEY":"existing"}`), 0600)
	stdout, stderr, code, _ := executeAuth("auth", "login", "--yes")
	if code == 0 {
		t.Fatal("failed creation succeeded")
	}
	assertNoSecrets(t, stdout, stderr)
	if readAuthConfig(t)["CRONITOR_API_KEY"] != "existing" {
		t.Fatal("replaced credential")
	}
}

func TestAuthLoginRemoteNeedsNoListener(t *testing.T) {
	_, cleanup := withAuthTest(t, nil)
	defer cleanup()
	listener, err := net.Listen("tcp4", "127.0.0.1:8319")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if _, _, code, _ := executeAuth("auth", "login", "--yes", "--no-browser"); code != 0 {
		t.Fatal("remote login requires local listener")
	}
}

func TestCallbackInputEOFTimeoutAndNoEcho(t *testing.T) {
	for _, name := range []string{"eof", "timeout", "success", "too-long"} {
		t.Run(name, func(t *testing.T) {
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			original := os.Stdin
			os.Stdin = reader
			defer func() { os.Stdin = original; reader.Close(); writer.Close() }()
			switch name {
			case "eof":
				writer.Close()
			case "success":
				go func() { io.WriteString(writer, "callback-secret\n"); writer.Close() }()
			case "too-long":
				go func() { io.WriteString(writer, strings.Repeat("x", 20<<10)); writer.Close() }()
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			var value string
			var readErr error
			stdout, stderr := testutil.CaptureStdoutStderr(func() { value, readErr = readCallbackFromTerminal(ctx, "") })
			if name == "success" {
				if value != "callback-secret" || readErr != nil {
					t.Fatalf("input failed: %v", readErr)
				}
			} else if readErr == nil {
				t.Fatal("expected input failure")
			}
			if strings.Contains(stdout+stderr, "callback-secret") {
				t.Fatal("echoed callback")
			}
		})
	}
}
