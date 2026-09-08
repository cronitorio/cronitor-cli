//go:build !windows

package cmd

import (
	"errors"
	"io/fs"
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

// persistPreserveOwner keeps the replaced file's owner and group when the
// writer is allowed to. A non-root user rewriting a group-writable file it
// does not own gets EPERM from chown; that write used to succeed in place,
// so ownership falls to the writer instead of failing the save.
func persistPreserveOwner(tmpName string, existing os.FileInfo) error {
	stat, ok := existing.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	err := os.Chown(tmpName, int(stat.Uid), int(stat.Gid))
	if err != nil && errors.Is(err, fs.ErrPermission) {
		return nil
	}
	return err
}

func persistReplaceFile(tmpName, dest string) error {
	// rename(2) replaces a regular file and does not follow a dest symlink
	// (it replaces the symlink inode). Callers reject unexpected symlinks
	// via Lstat before we get here.
	return os.Rename(tmpName, dest)
}

// persistPreserveAccess gives the replacement file the same mode bits as the
// file it is about to replace.
func persistPreserveAccess(tmpName, _ string, existing os.FileInfo) error {
	return os.Chmod(tmpName, existing.Mode().Perm())
}

// configReadableByOthers reports group or other read bits on the file.
func configReadableByOthers(_ string, info os.FileInfo) bool {
	return info.Mode().Perm()&0044 != 0
}
