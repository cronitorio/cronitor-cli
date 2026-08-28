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
	viper.Set("CRONITOR_CORS_ALLOWED_ORIGINS", "")
	viper.Set("mcp_instances", nil)
	t.Cleanup(func() {
		verbose = false
		persistTestHook = nil
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
