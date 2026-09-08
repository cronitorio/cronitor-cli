//go:build linux

package cmd

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// posixACL builds a POSIX ACL xattr blob: version 2 header then
// (tag u16, perm u16, id u32) entries, all little-endian.
func posixACL(entries ...[3]uint32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint32(2))
	for _, e := range entries {
		binary.Write(buf, binary.LittleEndian, uint16(e[0]))
		binary.Write(buf, binary.LittleEndian, uint16(e[1]))
		binary.Write(buf, binary.LittleEndian, e[2])
	}
	return buf.Bytes()
}

const (
	aclUserObj  = 0x01
	aclUser     = 0x02
	aclGroupObj = 0x04
	aclMask     = 0x10
	aclOther    = 0x20
	aclNoID     = 0xffffffff
	nobodyUID   = 65534
)

// grantNobodyReadDefaultACL puts a default ACL on dir so files created in it
// inherit "user:nobody:r--". Skips the test if the filesystem refuses.
func grantNobodyReadDefaultACL(t *testing.T, dir string) {
	t.Helper()
	def := posixACL(
		[3]uint32{aclUserObj, 6, aclNoID},
		[3]uint32{aclUser, 4, nobodyUID},
		[3]uint32{aclGroupObj, 4, aclNoID},
		[3]uint32{aclMask, 4, aclNoID},
		[3]uint32{aclOther, 0, aclNoID},
	)
	if err := unix.Setxattr(dir, "system.posix_acl_default", def, 0); err != nil {
		t.Skipf("POSIX ACLs unsupported here: %v", err)
	}
}

func hasAccessACL(t *testing.T, path string) bool {
	t.Helper()
	_, err := unix.Getxattr(path, "system.posix_acl_access", nil)
	if err == nil {
		return true
	}
	if err == unix.ENODATA {
		return false
	}
	t.Fatalf("getxattr: %v", err)
	return false
}

func TestPersistConfigFile_DoesNotInheritDirectoryDefaultACL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"h"}`), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	// The default ACL arrives after the file exists, so the original has no
	// access ACL but any new file in the directory inherits one.
	grantNobodyReadDefaultACL(t, dir)
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if !hasAccessACL(t, probe) {
		t.Skip("default ACL not inherited on this filesystem")
	}

	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"h","CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if hasAccessACL(t, path) {
		t.Error("replacement inherited an ACL grant the original never had")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0640 {
		t.Errorf("mode = %#o, want 0640", perm)
	}
}

func TestPersistConfigFile_NewAndRestrictedFilesCarryNoInheritedACL(t *testing.T) {
	dir := t.TempDir()
	grantNobodyReadDefaultACL(t, dir)
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if !hasAccessACL(t, probe) {
		t.Skip("default ACL not inherited on this filesystem")
	}

	newFile := filepath.Join(dir, "new.json")
	if err := persistConfigFile(newFile, []byte(`{"CRONITOR_API_KEY":"test-api-key-not-real"}`)); err != nil {
		t.Fatal(err)
	}
	if hasAccessACL(t, newFile) {
		t.Error("new owner-only file inherited a directory ACL grant")
	}

	existing := filepath.Join(dir, "existing.json")
	if err := os.WriteFile(existing, []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := persistConfigFileMode(existing, []byte(`{"CRONITOR_API_KEY":"test-api-key-not-real"}`), true); err != nil {
		t.Fatal(err)
	}
	if hasAccessACL(t, existing) {
		t.Error("--restrict left an inherited ACL grant on the file")
	}
}

func TestPersistConfigFile_CopiesExistingAccessACLExactly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(path, []byte(`{"CRONITOR_HOSTNAME":"h"}`), 0640); err != nil {
		t.Fatal(err)
	}
	access := posixACL(
		[3]uint32{aclUserObj, 6, aclNoID},
		[3]uint32{aclUser, 4, nobodyUID},
		[3]uint32{aclGroupObj, 4, aclNoID},
		[3]uint32{aclMask, 4, aclNoID},
		[3]uint32{aclOther, 0, aclNoID},
	)
	if err := unix.Setxattr(path, "system.posix_acl_access", access, 0); err != nil {
		t.Skipf("POSIX ACLs unsupported here: %v", err)
	}
	want := make([]byte, 256)
	n, err := unix.Getxattr(path, "system.posix_acl_access", want)
	if err != nil {
		t.Fatal(err)
	}
	want = want[:n]

	if err := persistConfigFile(path, []byte(`{"CRONITOR_HOSTNAME":"h","CRONITOR_ENV":"prod"}`)); err != nil {
		t.Fatalf("persist: %v", err)
	}
	got := make([]byte, 256)
	n, err = unix.Getxattr(path, "system.posix_acl_access", got)
	if err != nil {
		t.Fatalf("ACL missing after save: %v", err)
	}
	if !bytes.Equal(got[:n], want) {
		t.Errorf("ACL changed across save:\nwant %x\ngot  %x", want, got[:n])
	}
}
