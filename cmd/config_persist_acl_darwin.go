//go:build darwin

package cmd

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// macOS keeps a file's ACL in a kauth_filesec blob reachable through
// getattrlist and setattrlist with ATTR_CMN_EXTENDED_SECURITY. That is the
// interface Apple's acl_get_file and acl_set_file use; the reserved
// com.apple.system.Security xattr returns EPERM and must not be used.

func extendedSecurityAttrlist() unix.Attrlist {
	return unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
}

func readACL(path string) ([]byte, bool, error) {
	p, err := syscall.BytePtrFromString(path)
	if err != nil {
		return nil, false, err
	}
	list := extendedSecurityAttrlist()
	// An ACL holds at most 128 entries of 24 bytes plus a 44-byte header.
	buf := make([]byte, 8192)
	_, _, errno := syscall.Syscall6(syscall.SYS_GETATTRLIST,
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&list)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unix.FSOPT_NOFOLLOW), 0)
	if errno != 0 {
		return nil, false, errno
	}
	return parseExtendedSecurity(buf)
}

func writeACL(path string, blob []byte) error {
	list := extendedSecurityAttrlist()
	return unix.Setattrlist(path, &list, encodeExtendedSecurity(blob), unix.FSOPT_NOFOLLOW)
}

// removeACL clears the ACL. A zero-length attribute asks the kernel to drop
// the ACL; if the file still reports one, a filesec carrying
// KAUTH_FILESEC_NOACL is written, which is the other form the kernel treats
// as "no ACL". The caller verifies the result either way.
func removeACL(path string) error {
	list := extendedSecurityAttrlist()
	if err := unix.Setattrlist(path, &list, encodeExtendedSecurity(nil), unix.FSOPT_NOFOLLOW); err == nil {
		if _, has, rerr := readACL(path); rerr == nil && !has {
			return nil
		}
	}
	return unix.Setattrlist(path, &list, encodeExtendedSecurity(noACLFilesec()), unix.FSOPT_NOFOLLOW)
}
