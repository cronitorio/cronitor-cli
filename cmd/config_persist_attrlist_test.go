package cmd

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func attrlistResponse(offset int32, blob []byte) []byte {
	// uint32 total; attrreference{offset,len}; data at 4+offset
	buf := make([]byte, 12+len(blob))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(buf)))
	binary.LittleEndian.PutUint32(buf[4:8], uint32(offset))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(len(blob)))
	copy(buf[12:], blob)
	return buf
}

func TestParseExtendedSecurity(t *testing.T) {
	blob := []byte{0x6d, 0xc1, 0x2c, 0x01, 1, 2, 3, 4}
	got, has, err := parseExtendedSecurity(attrlistResponse(8, blob))
	if err != nil || !has || !bytes.Equal(got, blob) {
		t.Errorf("blob at offset 8: got %x has=%v err=%v", got, has, err)
	}

	_, has, err = parseExtendedSecurity(attrlistResponse(8, nil))
	if err != nil || has {
		t.Errorf("zero-length reference must mean no ACL, has=%v err=%v", has, err)
	}

	short := make([]byte, 4)
	binary.LittleEndian.PutUint32(short, 4)
	if _, has, err := parseExtendedSecurity(short); err != nil || has {
		t.Errorf("response without a reference must mean no ACL, has=%v err=%v", has, err)
	}

	if _, _, err := parseExtendedSecurity(attrlistResponse(64, blob)); err == nil {
		t.Error("reference pointing past the buffer must be rejected")
	}
}

func TestEncodeExtendedSecurity(t *testing.T) {
	blob := []byte{9, 8, 7}
	enc := encodeExtendedSecurity(blob)
	if len(enc) != 8+3 {
		t.Fatalf("len = %d", len(enc))
	}
	if binary.LittleEndian.Uint32(enc[0:4]) != 8 || binary.LittleEndian.Uint32(enc[4:8]) != 3 {
		t.Errorf("reference = %x", enc[:8])
	}
	if !bytes.Equal(enc[8:], blob) {
		t.Errorf("payload = %x", enc[8:])
	}
	// Round trip through the parser as the kernel would echo it back.
	resp := append([]byte{0, 0, 0, 0}, enc...)
	binary.LittleEndian.PutUint32(resp[0:4], uint32(len(resp)))
	got, has, err := parseExtendedSecurity(resp)
	if err != nil || !has || !bytes.Equal(got, blob) {
		t.Errorf("round trip: got %x has=%v err=%v", got, has, err)
	}

	empty := encodeExtendedSecurity(nil)
	if len(empty) != 8 || binary.LittleEndian.Uint32(empty[4:8]) != 0 {
		t.Errorf("empty encoding = %x", empty)
	}

	noacl := noACLFilesec()
	if binary.LittleEndian.Uint32(noacl[0:4]) != kauthFilesecMagic || binary.LittleEndian.Uint32(noacl[36:40]) != kauthFilesecNoACL {
		t.Errorf("noACL filesec = %x", noacl)
	}
}
