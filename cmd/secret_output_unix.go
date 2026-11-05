//go:build !windows

package cmd

import "syscall"

// secretFileOpenFlags adds O_NOFOLLOW so writeOwnerOnlyFile will not follow
// a symlink that appears after the Lstat check.
func secretFileOpenFlags() int {
	return syscall.O_NOFOLLOW
}
