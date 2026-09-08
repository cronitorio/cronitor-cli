//go:build linux || darwin

package cmd

import (
	"bytes"
	"errors"
	"fmt"
)

// The access ACL is handled through three platform primitives: readACL
// returns the file's ACL blob and whether it has one, writeACL sets it, and
// removeACL deletes it. Linux uses the system.posix_acl_access xattr; macOS
// uses getattrlist/setattrlist with ATTR_CMN_EXTENDED_SECURITY, the
// interface Apple's acl_get_file and acl_set_file are built on.

// persistSyncACL makes dst's access ACL identical to src's: copied when src
// has one, removed when it does not (a temp file can inherit one from the
// directory). The result is read back and compared, so an ACL that cannot be
// reproduced fails the save instead of silently changing who can read the
// credentials.
func persistSyncACL(src, dst string) error {
	want, has, err := readACL(src)
	if err != nil {
		return fmt.Errorf("cannot read ACL of %s: %w", src, err)
	}
	if has {
		if err := writeACL(dst, want); err != nil {
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

// persistClearACL removes any access ACL from the file and confirms it is
// gone. A file that has none is left alone, so the common case makes no
// modifying call at all.
func persistClearACL(path string) error {
	_, has, err := readACL(path)
	if err != nil {
		return fmt.Errorf("cannot read ACL on the replacement file: %w", err)
	}
	if !has {
		return nil
	}
	if err := removeACL(path); err != nil {
		return fmt.Errorf("cannot remove inherited ACL from the replacement file: %w", err)
	}
	if _, has, err = readACL(path); err != nil {
		return fmt.Errorf("cannot verify ACL on the replacement file: %w", err)
	} else if has {
		return errors.New("inherited ACL is still present on the replacement file")
	}
	return nil
}
