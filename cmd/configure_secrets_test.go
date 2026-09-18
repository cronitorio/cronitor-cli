package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/spf13/viper"
)

func captureOutput(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	return testutil.CaptureStdoutStderr(fn)
}

func resetConfigureTestState(t *testing.T) {
	t.Helper()
	verbose = false
	persistTestHook = nil
	viper.Set(varApiKey, "")
	viper.Set(varPingApiKey, "")
	viper.Set(varDashUsername, "")
	viper.Set(varDashPassword, "")
	viper.Set(varHostname, "")
	viper.Set(varEnv, "")
	viper.Set(varExcludeText, []string{})
	viper.Set(varLog, "")
	viper.Set(varUsers, "")
	viper.Set(varAllowedIPs, "")
	viper.Set(varApiVersion, "")
	viper.Set(varMCPEnabled, false)
	viper.Set(varAuthManaged, false)
	viper.Set(varMachineCredentialName, "")
	resetAPIKeyFlag()
	viper.Set("CRONITOR_CORS_ALLOWED_ORIGINS", "")
	viper.Set("mcp_instances", nil)
	prevConfig := viper.GetString(varConfig)
	t.Cleanup(func() {
		verbose = false
		persistTestHook = nil
		// Leave no fake credentials in viper for later test files. Other
		// tests gate live API calls on the API key being set.
		viper.Set(varApiKey, "")
		viper.Set(varPingApiKey, "")
		viper.Set(varDashUsername, "")
		viper.Set(varDashPassword, "")
		viper.Set(varConfig, prevConfig)
		viper.Set("mcp_instances", nil)
	})
}

