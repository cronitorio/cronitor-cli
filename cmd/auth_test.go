package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	authTestDeviceCode  = "DEVICE_POLL_CODE_SECRET_do_not_print"
	authTestUserCode    = "WD-TEST-42"
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
	case method == "POST" && path == "/oauth2/device_authorization":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"device_code":%q,
			"user_code":%q,
			"verification_uri":"https://login.example/device",
			"verification_uri_complete":"https://login.example/device?user_code=%s",
			"expires_in":300,
			"interval":5
		}`, authTestDeviceCode, authTestUserCode, authTestUserCode)
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
		case "pending":
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"authorization_pending"}`)
		case "slow_down":
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"slow_down"}`)
		case "denied":
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"access_denied"}`)
		case "expired":
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"expired_token"}`)
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
	openBrowserFn = func(string) {}
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
	for _, secret := range []string{authTestDeviceCode, authTestAccessToken, authTestRefreshTok, authTestMachineKey} {
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
	openBrowserFn = func(u string) { opened = append(opened, u) }

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, authTestUserCode) {
		t.Errorf("expected user_code in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "https://login.example/device") {
		t.Errorf("expected verification URI:\n%s", stdout)
	}
	if !strings.Contains(stdout, authTestCredName) {
		t.Errorf("expected credential name:\n%s", stdout)
	}
	if len(opened) != 1 || !strings.Contains(opened[0], authTestUserCode) {
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
	for _, leak := range []string{authTestDeviceCode, authTestAccessToken, authTestRefreshTok} {
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
	_, cleanup := withAuthTest(t, newAuthFake())
	defer cleanup()

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
	if strings.Contains(help, "Full Name") || strings.Contains(help, "email") && strings.Contains(help, "password") {
		t.Errorf("signup help still describes the old TUI sign-up path:\n%s", help)
	}
}

func TestSignup_InstallsMachineCredentialViaAuthLogin(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	var opened []string
	openBrowserFn = func(u string) { opened = append(opened, u) }

	stdout, stderr, code, err := executeAuth("signup", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, authTestUserCode) {
		t.Errorf("expected user_code in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "https://login.example/device") {
		t.Errorf("expected verification URI:\n%s", stdout)
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
	var sawDevice, sawCreate, sawLegacySignup bool
	for _, req := range fake.requests {
		switch {
		case req.Method == "POST" && req.Path == "/oauth2/device_authorization":
			sawDevice = true
		case req.Method == "POST" && req.Path == "/api/cli/machine-credentials":
			sawCreate = true
		case strings.Contains(req.Path, "sign-up") || strings.Contains(req.Path, "signup"):
			sawLegacySignup = true
			t.Errorf("signup command hit old signup path: %s %s", req.Method, req.Path)
		}
	}
	if !sawDevice {
		t.Fatal("signup did not POST device_authorization")
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

func TestAuthLogin_PendingThenSlowDownThenSuccess(t *testing.T) {
	fake := newAuthFake()
	fake.tokenScript = []string{"pending", "slow_down", "ok"}
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	var sleeps []time.Duration
	sleepFn = func(d time.Duration) { sleeps = append(sleeps, d) }

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if len(sleeps) < 2 {
		t.Fatalf("expected polls to sleep, got %#v", sleeps)
	}
	if sleeps[0] != 5*time.Second {
		t.Errorf("first interval: %s", sleeps[0])
	}
	if sleeps[1] != 10*time.Second {
		t.Errorf("slow_down should add 5s, got %s", sleeps[1])
	}
}

func TestAuthLogin_AccessDenied(t *testing.T) {
	fake := newAuthFake()
	fake.tokenScript = []string{"denied"}
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	stdout, stderr, code, _ := executeAuth("auth", "login", "--yes")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout+stderr, "denied") {
		t.Errorf("expected denial message: %s %s", stdout, stderr)
	}
	if _, err := os.Stat(configFilePath()); err == nil {
		raw, _ := os.ReadFile(configFilePath())
		if strings.Contains(string(raw), authTestMachineKey) {
			t.Fatal("denied login stored a key")
		}
	}
}

func TestAuthLogin_ExpiredToken(t *testing.T) {
	fake := newAuthFake()
	fake.tokenScript = []string{"expired"}
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	stdout, stderr, code, _ := executeAuth("auth", "login", "--yes")
	if code == 0 {
		t.Fatal("expected non-zero exit")
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(strings.ToLower(stdout+stderr), "expired") {
		t.Errorf("expected expiry message: %s %s", stdout, stderr)
	}
}

func TestAuthLogin_ExpiresInStopsPolling(t *testing.T) {
	fake := newAuthFake()
	fake.tokenScript = []string{"pending", "pending", "pending"}
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	start := time.Unix(1_700_000_000, 0)
	nowFn = func() time.Time { return start }
	sleepFn = func(time.Duration) {
		start = start.Add(10 * time.Minute)
		nowFn = func() time.Time { return start }
	}

	stdout, stderr, code, _ := executeAuth("auth", "login", "--yes")
	if code == 0 {
		t.Fatal("expected timeout/expiry")
	}
	assertNoSecrets(t, stdout, stderr)
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

func TestAuthLogin_BrowserFallbackStillShowsCode(t *testing.T) {
	fake := newAuthFake()
	_, cleanup := withAuthTest(t, fake)
	defer cleanup()

	openBrowserFn = func(string) {
		fmt.Println("Failed to open browser: no display")
	}

	stdout, stderr, code, err := executeAuth("auth", "login", "--yes")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}
	assertNoSecrets(t, stdout, stderr)
	if !strings.Contains(stdout, authTestUserCode) || !strings.Contains(stdout, "https://login.example/device") {
		t.Errorf("fallback must still show URI and user_code:\n%s", stdout)
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
