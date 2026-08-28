//go:build windows

package cmd

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"golang.org/x/sys/windows"
)

// persistApplyMode is a no-op for Unix mode bits on Windows. Owner-restricted
// ACLs are applied via persistLockdownNewFile / persistPreserveSecurity.
//
// Platform limitation: Windows does not honor 0600/0640 the way Unix does.
// os.Chmod only toggles the read-only attribute and is not used here.
func persistApplyMode(_ *os.File, _ string, _ os.FileMode) error {
	return nil
}

func persistApplyPathMode(_ string, _ os.FileMode) error {
	return nil
}

func persistLockdownNewFile(path string) error {
	return applyOwnerOnlyACL(path)
}

func persistPreserveSecurity(tmpName, dest string, _ os.FileInfo) error {
	// Copy the existing DACL so a carefully set group-readable ACL is not
	// replaced with owner-only on update. New files use persistLockdownNewFile.
	return copyFileDACL(dest, tmpName)
}

func persistReplaceFile(tmpName, dest string) error {
	from, err := windows.UTF16PtrFromString(tmpName)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dest)
	if err != nil {
		return err
	}
	// os.Rename on Windows cannot replace an existing file. MoveFileEx with
	// MOVEFILE_REPLACE_EXISTING is the atomic replace equivalent.
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING)
}

func checkExistingConfigPerms(path string, _ os.FileInfo) error {
	world, err := windowsGrantsWorldRead(path)
	if err != nil {
		// Fail closed: if we cannot read the ACL we will not overwrite a
		// file that might be readable by other users.
		return fmt.Errorf("could not inspect ACL on %s: %w\n\n%s", path, err, configMigrationGuidance)
	}
	if world {
		return configOverlyPermissiveError(path, 0644)
	}
	return nil
}

func existingConfigMode(_ os.FileInfo) os.FileMode {
	// Unix mode bits are unused on Windows; ACL preservation happens separately.
	return configModeOwnerOnly
}

func applyOwnerOnlyACL(path string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// Protected DACL: full access for the current user, SYSTEM, and
	// Administrators. Administrators/SYSTEM retaining access is a Windows
	// platform limitation (analogous to root on Unix). Everyone and Users
	// are not granted access.
	sddl := "D:P(A;;FA;;;" + u.Uid + ")(A;;FA;;;SY)(A;;FA;;;BA)"
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

func copyFileDACL(from, to string) error {
	sd, err := windows.GetNamedSecurityInfo(from, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		to,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}

func windowsGrantsWorldRead(path string) (bool, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return false, err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return false, err
	}
	if dacl == nil {
		// A NULL DACL grants everyone full access.
		return true, nil
	}

	sddl := sd.String()
	// SDDL allow ACEs for Everyone (WD) or BUILTIN\Users (BU) mean the file
	// is readable beyond the owner — the Windows analog of 0644+.
	// Administrators (BA) and SYSTEM (SY) are expected and ignored.
	return sddlContainsWorldAllow(strings.ToUpper(sddl)), nil
}

func sddlContainsWorldAllow(sddl string) bool {
	// Look for allow ACEs granted to Everyone or Users.
	for _, ace := range strings.Split(sddl, "(") {
		ace = strings.ToUpper(ace)
		if !strings.HasPrefix(ace, "A;") {
			continue
		}
		if strings.Contains(ace, ";;;WD)") || strings.Contains(ace, ";;;BU)") {
			return true
		}
	}
	return false
}
