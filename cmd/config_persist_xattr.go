//go:build linux || darwin

package cmd

import (
	"errors"
	"strings"

	"golang.org/x/sys/unix"
)

// persistCloneXattrs copies extended attributes from src to dst, other than
// the access ACL, which persistSyncACL handles so that it can also remove an
// inherited one. Attributes the writer is not allowed to set, such as
// security.* labels, are skipped.
func persistCloneXattrs(src, dst string) error {
	names, err := listXattrs(src)
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			return nil
		}
		return err
	}
	for _, name := range names {
		if isACLXattr(name) {
			continue
		}
		value, err := getXattr(src, name)
		if err != nil {
			continue
		}
		// security.*, trusted.*, and similar need privileges we may lack.
		_ = unix.Setxattr(dst, name, value, 0)
	}
	return nil
}

func isACLXattr(name string) bool {
	return strings.HasPrefix(name, "system.posix_acl_") || name == "com.apple.system.Security"
}

func listXattrs(path string) ([]string, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	n, err := unix.Listxattr(path, buf)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, name := range strings.Split(string(buf[:n]), "\x00") {
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func getXattr(path, name string) ([]byte, error) {
	size, err := unix.Getxattr(path, name, nil)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, size)
	n, err := unix.Getxattr(path, name, buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}
