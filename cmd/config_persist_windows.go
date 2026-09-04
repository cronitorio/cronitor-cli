//go:build windows

package cmd

import (
	"os"
	"os/user"
	"strings"

	"golang.org/x/sys/windows"
)

// persistApplyMode is a no-op for Unix mode bits on Windows. Owner-restricted
// ACLs are applied via persistLockdownNewFile.
//
// Platform limitation: Windows does not honor 0600/0640 the way Unix does.
// os.Chmod only toggles the read-only attribute and is not used here.
func persistApplyMode(_ *os.File, _ string, _ os.FileMode) error {
	return nil
}

func persistLockdownNewFile(path string) error {
	return applyOwnerOnlyACL(path)
}

func persistPreserveOwner(_ string, _ os.FileInfo) error {
	// Windows replace keeps the destination name; owner-only ACL is applied
	// to the replacement file. The writing user is the new owner.
	return nil
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

// persistPreserveAccess copies the existing file's DACL onto the replacement
// so a save does not change who can read the configuration.
func persistPreserveAccess(tmpName, dest string, _ os.FileInfo) error {
	sd, err := windows.GetNamedSecurityInfo(dest, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(
		tmpName,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
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

func windowsACLBroaderThanOwner(path string) (bool, error) {
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

	sddl := strings.ToUpper(sd.String())
	return sddlContainsWorldAllow(sddl), nil
}

func windowsGrantsWorldRead(path string) (bool, error) {
	return windowsACLBroaderThanOwner(path)
}

func sddlContainsWorldAllow(sddl string) bool {
	// Look for allow ACEs granted to Everyone, Users, or Authenticated Users.
	for _, ace := range strings.Split(sddl, "(") {
		ace = strings.ToUpper(ace)
		if !strings.HasPrefix(ace, "A;") {
			continue
		}
		if strings.Contains(ace, ";;;WD)") || strings.Contains(ace, ";;;BU)") || strings.Contains(ace, ";;;AU)") {
			return true
		}
	}
	return false
}
