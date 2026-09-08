//go:build darwin

package cmd

import "golang.org/x/sys/unix"

// macOS exposes a file's ACL through this extended attribute. listxattr does
// not report it, so it is handled by name.
const aclXattrName = "com.apple.system.Security"

// errNoACL is what getxattr returns when the file has no ACL.
var errNoACL = unix.ENOATTR
