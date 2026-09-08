//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"syscall"
)

// persistApplyMode sets Unix file mode on an open temp file.
func persistApplyMode(f *os.File, _ string, mode os.FileMode) error {
	return f.Chmod(mode)
}

func persistLockdownNewFile(path string) error {
	return os.Chmod(path, configModeOwnerOnly)
}

func persistReplaceFile(tmpName, dest string) error {
	// rename(2) replaces a regular file and does not follow a dest symlink
	// (it replaces the symlink inode). Callers reject unexpected symlinks
	// via Lstat before we get here.
	return os.Rename(tmpName, dest)
}

// persistCloneAccess gives the replacement file the access of the file it
// replaces: mode bits, owner and group, extended attributes, and an access
// ACL identical to the original's (or none). The mode is checked afterwards
// because applying an ACL can rewrite the group bits.
func persistCloneAccess(tmpName, src string, existing os.FileInfo) error {
	want := existing.Mode().Perm()
	if err := os.Chmod(tmpName, want); err != nil {
		return err
	}
	if err := persistCloneOwner(tmpName, existing); err != nil {
		return err
	}
	if err := persistCloneXattrs(src, tmpName); err != nil {
		return err
	}
	if err := persistSyncACL(src, tmpName); err != nil {
		return err
	}
	info, err := os.Stat(tmpName)
	if err != nil {
		return err
	}
	if got := info.Mode().Perm(); got != want {
		return fmt.Errorf("replacement file mode is %#o, want %#o", got, want)
	}
	return nil
}

// persistCloneOwner keeps the previous owner and group. Only root may give a
// file away, so a non-root writer falls back to keeping just the group, which
// is what shared access depends on. If even the group cannot be kept and the
// mode grants the group anything, the save is refused rather than silently
// cutting other group members off.
func persistCloneOwner(tmpName string, existing os.FileInfo) error {
	stat, ok := existing.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	uid, gid := int(stat.Uid), int(stat.Gid)
	if os.Chown(tmpName, uid, gid) == nil {
		return nil
	}
	if err := os.Chown(tmpName, -1, gid); err != nil {
		if existing.Mode().Perm()&0070 != 0 {
			return fmt.Errorf("cannot keep group %d on the replacement file, which other group members depend on: %w", gid, err)
		}
	}
	return nil
}

// configReadableByOthers reports group or other read bits on the file.
func configReadableByOthers(_ string, info os.FileInfo) bool {
	return info.Mode().Perm()&0044 != 0
}
