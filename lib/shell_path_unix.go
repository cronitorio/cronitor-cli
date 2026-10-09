//go:build !windows

package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func pathWritable(path string) bool {
	return unix.Access(path, unix.W_OK) == nil
}

func warnShimBinaryOwnership(dest, bin string) {
	if !strings.HasPrefix(dest, "/etc/") {
		return
	}
	info, err := os.Stat(bin)
	if err != nil {
		return
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: %s is not root-owned; a shim installed under %s will not follow a non-root upgrade of that binary\n", bin, filepath.Dir(dest))
}
