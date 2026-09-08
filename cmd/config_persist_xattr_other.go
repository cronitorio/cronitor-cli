//go:build !windows && !linux && !darwin && !freebsd

package cmd

// persistCloneXattrs is a no-op where x/sys offers no xattr interface. Mode,
// owner, and group are still preserved.
func persistCloneXattrs(_, _ string) error { return nil }