func TestConfigureOutput_RedactsAPIAndPingKeys(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	viper.Set(varConfig, path)
	viper.Set(varApiKey, testAPIKey)
	viper.Set(varPingApiKey, testPingKey)

	stdout, stderr := captureOutput(t, func() {
		configureCmd.Run(configureCmd, []string{})
	})
	combined := stdout + stderr
	if strings.Contains(combined, testAPIKey) {
		t.Errorf("configure printed API key.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if strings.Contains(combined, testPingKey) {
		t.Errorf("configure printed ping key.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "API Key:") || !strings.Contains(stdout, "Set") {
		t.Errorf("expected API Key: Set, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Ping API Key:") {
		t.Errorf("expected Ping API Key section, got:\n%s", stdout)
	}
}

func TestConfigureOutput_RedactsDashboardPassword(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	viper.Set(varConfig, path)
	viper.Set(varDashUsername, "dash-user")
	viper.Set(varDashPassword, testDashPass)

	stdout, stderr := captureOutput(t, func() {
		configureCmd.Run(configureCmd, []string{})
	})
	combined := stdout + stderr
	if strings.Contains(combined, testDashPass) {
		t.Errorf("configure printed dashboard password.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "********") {
		t.Errorf("expected masked dashboard password, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "dash-user") {
		t.Errorf("expected dashboard username to remain visible, got:\n%s", stdout)
	}
}

func TestConfigureOutput_VerboseDoesNotDumpSecrets(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	t.Setenv("CRONITOR_API_KEY", testAPIKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", testAWSSecret)
	viper.Set(varConfig, path)
	viper.Set(varApiKey, testAPIKey)
	verbose = true

	stdout, stderr := captureOutput(t, func() {
		configureCmd.Run(configureCmd, []string{})
	})
	combined := stdout + stderr
	if strings.Contains(combined, testAPIKey) {
		t.Error("verbose configure leaked CRONITOR_API_KEY value")
	}
	if strings.Contains(combined, testAWSSecret) {
		t.Error("verbose configure leaked AWS secret value")
	}
	if strings.Contains(combined, "AWS_SECRET_ACCESS_KEY") {
		t.Error("verbose configure listed unrelated AWS_SECRET_ACCESS_KEY")
	}
	if strings.Contains(combined, "Enviornment") {
		t.Error("old misspelled environment dump heading should be gone")
	}
	if !strings.Contains(stdout, "CRONITOR_API_KEY: Set") {
		t.Errorf("expected allowlisted env name, got:\n%s", stdout)
	}
}

func TestConfigureOutput_DoesNotPrintMCPPasswords(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	mcpPass := "mcp-instance-password-not-real"
	viper.Set(varConfig, path)
	viper.Set("mcp_instances", map[string]interface{}{
		"default": map[string]interface{}{
			"url":      "http://127.0.0.1:9000",
			"username": "mcp-user",
			"password": mcpPass,
		},
	})

	stdout, stderr := captureOutput(t, func() {
		configureCmd.Run(configureCmd, []string{})
	})
	combined := stdout + stderr
	if strings.Contains(combined, mcpPass) {
		t.Errorf("configure printed MCP instance password.\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "http://127.0.0.1:9000") {
		t.Errorf("expected MCP URL in output, got:\n%s", stdout)
	}
}

func TestConfigure_WritesNewKeyWithoutDroppingSettings(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	viper.Set(varConfig, path)
	viper.Set(varApiKey, testAPIKey)
	viper.Set(varHostname, "keep-hostname")
	viper.Set(varEnv, "keep-env")
	viper.Set(varExcludeText, []string{"/keep/exclude"})

	_, stderr := captureOutput(t, func() {
		configureCmd.Run(configureCmd, []string{})
	})
	if stderr != "" && strings.Contains(stderr, "ERROR") {
		t.Fatalf("configure failed: %s", stderr)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got ConfigFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ApiKey != testAPIKey {
		t.Errorf("api key: got %q", got.ApiKey)
	}
	if got.Hostname != "keep-hostname" {
		t.Errorf("hostname: got %q", got.Hostname)
	}
	if got.Env != "keep-env" {
		t.Errorf("env: got %q", got.Env)
	}
	if len(got.ExcludeText) != 1 || got.ExcludeText[0] != "/keep/exclude" {
		t.Errorf("exclude-text: got %#v", got.ExcludeText)
	}
}

func TestInitConfig_ReportsUnreadableConfigFile(t *testing.T) {
	prev := viper.GetString(varConfig)
	t.Cleanup(func() {
		viper.Set(varConfig, prev)
		viper.SetConfigFile("")
	})

	// A directory at the config path exists but cannot be read as a file.
	dir := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	viper.Set(varConfig, dir)
	_, stderr := captureOutput(t, initConfig)
	if !strings.Contains(stderr, "could not read") || !strings.Contains(stderr, dir) {
		t.Errorf("expected an unreadable-config notice naming the path, stderr:\n%s", stderr)
	}

	// A missing file is the normal case for a fresh install and stays silent.
	missing := filepath.Join(t.TempDir(), "missing.json")
	viper.Set(varConfig, missing)
	_, stderr = captureOutput(t, initConfig)
	if stderr != "" {
		t.Errorf("missing config file must stay silent, stderr:\n%s", stderr)
	}
}

func TestInitConfig_UnreadableWarningDoesNotEchoFileContents(t *testing.T) {
	prev := viper.GetString(varConfig)
	t.Cleanup(func() {
		viper.Set(varConfig, prev)
		viper.SetConfigFile("")
	})

	// Malformed JSON that still contains a secret-looking value. Parser
	// diagnostics must not reproduce any of the file's text.
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_API_KEY": "`+testAPIKey+`" trailing garbage`), 0600); err != nil {
		t.Fatal(err)
	}
	viper.Set(varConfig, path)
	_, stderr := captureOutput(t, initConfig)
	if !strings.Contains(stderr, "could not read") || !strings.Contains(stderr, path) {
		t.Errorf("expected unreadable-config warning naming the path, stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, testAPIKey) || strings.Contains(stderr, "trailing garbage") {
		t.Errorf("warning echoed file contents:\n%s", stderr)
	}

	// Same check through a parser that quotes source lines in its errors.
	yamlPath := filepath.Join(t.TempDir(), "cronitor.yaml")
	if err := os.WriteFile(yamlPath, []byte("CRONITOR_API_KEY: "+testAPIKey+"\n  bad: [unclosed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	viper.Set(varConfig, yamlPath)
	_, stderr = captureOutput(t, initConfig)
	if strings.Contains(stderr, testAPIKey) || strings.Contains(stderr, "unclosed") {
		t.Errorf("warning echoed yaml file contents:\n%s", stderr)
	}
}

func writeManagedConfigureFile(t *testing.T, path, key, name string) {
	t.Helper()
	raw, err := json.MarshalIndent(map[string]interface{}{
		"CRONITOR_API_KEY":                 key,
		"CRONITOR_AUTH_MANAGED":            true,
		"CRONITOR_MACHINE_CREDENTIAL_NAME": name,
		"CRONITOR_HOSTNAME":                "keep-host",
	}, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func readConfigureFile(t *testing.T, path string) ConfigFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got ConfigFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func executeConfigure(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	oldExit := exitFn
	code = 0
	exitFn = func(c int) { panic(exitSentinel(c)) }
	defer func() { exitFn = oldExit }()

	RootCmd.SetArgs(args)
	stdout, stderr = captureOutput(t, func() {
		defer func() {
			if rec := recover(); rec != nil {
				if c, ok := rec.(exitSentinel); ok {
					code = int(c)
					return
				}
				panic(rec)
			}
		}()
		if err := RootCmd.Execute(); err != nil && code == 0 {
			t.Errorf("execute: %v", err)
		}
	})
	return stdout, stderr, code
}

func TestConfigure_EnvSuppliedKeyClearsManagedMetadata(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	envKey := "cronitor_env_configure_SECRET_not-real"
	writeManagedConfigureFile(t, path, testAPIKey, "cli-testhost")

	t.Setenv(varApiKey, envKey)
	viper.Set(varConfig, path)
	viperUnset(varApiKey, varAuthManaged, varMachineCredentialName, varHostname, varEnv, varPingApiKey)
	viper.AutomaticEnv()

	if got := viper.GetString(varApiKey); got != envKey {
		t.Fatalf("viper did not take real env key, got %q", got)
	}

	stdout, stderr, code := executeConfigure(t, "configure", "--config", path)
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}

	got := readConfigureFile(t, path)
	if got.ApiKey != envKey {
		t.Errorf("persisted key %q, want env key", got.ApiKey)
	}
	if got.AuthManaged {
		t.Error("managed metadata should be cleared when configure persists an env-supplied key")
	}
	if got.MachineCredentialName != "" {
		t.Errorf("stale credential name kept: %q", got.MachineCredentialName)
	}
	if got.Hostname != "keep-host" {
		t.Errorf("lost hostname: %q", got.Hostname)
	}
}

func TestConfigure_EnvSameKeyKeepsManagedMetadata(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	writeManagedConfigureFile(t, path, testAPIKey, "cli-testhost")

	t.Setenv(varApiKey, testAPIKey)
	viper.Set(varConfig, path)
	viperUnset(varApiKey, varAuthManaged, varMachineCredentialName, varHostname, varEnv, varPingApiKey)
	viper.AutomaticEnv()

	stdout, stderr, code := executeConfigure(t, "configure", "--config", path)
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}

	got := readConfigureFile(t, path)
	if got.ApiKey != testAPIKey {
		t.Errorf("key: %q", got.ApiKey)
	}
	if !got.AuthManaged {
		t.Error("same env key must keep managed metadata")
	}
	if got.MachineCredentialName != "cli-testhost" {
		t.Errorf("name: %q", got.MachineCredentialName)
	}
}

func TestConfigure_ApiKeyFlagClearsManagedMetadata(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	writeManagedConfigureFile(t, path, testAPIKey, "cli-testhost")
	viper.Set(varConfig, path)
	viper.Set(varApiKey, testAPIKey)
	viper.Set(varAuthManaged, true)
	viper.Set(varMachineCredentialName, "cli-testhost")
	viper.Set(varHostname, "keep-host")

	stdout, stderr, code := executeConfigure(t, "configure", "--config", path, "--api-key", testAPIKey)
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stdout, stderr)
	}

	got := readConfigureFile(t, path)
	if got.ApiKey != testAPIKey {
		t.Errorf("key: %q", got.ApiKey)
	}
	if got.AuthManaged {
		t.Error("--api-key should clear managed metadata even when the value matches")
	}
	if got.MachineCredentialName != "" {
		t.Errorf("stale name kept: %q", got.MachineCredentialName)
	}
}
