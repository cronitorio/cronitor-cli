//go:build !windows
// +build !windows

package cmd

import "syscall"

// getPlatformSysProcAttr returns platform-specific SysProcAttr configuration
func getPlatformSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true, // Put child in its own process group
	}
}

// getPlatformSysProcAttrForDash returns platform-specific SysProcAttr configuration for dash command
func getPlatformSysProcAttrForDash() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true, // Create a new process group for each "run now" command
	}
}
