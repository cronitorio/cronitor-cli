//go:build !windows && !linux && !darwin

package cmd

// On FreeBSD and other Unix targets ACLs are not reachable through the xattr
// interface x/sys provides, so a save preserves mode, owner, and group but not
// an ACL. Documented limitation for those platforms.
func persistSyncACL(_, _ string) error { return nil }

func persistClearACL(_ string) error { return nil }
