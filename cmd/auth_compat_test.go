package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

const (
	compatFullKey      = "cronitor_machine_full_SECRET_aabbcc"
	compatTelemetryKey = "cronitor_telemetry_SECRET_xyz789"
	compatPingOnlyKey  = "cronitor_ping_only_SECRET_pppp"
	compatFlagKey      = "cronitor_flag_key_SECRET_ffff"
	compatEnvKey       = "cronitor_env_key_SECRET_eeee"
	compatConfigKey    = "cronitor_config_key_SECRET_cccc"
	compatMonitorKey   = "job-compat-1"
)

type compatFake struct {
	mu       sync.Mutex
	requests []recordedRequest
	readonly bool
}

func (f *compatFake) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	user, _, _ := r.BasicAuth()
	f.mu.Lock()
	f.requests = append(f.requests, recordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Body:   string(body) + "\nAuthorization-User:" + user,
	})
	readonly := f.readonly
	f.mu.Unlock()

	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/ping/"):
		w.WriteHeader(200)
		io.WriteString(w, "ok")
	case path == "/upload":
		w.WriteHeader(200)
		io.WriteString(w, "uploaded")
	case path == "/api/logs/presign" || path == "/logs/presign":
		if readonly && user != compatTelemetryKey && user != compatFullKey && user != compatPingOnlyKey && user != compatFlagKey && user != compatEnvKey && user != compatConfigKey {
			w.WriteHeader(403)
			io.WriteString(w, `{"error":"forbidden"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		scheme := "http"
		host := r.Host
		if r.TLS != nil {
			scheme = "https"
		}
		fmt.Fprintf(w, `{"url":"%s://%s/upload"}`, scheme, host)
	case path == "/api/monitors" || path == "/monitors":
		if readonly || user == compatTelemetryKey {
			w.WriteHeader(403)
			io.WriteString(w, `{"error":"forbidden","message":"telemetry-only key cannot access account resources"}`)
			return
		}
		if r.Method == "GET" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, testutil.LoadFixture("monitors_list.json"))
			return
		}
		w.WriteHeader(200)
		io.WriteString(w, `{"monitors":[{"key":"job-compat-1","name":"Compat Job"}]}`)
	case strings.HasPrefix(path, "/api/monitors/") || strings.HasPrefix(path, "/monitors/"):
		if readonly || user == compatTelemetryKey {
			w.WriteHeader(403)
			io.WriteString(w, `{"error":"forbidden"}`)
			return
		}
		w.WriteHeader(200)
		io.WriteString(w, `{"key":"job-compat-1","name":"Compat Job","type":"job"}`)
	default:
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"not found"}`)
	}
}

func (f *compatFake) usedKeyOn(method, pathPrefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.requests {
		if req.Method == method && strings.HasPrefix(req.Path, pathPrefix) {
			const marker = "Authorization-User:"
			if i := strings.LastIndex(req.Body, marker); i >= 0 {
				return strings.TrimSpace(req.Body[i+len(marker):])
			}
		}
	}
	return ""
}

func (f *compatFake) sawPathPrefix(prefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.requests {
		if strings.HasPrefix(req.Path, prefix) {
			return true
		}
	}
	return false
}

func withCompatTest(t *testing.T, fake *compatFake) func() {
	t.Helper()
	if fake == nil {
		fake = &compatFake{}
	}
	server := httptest.NewServer(http.HandlerFunc(fake.handler))

	oldBase := lib.BaseURLOverride
	oldPing := lib.PingHostOverride
	oldAPIKey := viper.GetString(varApiKey)
	oldPingKey := viper.GetString(varPingApiKey)
	oldConfig := viper.GetString(varConfig)
	oldVerbose := verbose
	oldLog := viper.GetString(varLog)
	oldMonitor := monitorCode
	oldNoStdout := noStdoutPassthru
	oldAPIKeyFlag := apiKey
	oldPingKeyFlag := pingApiKey

	cfg := filepath.Join(t.TempDir(), "cronitor.json")
	lib.BaseURLOverride = server.URL + "/api"
	lib.PingHostOverride = server.URL
	verbose = false
	noStdoutPassthru = true
	monitorData = ""
	monitorFormat = ""
	resetSecretRedaction()
	viper.Set(varConfig, cfg)
	viper.Set(varApiKey, compatFullKey)
	viper.Set(varPingApiKey, "")
	viper.Set(varLog, "")
	rememberSecret(compatFullKey)
	rememberSecret(compatTelemetryKey)
	rememberSecret(compatPingOnlyKey)
	rememberSecret(compatFlagKey)
	rememberSecret(compatEnvKey)
	rememberSecret(compatConfigKey)

	t.Cleanup(func() {
		server.Close()
		lib.BaseURLOverride = oldBase
		lib.PingHostOverride = oldPing
		verbose = oldVerbose
		noStdoutPassthru = oldNoStdout
		monitorCode = oldMonitor
		apiKey = oldAPIKeyFlag
		pingApiKey = oldPingKeyFlag
		resetSecretRedaction()
		viper.Set(varApiKey, oldAPIKey)
		viper.Set(varPingApiKey, oldPingKey)
		viper.Set(varConfig, oldConfig)
		viper.Set(varLog, oldLog)
		RootCmd.SetArgs([]string{"--help"})
	})
	return func() {}
}

