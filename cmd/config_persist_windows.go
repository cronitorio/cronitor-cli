//go:build windows

package cmd

import (
	"os"
	"os/user"
	"strings"

	"golang.org/x/sys/windows"
)

// persistApplyMode restricts a freshly created temp file before any secret
// is written to it. Unix mode bits mean nothing here; a temp file in
// %ProgramData% would otherwise inherit that folder's ACL, which usually
// grants BUILTIN\Users read, for the duration of the write.
func persistApplyMode(_ *os.File, tmpName string, _ os.FileMode) error {
	return applyOwnerOnlyACL(tmpName)
}

func persistLockdownNewFile(path string) error {
	return applyOwnerOnlyACL(path)
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

// persistClearACL is not needed on Windows: the owner-only DACL applied to
// the temp file is protected, so nothing is inherited from the directory.
func persistClearACL(_ string) error { return nil }

// persistCloneAccess copies the existing file's DACL onto the replacement so
// a save does not change who can read the configuration.
func persistCloneAccess(tmpName, src string, _ os.FileInfo) error {
	sd, err := windows.GetNamedSecurityInfo(src, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
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

// configReadableByOthers reports an allow ACE for Everyone, Users, or
// Authenticated Users. An unreadable ACL is treated as not shared so the
// notice never fires on a guess.
func configReadableByOthers(path string, _ os.FileInfo) bool {
	shared, err := windowsGrantsWorldRead(path)
	return err == nil && shared
}
