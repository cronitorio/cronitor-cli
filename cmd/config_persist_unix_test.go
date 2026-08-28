//go:build !windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestPersistConfigFile_Existing0644RewrittenTo0600WithWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	original := []byte(`{"CRONITOR_HOSTNAME":"keep-me","CRONITOR_ENV":"keep-env"}`)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}

	updated := []byte(`{"CRONITOR_HOSTNAME":"keep-me","CRONITOR_ENV":"keep-env","CRONITOR_API_KEY":"test-api-key-not-real"}`)
	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, updated); err != nil {
			t.Errorf("0644 persist should succeed: %v", err)
		}
	})
	if !strings.Contains(stderr, "WARNING") {
		t.Errorf("expected loud warning on 0644 rewrite, stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "CRONITOR_API_KEY") {
		t.Errorf("warning should mention CRONITOR_API_KEY, stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, testAPIKey) {
		t.Error("warning leaked API key")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("rewritten 0644 file mode = %#o, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "keep-me") {
		t.Error("hostname did not survive 0644 rewrite")
	}
	if !strings.Contains(string(data), "keep-env") {
		t.Error("env did not survive 0644 rewrite")
	}
	if !strings.Contains(string(data), testAPIKey) {
		t.Error("new API key was not written")
	}
}

func TestPersistConfigFile_Existing0640RewrittenTo0600WithWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}

	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared","CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
			t.Errorf("0640 persist should succeed: %v", err)
		}
	})
	if !strings.Contains(stderr, "WARNING") {
		t.Errorf("expected warning when 0640 is narrowed to 0600, stderr:\n%s", stderr)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("rewritten 0640 file mode = %#o, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "shared") {
		t.Error("0640 file lost hostname")
	}
}

func TestPersistConfigFile_Existing0600DoesNotWarn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"owned"}`)); err != nil {
		t.Fatal(err)
	}

	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"owned","CRONITOR_ENV":"prod"}`)); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(stderr, "WARNING") {
		t.Errorf("did not expect access-narrowed warning for 0600 file, stderr:\n%s", stderr)
	}
}
