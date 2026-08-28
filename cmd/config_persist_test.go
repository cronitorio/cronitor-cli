package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

const (
	testAPIKey    = "test-api-key-not-real"
	testPingKey   = "test-ping-key-not-real"
	testDashPass  = "test-dash-password-not-real"
	testAWSSecret = "fake-aws-secret-value-not-real"
)

func TestPersistConfigFile_PreservesNonSecretSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	first := ConfigFile{
		ApiKey:      "old-key-not-real",
		Hostname:    "keep-hostname",
		Env:         "keep-env",
		ExcludeText: []string{"/keep/exclude"},
	}
	b, err := json.MarshalIndent(first, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := persistConfigFile(path, b); err != nil {
		t.Fatalf("initial persist: %v", err)
	}

	first.ApiKey = testAPIKey
	b, err = json.MarshalIndent(first, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	if err := persistConfigFile(path, b); err != nil {
		t.Fatalf("update persist: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got ConfigFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
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
	if got.ApiKey != testAPIKey {
		t.Errorf("api key: got %q", got.ApiKey)
	}
}

func TestPersistConfigFile_AtomicFailureLeavesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	original := []byte(`{"CRONITOR_HOSTNAME":"original-valid"}`)
	if err := persistConfigFile(path, original); err != nil {
		t.Fatalf("initial persist: %v", err)
	}

	persistTestHook = func() error {
		return errors.New("simulated write failure")
	}
	defer func() { persistTestHook = nil }()

	err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"should-not-be-written"}`))
	if err == nil {
		t.Fatal("expected persist to fail")
	}
	if !strings.Contains(err.Error(), "CRONITOR_CONFIG") {
		t.Errorf("error should mention CRONITOR_CONFIG, got: %v", err)
	}
	if strings.Contains(err.Error(), testAPIKey) {
		t.Errorf("error leaked a key: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("original file changed after failed write:\n%s", data)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".cronitor-config-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestPersistConfigFile_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"CRONITOR_HOSTNAME":"via-symlink"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "cronitor.json")
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink not permitted: %v", err)
		}
		t.Fatal(err)
	}

	err := persistConfigFile(link, []byte(`{"CRONITOR_HOSTNAME":"replaced"}`))
	if err == nil {
		t.Fatal("expected symlink persist to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "symlink") {
		t.Errorf("expected symlink error, got: %v", err)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "via-symlink") {
		t.Errorf("symlink target was modified: %s", data)
	}
}

func TestFormatSignupPersistError_OmitsKeysAndSuggestsRecovery(t *testing.T) {
	err := formatSignupPersistError(errors.New("write failed"))
	msg := err.Error()
	for _, leak := range []string{testAPIKey, testPingKey, "--api-key " + testAPIKey, "sudo cronitor configure --api-key"} {
		if strings.Contains(msg, leak) {
			t.Errorf("signup persist error contained %q: %s", leak, msg)
		}
	}
	if !strings.Contains(msg, "account was created") {
		t.Errorf("expected account-created recovery text, got: %s", msg)
	}
	if !strings.Contains(msg, "https://cronitor.io") {
		t.Errorf("expected website sign-in guidance, got: %s", msg)
	}
	if !strings.Contains(msg, "CRONITOR_CONFIG") {
		t.Errorf("expected CRONITOR_CONFIG guidance, got: %s", msg)
	}
}

func TestSignupSuccessClientMessage_OmitsKeys(t *testing.T) {
	body := signupSuccessClientMessage()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(encoded)
	if strings.Contains(s, "api_key") || strings.Contains(s, "ping_api_key") {
		t.Errorf("signup client JSON must not include key fields: %s", s)
	}
	if strings.Contains(s, testAPIKey) || strings.Contains(s, testPingKey) {
		t.Errorf("signup client JSON leaked keys: %s", s)
	}
}

func TestSaveSignupCredentials_ErrorOmitsReturnedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-file")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}

	prev := viper.GetString(varConfig)
	viper.Set(varConfig, path)
	t.Cleanup(func() { viper.Set(varConfig, prev) })

	apiKey := "signup-api-key-must-not-leak"
	pingKey := "signup-ping-key-must-not-leak"
	err := saveSignupCredentials(apiKey, pingKey)
	if err == nil {
		t.Fatal("expected persist to fail when the config path is a directory")
	}
	wrapped := formatSignupPersistError(err)
	msg := wrapped.Error()
	if strings.Contains(msg, apiKey) || strings.Contains(msg, pingKey) {
		t.Fatalf("signup persist error leaked keys: %s", msg)
	}
	if !strings.Contains(msg, "could not be saved") {
		t.Errorf("expected safe recovery message, got: %s", msg)
	}
}

func TestPrintCronitorEnvSources_RedactsValuesAndUnrelatedNames(t *testing.T) {
	t.Setenv("CRONITOR_API_KEY", testAPIKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", testAWSSecret)

	stdout, stderr := captureOutput(t, printCronitorEnvSources)
	combined := stdout + stderr
	if strings.Contains(combined, testAPIKey) {
		t.Error("verbose env output leaked CRONITOR_API_KEY value")
	}
	if strings.Contains(combined, testAWSSecret) {
		t.Error("verbose env output leaked AWS secret value")
	}
	if strings.Contains(combined, "AWS_SECRET_ACCESS_KEY") {
		t.Error("verbose env output listed unrelated AWS_SECRET_ACCESS_KEY")
	}
	if !strings.Contains(stdout, "CRONITOR_API_KEY: Set") {
		t.Errorf("expected allowlisted name with Set, got:\n%s", stdout)
	}
}
