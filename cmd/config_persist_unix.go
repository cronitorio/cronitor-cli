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

func persistPreserveOwner(tmpName string, existing os.FileInfo) error {
	stat, ok := existing.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Chown(tmpName, int(stat.Uid), int(stat.Gid))
}

func persistReplaceFile(tmpName, dest string) error {
	// rename(2) replaces a regular file and does not follow a dest symlink
	// (it replaces the symlink inode). Callers reject unexpected symlinks
	// via Lstat before we get here.
	return os.Rename(tmpName, dest)
}

func existingAccessWillNarrow(_ string, info os.FileInfo) bool {
	// Group or other bits mean this save to 0600 will drop shared read access.
	return info.Mode().Perm()&0077 != 0
}
