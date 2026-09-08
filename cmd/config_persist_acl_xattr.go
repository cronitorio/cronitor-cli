//go:build linux || darwin

package cmd

import (
	"bytes"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// readACL returns the file's access ACL blob and whether it has one.
func readACL(path string) ([]byte, bool, error) {
	size, err := unix.Getxattr(path, aclXattrName, nil)
	if err != nil {
		if errors.Is(err, errNoACL) {
			return nil, false, nil
		}
		return nil, false, err
	}
	buf := make([]byte, size)
	n, err := unix.Getxattr(path, aclXattrName, buf)
	if err != nil {
		if errors.Is(err, errNoACL) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return buf[:n], true, nil
}

// persistSyncACL makes dst's access ACL identical to src's: copied when src
// has one, removed when it does not (a temp file can inherit one from the
// directory's default ACL). The result is read back and compared, so an ACL
// that cannot be reproduced fails the save instead of silently changing who
// can read the credentials.
func persistSyncACL(src, dst string) error {
	want, has, err := readACL(src)
	if err != nil {
		return fmt.Errorf("cannot read ACL of %s: %w", src, err)
	}
	if has {
		if err := unix.Setxattr(dst, aclXattrName, want, 0); err != nil {
			return fmt.Errorf("cannot preserve ACL on the replacement file: %w", err)
		}
	} else if err := persistClearACL(dst); err != nil {
		return err
	}
	got, gotHas, err := readACL(dst)
	if err != nil {
		return fmt.Errorf("cannot verify ACL on the replacement file: %w", err)
	}
	if gotHas != has || !bytes.Equal(got, want) {
		return errors.New("replacement file ACL does not match the original")
	}
	return nil
}

// persistClearACL removes any access ACL from the file and confirms it is gone.
func persistClearACL(path string) error {
	if err := unix.Removexattr(path, aclXattrName); err != nil && !errors.Is(err, errNoACL) {
		return fmt.Errorf("cannot remove inherited ACL from the replacement file: %w", err)
	}
	if _, has, err := readACL(path); err != nil {
		return fmt.Errorf("cannot verify ACL on the replacement file: %w", err)
	} else if has {
		return errors.New("inherited ACL is still present on the replacement file")
	}
	return nil
}
