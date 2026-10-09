//go:build windows

package cmd

import "syscall"

func exitStatusOf(status syscall.WaitStatus) int {
	return status.ExitStatus()
}
