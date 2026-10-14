//go:build windows

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteOwnerOnlyFile_WindowsACL(t *testing.T) {
	dir := t.TempDir()

	newFile := filepath.Join(dir, "keys.json")
	if err := writeOwnerOnlyFile(newFile, []byte(`{"key":"secret"}`)); err != nil {
		t.Fatal(err)
	}
	assertWindowsOwnerOnly(t, newFile)

	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := grantEveryoneRead(existing); err != nil {
		t.Skipf("could not grant Everyone read for test: %v", err)
	}
	world, err := windowsGrantsWorldRead(existing)
	if err != nil {
		t.Fatal(err)
	}
	if !world {
		t.Fatal("setup: existing file is not world-readable")
	}
	if err := writeOwnerOnlyFile(existing, []byte(`{"key":"secret"}`)); err != nil {
		t.Fatal(err)
	}
	assertWindowsOwnerOnly(t, existing)
}

func assertWindowsOwnerOnly(t *testing.T, path string) {
	t.Helper()
	world, err := windowsGrantsWorldRead(path)
	if err != nil {
		t.Fatal(err)
	}
	if world {
		t.Errorf("%s is readable by Everyone or Users", path)
	}
}
