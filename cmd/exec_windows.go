//go:build windows
// +build windows

package cmd

import "syscall"

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
