//go:build windows

package cmd

// Windows has no O_NOFOLLOW; writeOwnerOnlyFile still refuses a symlink
// via Lstat.
func secretFileOpenFlags() int {
	return 0
}
