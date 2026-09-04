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

func TestPersistConfigFile_Existing0644KeepsMode(t *testing.T) {
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
	if strings.Contains(stderr, "WARNING") {
		t.Errorf("must not warn about narrowing when the mode is preserved, stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, testAPIKey) {
		t.Error("stderr leaked API key")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Fatalf("existing 0644 file mode = %#o after save, want 0644 preserved", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"keep-me", "keep-env", testAPIKey} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected %q in rewritten file, got %s", want, data)
		}
	}
}

func TestPersistConfigFile_Existing0640KeepsMode(t *testing.T) {
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
		t.Fatalf("existing 0640 file mode = %#o after save, want 0640 preserved", perm)
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
