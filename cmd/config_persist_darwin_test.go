//go:build darwin

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// everyoneRead matches an explicit or inherited "everyone allow read" entry
// as ls -le prints them: "everyone allow read" or "everyone inherited allow read".
var everyoneRead = regexp.MustCompile(`everyone (inherited )?allow read`)

func grantsEveryoneRead(t *testing.T, path string) bool {
	t.Helper()
	return everyoneRead.MatchString(lsACL(t, path))
}

func lsACL(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command("/bin/ls", "-le", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ls -le: %v\n%s", err, out)
	}
	return string(out)
}

func TestDarwinACL_ReadOnPlainFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, has, err := readACL(path)
	if err != nil {
		t.Fatalf("readACL on a file without an ACL must not fail: %v", err)
	}
	if has {
		t.Fatal("fresh file reported an ACL")
	}
	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"h"}`)); err != nil {
		t.Fatalf("plain save failed: %v", err)
	}
}

func TestDarwinACL_ExplicitGrantSurvivesSaveAndRestrictDropsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput(); err != nil {
		t.Skipf("chmod +a unavailable: %v %s", err, out)
	}
	if !grantsEveryoneRead(t, path) {
		t.Skip("ACL not applied by chmod +a on this volume")
	}

	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"h"}`)); err != nil {
		t.Fatalf("save with ACL failed: %v", err)
	}
	if !grantsEveryoneRead(t, path) {
		t.Errorf("explicit grant lost across a normal save:\n%s", lsACL(t, path))
	}

	if err := persistConfigFileMode(path, []byte(`{"CRONITOR_HOSTNAME":"h"}`), true); err != nil {
		t.Fatalf("--restrict failed: %v", err)
	}
	if grantsEveryoneRead(t, path) {
		t.Errorf("--restrict left the grant in place:\n%s", lsACL(t, path))
	}
}

func TestDarwinACL_InheritedDirectoryGrantIsNotAdded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cronitor.json")
	if err := os.WriteFile(path, []byte(`{}`), 0640); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read,file_inherit", dir).CombinedOutput(); err != nil {
		t.Skipf("chmod +a unavailable: %v %s", err, out)
	}
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if !grantsEveryoneRead(t, probe) {
		t.Skip("directory ACL not inherited on this volume")
	}

	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"h"}`)); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if grantsEveryoneRead(t, path) {
		t.Errorf("save added an inherited grant the original never had:\n%s", lsACL(t, path))
	}
	newFile := filepath.Join(dir, "new.json")
	if err := persistConfigFile(newFile, []byte(`{}`)); err != nil {
		t.Fatalf("new file failed: %v", err)
	}
	if grantsEveryoneRead(t, newFile) {
		t.Errorf("new owner-only file carries an inherited grant:\n%s", lsACL(t, newFile))
	}
}
