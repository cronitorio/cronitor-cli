//go:build !windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestPersistConfigFile_NewFileIsOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"new"}`)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("new credential file mode = %#o, want 0600", perm)
	}
}

func TestPersistConfigFile_Existing0640Preserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}

	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared","CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
		t.Fatalf("0640 persist should succeed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0640 {
		t.Fatalf("existing 0640 file mode = %#o, want 0640", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "test-api-key-not-real") {
		t.Error("0640 file was not updated")
	}
	if !strings.Contains(string(data), "shared") {
		t.Error("0640 file lost hostname")
	}
}

func TestPersistConfigFile_Existing0644Rejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	original := []byte(`{"CRONITOR_HOSTNAME":"keep-me"}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}

	err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"changed"}`))
	if err == nil {
		t.Fatal("expected 0644 persist to be rejected")
	}
	msg := err.Error()
	if !strings.Contains(msg, "0644") && !strings.Contains(msg, "overly permissive") {
		t.Errorf("expected overly permissive error, got: %v", err)
	}
	if !strings.Contains(msg, "CRONITOR_CONFIG") {
		t.Errorf("expected CRONITOR_CONFIG migration text, got: %v", err)
	}
	if !strings.Contains(msg, "0640") {
		t.Errorf("expected 0640 migration option, got: %v", err)
	}
	if !strings.Contains(msg, "crontab") {
		t.Errorf("expected crontab env migration option, got: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Errorf("0644 file was modified: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Fatalf("rejected 0644 file mode changed to %#o", perm)
	}
}

func TestSaveSignupCredentials_ErrorOmitsReturnedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}

	prev := viper.GetString(varConfig)
	viper.Set(varConfig, path)
	t.Cleanup(func() { viper.Set(varConfig, prev) })

	apiKey := "signup-api-key-must-not-leak"
	pingKey := "signup-ping-key-must-not-leak"
	err := saveSignupCredentials(apiKey, pingKey)
	if err == nil {
		t.Fatal("expected persist to fail on 0644 file")
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
