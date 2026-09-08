//go:build !windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
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

func TestPersistConfigFileMode_RestrictNarrowsExisting0644(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr := captureOutput(t, func() {
		if err := persistConfigFileMode(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), true); err != nil {
			t.Errorf("restricted persist should succeed: %v", err)
		}
	})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("restricted save left mode %#o, want 0600", perm)
	}
	if strings.Contains(stderr, "readable by other users") {
		t.Errorf("no shared-access notice expected after --restrict, stderr:\n%s", stderr)
	}
}

func TestPersistConfigFile_SharedReadableNotice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}

	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared","CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
			t.Errorf("persist should succeed: %v", err)
		}
	})
	for _, want := range []string{"readable by other users", "--restrict", "CRONITOR_API_KEY"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in shared-access notice, stderr:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "WARNING") {
		t.Errorf("this is a notice, not a warning, stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, testAPIKey) {
		t.Error("notice leaked API key")
	}
}

func TestPersistConfigFile_NewFileNotice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, []byte(`{"CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"owner-only", "other users", "CRONITOR_API_KEY", "chmod"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("expected %q in new-file notice, stderr:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "readable by other users") {
		t.Errorf("new owner-only file must not get the shared-access notice, stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, testAPIKey) {
		t.Error("new-file notice leaked API key")
	}
}

func TestPersistConfigFile_RestrictedSaveIsQuiet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"owned"}`)); err != nil {
		t.Fatal(err)
	}
	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"owned","CRONITOR_ENV":"prod"}`)); err != nil {
			t.Fatal(err)
		}
	})
	if stderr != "" {
		t.Errorf("saving an existing 0600 file must print nothing, stderr:\n%s", stderr)
	}
}

func TestConfigure_RestrictFlagNarrowsExistingFile(t *testing.T) {
	resetConfigureTestState(t)
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	viper.Set(varConfig, path)
	configureRestrict = true
	t.Cleanup(func() { configureRestrict = false })

	captureOutput(t, func() {
		configureCmd.Run(configureCmd, []string{})
	})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("configure --restrict left mode %#o, want 0600", perm)
	}
}

func TestPersistConfigFile_ExistingFileKeepsOwnerGroupAndInode(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root to set up a file owned by another user")
	}
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared"}`), 0660); err != nil {
		t.Fatal(err)
	}
	// nobody:nogroup, group-writable: the shape of a shared team config.
	if err := os.Chown(path, 65534, 65534); err != nil {
		t.Skipf("cannot chown to nobody: %v", err)
	}
	if err := os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"shared","CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
		t.Fatalf("persist should succeed: %v", err)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	bs, as := before.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
	if as.Uid != bs.Uid || as.Gid != bs.Gid {
		t.Errorf("owner changed: before %d:%d after %d:%d", bs.Uid, bs.Gid, as.Uid, as.Gid)
	}
	if as.Ino != bs.Ino {
		t.Errorf("inode changed (%d -> %d); an existing file must be updated in place so ACLs and xattrs survive", bs.Ino, as.Ino)
	}
	if perm := after.Mode().Perm(); perm != 0660 {
		t.Errorf("mode changed to %#o, want 0660", perm)
	}
}
