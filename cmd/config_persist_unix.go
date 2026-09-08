//go:build !windows

package cmd

import (
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
func persistReplaceFile(tmpName, dest string) error {
	// rename(2) replaces a regular file and does not follow a dest symlink
	// (it replaces the symlink inode). Callers reject unexpected symlinks
	// via Lstat before we get here.
	return os.Rename(tmpName, dest)
}

// persistOpenExisting opens an existing config file for an in-place rewrite.
// O_NOFOLLOW backs up the Lstat symlink check against a race.
func persistOpenExisting(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0)
}

// configReadableByOthers reports group or other read bits on the file.
func configReadableByOthers(_ string, info os.FileInfo) bool {
	return info.Mode().Perm()&0044 != 0
}
