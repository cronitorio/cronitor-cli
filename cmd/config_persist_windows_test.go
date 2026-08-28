//go:build windows

package cmd

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPersistConfigFile_NewFileGetsOwnerACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"new"}`)); err != nil {
		t.Fatal(err)
	}
	world, err := windowsGrantsWorldRead(path)
	if err != nil {
		t.Fatal(err)
	}
	if world {
		t.Fatal("new Windows credential file is readable by Everyone or Users")
	}
}

func TestPersistConfigFile_WorldReadableRewrittenOwnerOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"keep-me"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := grantEveryoneRead(path); err != nil {
		t.Skipf("could not grant Everyone read for test: %v", err)
	}

	_, stderr := captureOutput(t, func() {
		if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"keep-me","CRONITOR_ENV":"keep-env"}`)); err != nil {
			t.Errorf("world-readable persist should succeed: %v", err)
		}
	})
	if !strings.Contains(stderr, "WARNING") {
		t.Errorf("expected warning when tightening Windows ACL, stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "scheduled task") && !strings.Contains(stderr, "Windows") {
		t.Errorf("Windows warning should be actionable, stderr:\n%s", stderr)
	}

	world, err := windowsGrantsWorldRead(path)
	if err != nil {
		t.Fatal(err)
	}
	if world {
		t.Fatal("rewritten Windows credential file is still readable by Everyone or Users")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "keep-me") {
		t.Errorf("hostname did not survive rewrite: %s", data)
	}
}

func grantEveryoneRead(path string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// Allow Everyone read plus owner/admin full — analog of 0644.
	sddl := "D:P(A;;FA;;;" + u.Uid + ")(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;WD)"
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}
