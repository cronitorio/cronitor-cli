//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// persistApplyMode sets Unix file mode on an open temp file. Windows modes
// are not meaningful here.
func persistApplyMode(f *os.File, _ string, mode os.FileMode) error {
	return f.Chmod(mode)
}

func persistApplyPathMode(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func persistLockdownNewFile(path string) error {
	return os.Chmod(path, configModeOwnerOnly)
}

func persistPreserveSecurity(tmpName, _ string, existing os.FileInfo) error {
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

func checkExistingConfigPerms(path string, info os.FileInfo) error {
	perm := info.Mode().Perm()
	if isAllowedConfigMode(perm) {
		return nil
	}
	// Never silently chmod an existing world-readable file, including the
	// shared system path /etc/cronitor/cronitor.json. Tightening that file
	// to 0600 would break non-root cron jobs that currently read it.
	return configOverlyPermissiveError(path, perm)
}

func existingConfigMode(info os.FileInfo) os.FileMode {
	return info.Mode().Perm()
}
