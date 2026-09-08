//go:build darwin

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// persistCreateTemp makes the staging file next to the destination. When the
// save must keep the existing file's access, the temp is created with
// clonefile(2) so its ACL, extended attributes, and mode come along; macOS
// ACLs are not exposed through listxattr. On a volume without clonefile the
// temp falls back to a plain file and the xattr copy covers what it can.
func persistCreateTemp(path string, _ os.FileInfo, cloneFrom bool) (*os.File, error) {
	if cloneFrom {
		tmpName := filepath.Join(filepath.Dir(path), fmt.Sprintf(".cronitor-config-%d-%d.tmp", os.Getpid(), time.Now().UnixNano()))
		if err := unix.Clonefile(path, tmpName, unix.CLONE_NOFOLLOW); err == nil {
			f, err := os.OpenFile(tmpName, os.O_WRONLY|os.O_TRUNC, 0)
			if err != nil {
				_ = os.Remove(tmpName)
				return nil, err
			}
			return f, nil
		}
	}
	return os.CreateTemp(filepath.Dir(path), ".cronitor-config-*.tmp")
}
