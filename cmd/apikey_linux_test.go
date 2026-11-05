//go:build linux

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteOwnerOnlyFile_ClearsInheritedACL(t *testing.T) {
	dir := t.TempDir()
	grantNobodyReadDefaultACL(t, dir)
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if !hasAccessACL(t, probe) {
		t.Skip("default ACL not inherited on this filesystem")
	}

	newFile := filepath.Join(dir, "keys.json")
	if err := writeOwnerOnlyFile(newFile, []byte(`{"key":"secret"}`)); err != nil {
		t.Fatal(err)
	}
	if hasAccessACL(t, newFile) {
		t.Error("new owner-only key file inherited a directory ACL grant")
	}
	info, err := os.Stat(newFile)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("mode = %#o, want 0600", perm)
	}

	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeOwnerOnlyFile(existing, []byte(`{"key":"secret"}`)); err != nil {
		t.Fatal(err)
	}
	if hasAccessACL(t, existing) {
		t.Error("existing key file kept an inherited ACL grant")
	}
	info, err = os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("existing mode = %#o, want 0600", perm)
	}
}
