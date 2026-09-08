//go:build linux

package cmd

import "golang.org/x/sys/unix"

// Linux stores a file's POSIX access ACL in this extended attribute.
const aclXattrName = "system.posix_acl_access"

// errNoACL is what getxattr returns when the file has no access ACL.
var errNoACL = unix.ENODATA