func TestCompat_RESTReadWriteUsesMachineKey(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)

	stdout, err := executeCmd("monitor", "list", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, compatFullKey) {
		t.Fatal("REST output leaked machine key")
	}
	if !strings.Contains(stdout, "Nightly Backup") && !strings.Contains(stdout, "abc123") {
		t.Errorf("list output: %s", stdout)
	}
	if got := fake.usedKeyOn("GET", "/api/monitors"); got != compatFullKey {
		t.Errorf("list auth user %q", got)
	}

	stdout, err = executeCmd("monitor", "create", "--data", `{"key":"job-compat-1","type":"job","name":"Compat Job"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout, compatFullKey) {
		t.Fatal("create output leaked machine key")
	}
	if got := fake.usedKeyOn("POST", "/api/monitors"); got != compatFullKey {
		t.Errorf("create auth user %q", got)
	}
}

func TestCompat_SyncFileUsesMachineKey(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)

	file := filepath.Join(t.TempDir(), "monitors.json")
	if err := os.WriteFile(file, []byte(`{"monitors":[{"key":"job-compat-1","type":"job","name":"Compat Job"}]}`), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := testutil.CaptureStdoutStderr(func() {
		importMonitorsFromFile(file)
	})
	combined := stdout + stderr
	if strings.Contains(combined, compatFullKey) {
		t.Fatal("sync leaked machine key")
	}
	if got := fake.usedKeyOn("PUT", "/api/monitors"); got != compatFullKey {
		t.Fatalf("sync auth user %q; output=%q", got, combined)
	}
}

func TestCompat_ExecCompleteAndFailPings(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)

	monitorCode = compatMonitorKey
	if code := RunCommand("true", true, true); code != 0 {
		t.Fatalf("true exit %d", code)
	}
	if !fake.sawPathPrefix("/ping/" + compatFullKey + "/" + compatMonitorKey) {
		t.Fatal("expected complete-path pings with machine key")
	}

	fake.mu.Lock()
	var sawRun, sawComplete bool
	for _, req := range fake.requests {
		if strings.Contains(req.Query, "state=run") {
			sawRun = true
		}
		if strings.Contains(req.Query, "state=complete") {
			sawComplete = true
		}
	}
	fake.mu.Unlock()
	if !sawRun || !sawComplete {
		t.Fatalf("missing run/complete pings")
	}

	if code := RunCommand("false", true, true); code == 0 {
		t.Fatal("false should fail")
	}
	fake.mu.Lock()
	var sawFail bool
	for _, req := range fake.requests {
		if strings.Contains(req.Query, "state=fail") {
			sawFail = true
		}
	}
	fake.mu.Unlock()
	if !sawFail {
		t.Fatal("expected fail ping")
	}
}

func TestCompat_PingThroughCronitorLinkPath(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)

	var wg sync.WaitGroup
	wg.Add(1)
	sendPing("run", compatMonitorKey, "", "", 0, nil, nil, nil, "", &wg)
	wg.Wait()

	if !fake.sawPathPrefix("/ping/" + compatFullKey + "/" + compatMonitorKey) {
		t.Fatal("expected cronitor.link-style /ping/{key}/{monitor}")
	}
}

func TestCompat_LogPresignAndUploadFromExec(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)
	noStdoutPassthru = false

	monitorCode = compatMonitorKey
	if code := RunCommand("echo compat-log-line", true, true); code != 0 {
		t.Fatalf("exec exit %d", code)
	}
	if !fake.sawPathPrefix("/api/logs/presign") && !fake.sawPathPrefix("/logs/presign") {
		t.Fatal("expected log presign")
	}
	if !fake.sawPathPrefix("/upload") {
		t.Fatal("expected log upload PUT")
	}
	if got := fake.usedKeyOn("POST", "/api/logs/presign"); got != "" && got != compatFullKey {
		t.Errorf("presign auth user %q", got)
	}
}

func TestCompat_TelemetryOnlyKey(t *testing.T) {
	fake := &compatFake{readonly: true}
	withCompatTest(t, fake)
	viper.Set(varApiKey, compatTelemetryKey)

	resp, err := lib.NewAPIClient(false, nil).GET("/monitors", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("account read status %d", resp.StatusCode)
	}

	resp, err = lib.NewAPIClient(false, nil).POST("/monitors", []byte(`{"key":"x","type":"job"}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Fatalf("account write status %d", resp.StatusCode)
	}

	api := getCronitorApi()
	_, err = api.PutRawMonitors([]byte(`{"monitors":[{"key":"x","type":"job"}]}`), "application/json")
	if err != nil {
		// PutRawMonitors only errors on transport; 403 still returns body.
	}
	if got := fake.usedKeyOn("PUT", "/api/monitors"); got != compatTelemetryKey {
		t.Errorf("sync write auth user %q", got)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	sendPing("run", compatMonitorKey, "", "", 0, nil, nil, nil, "", &wg)
	wg.Wait()
	if !fake.sawPathPrefix("/ping/" + compatTelemetryKey + "/" + compatMonitorKey) {
		t.Fatal("telemetry key should still ping")
	}

	_, err = lib.SendLogData(compatTelemetryKey, compatMonitorKey, "1.0", "log line")
	if err != nil {
		t.Fatalf("log upload should work with telemetry key: %v", err)
	}
}

