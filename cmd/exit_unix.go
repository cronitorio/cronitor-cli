//go:build !windows

package cmd

import "syscall"

func exitStatusOf(status syscall.WaitStatus) int {
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return status.ExitStatus()
}
