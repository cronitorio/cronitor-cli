//go:build windows

package dashboard

import "syscall"

// getPlatformSysProcAttrForDash returns platform-specific SysProcAttr configuration for dash command
func getPlatformSysProcAttrForDash() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		// Windows doesn't support Setpgid, so we return an empty SysProcAttr
		// Process group functionality is handled differently on Windows
	}
}