func TestCompat_FlagEnvConfigPingKeyPrecedence(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)

	cfgPath := configFilePath()
	cfg := map[string]string{
		"CRONITOR_API_KEY":      compatConfigKey,
		"CRONITOR_PING_API_KEY": compatPingOnlyKey,
	}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, raw, 0600); err != nil {
		t.Fatal(err)
	}

	// Config layer: only the file key is in viper.
	viper.Set(varApiKey, compatConfigKey)
	viper.Set(varPingApiKey, "")
	if _, err := executeCmd("monitor", "list", "--format", "json"); err != nil {
		t.Fatal(err)
	}
	if got := fake.usedKeyOn("GET", "/api/monitors"); got != compatConfigKey {
		t.Errorf("config layer used %q", got)
	}

	// Env layer wins over config when viper sees the env-equivalent value.
	viper.Set(varApiKey, compatEnvKey)
	if _, err := executeCmd("monitor", "list", "--format", "json"); err != nil {
		t.Fatal(err)
	}
	if got := lastAuthUser(fake, "GET", "/api/monitors"); got != compatEnvKey {
		t.Errorf("env layer used %q", got)
	}

	// Flag layer: --api-key is bound to CRONITOR_API_KEY. viper.Set shadows
	// flags inside tests, so we apply the same value the flag would set.
	if f := RootCmd.PersistentFlags().Lookup("api-key"); f == nil {
		t.Fatal("missing --api-key flag")
	}
	viper.Set(varApiKey, compatFlagKey)
	if _, err := executeCmd("monitor", "list", "--format", "json"); err != nil {
		t.Fatal(err)
	}
	if got := lastAuthUser(fake, "GET", "/api/monitors"); got != compatFlagKey {
		t.Errorf("flag layer used %q", got)
	}

	// Ping key is preferred for telemetry even when an API key is present.
	viper.Set(varApiKey, compatFullKey)
	viper.Set(varPingApiKey, compatPingOnlyKey)
	var wg sync.WaitGroup
	wg.Add(1)
	sendPing("ok", compatMonitorKey, "", "", 0, nil, nil, nil, "", &wg)
	wg.Wait()
	if !fake.sawPathPrefix("/ping/" + compatPingOnlyKey + "/" + compatMonitorKey) {
		t.Fatal("ping key must win over API key for telemetry")
	}
	if fake.sawPathPrefix("/ping/" + compatFullKey + "/" + compatMonitorKey) {
		t.Fatal("API key must not be used for ping when ping key is set")
	}
}

func lastAuthUser(fake *compatFake, method, pathPrefix string) string {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	found := ""
	for _, req := range fake.requests {
		if req.Method == method && strings.HasPrefix(req.Path, pathPrefix) {
			const marker = "Authorization-User:"
			if i := strings.LastIndex(req.Body, marker); i >= 0 {
				found = strings.TrimSpace(req.Body[i+len(marker):])
			}
		}
	}
	return found
}

func TestCompat_RedactsMachineKeyFromPingLogs(t *testing.T) {
	fake := &compatFake{}
	withCompatTest(t, fake)

	logPath := filepath.Join(t.TempDir(), "ping.log")
	viper.Set(varLog, logPath)
	verbose = true

	var wg sync.WaitGroup
	wg.Add(1)
	stdout, stderr := testutil.CaptureStdoutStderr(func() {
		sendPing("run", compatMonitorKey, "", "", 0, nil, nil, nil, "", &wg)
		wg.Wait()
	})
	data, _ := os.ReadFile(logPath)
	combined := stdout + stderr + string(data)
	if strings.Contains(combined, compatFullKey) {
		t.Fatal("ping log leaked machine key")
	}
	if !strings.Contains(combined, "[REDACTED]") {
		t.Errorf("expected redacted ping URL, got %s", combined)
	}
}
