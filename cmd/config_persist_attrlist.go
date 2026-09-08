package cmd

import (
	"encoding/binary"
	"errors"
)

// getattrlist and setattrlist exchange variable-length attributes through an
// attrreference_t: int32 attr_dataoffset (from the attrreference itself) and
// uint32 attr_length. A getattrlist result starts with a uint32 total length.
// These helpers are pure so the layout is unit-tested on every platform.

const (
	kauthFilesecMagic = 0x012cc16d
	kauthFilesecNoACL = 0xffffffff
	filesecHeaderSize = 4 + 16 + 16 // magic, owner GUID, group GUID
)

// parseExtendedSecurity extracts the kauth_filesec blob from a getattrlist
// response for ATTR_CMN_EXTENDED_SECURITY. A zero-length reference means the
// file has no ACL.
func parseExtendedSecurity(buf []byte) ([]byte, bool, error) {
	if len(buf) < 4 {
		return nil, false, errors.New("getattrlist response too short")
	}
	total := int(binary.LittleEndian.Uint32(buf[0:4]))
	if total < 12 {
		return nil, false, nil
	}
	if total > len(buf) {
		return nil, false, errors.New("getattrlist response exceeds buffer")
	}
	offset := int(int32(binary.LittleEndian.Uint32(buf[4:8])))
	length := int(binary.LittleEndian.Uint32(buf[8:12]))
	if length == 0 {
		return nil, false, nil
	}
	start := 4 + offset
	if offset < 0 || start+length > total {
		return nil, false, errors.New("malformed attrreference in getattrlist response")
	}
	return append([]byte(nil), buf[start:start+length]...), true, nil
}

// encodeExtendedSecurity builds a setattrlist buffer: an attrreference at the
// start pointing at the blob immediately after it. Placing the reference
// first means the offset is the same whether the kernel counts from the
// reference or from the buffer. A nil blob encodes a zero-length attribute.
func encodeExtendedSecurity(blob []byte) []byte {
	out := make([]byte, 8+len(blob))
	if len(blob) > 0 {
		binary.LittleEndian.PutUint32(out[0:4], 8)
	}
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(blob)))
	copy(out[8:], blob)
	return out
}

// noACLFilesec is a kauth_filesec whose acl_entrycount is KAUTH_FILESEC_NOACL,
// the value Apple's acl_delete_file writes to remove an ACL.
func noACLFilesec() []byte {
	out := make([]byte, filesecHeaderSize+8)
	binary.LittleEndian.PutUint32(out[0:4], kauthFilesecMagic)
	binary.LittleEndian.PutUint32(out[filesecHeaderSize:], kauthFilesecNoACL)
	return out
}
