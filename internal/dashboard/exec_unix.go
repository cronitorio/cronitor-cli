//go:build !windows

package dashboard

import "syscall"

// getPlatformSysProcAttrForDash returns platform-specific SysProcAttr configuration for dash command
func getPlatformSysProcAttrForDash() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true, // Create a new process group for each "run now" command
	}
}
