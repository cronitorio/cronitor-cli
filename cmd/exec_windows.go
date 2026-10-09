//go:build windows
// +build windows

package cmd

import (
	"os"
	"syscall"
)

// getPlatformSysProcAttr returns platform-specific SysProcAttr configuration
func getPlatformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Windows doesn't support Setpgid, so we return an empty SysProcAttr
		// Process group functionality is handled differently on Windows
	}
}

// getPlatformSysProcAttrForDash returns platform-specific SysProcAttr configuration for dash command
func getPlatformSysProcAttrForDash() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Windows doesn't support Setpgid, so we return an empty SysProcAttr
		// Process group functionality is handled differently on Windows
	}
}

func commitShimHandshake() {}

func execRealShell(string, string) int { return 127 }

func signalJob(proc *os.Process, sig os.Signal) {
	if proc != nil && sig != nil {
		_ = proc.Signal(sig)
	}
}
