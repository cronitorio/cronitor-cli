//go:build linux

package cmd

import (
	"errors"

	"golang.org/x/sys/unix"
)

// Linux stores a file's POSIX access ACL in this extended attribute.
const aclXattrName = "system.posix_acl_access"

func readACL(path string) ([]byte, bool, error) {
	size, err := unix.Getxattr(path, aclXattrName, nil)
	if err != nil {
		if errors.Is(err, unix.ENODATA) {
			return nil, false, nil
		}
		return nil, false, err
	}
	buf := make([]byte, size)
	n, err := unix.Getxattr(path, aclXattrName, buf)
	if err != nil {
		if errors.Is(err, unix.ENODATA) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return buf[:n], true, nil
}

func writeACL(path string, blob []byte) error {
	return unix.Setxattr(path, aclXattrName, blob, 0)
}

func removeACL(path string) error {
	err := unix.Removexattr(path, aclXattrName)
	if errors.Is(err, unix.ENODATA) {
		return nil
	}
	return err
}
