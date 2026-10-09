//go:build !windows

package lib

import "golang.org/x/sys/unix"

func init() {
	pathWritableFn = func(path string) bool {
		return unix.Access(path, unix.W_OK) == nil
	}
}
