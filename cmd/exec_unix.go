//go:build !windows
// +build !windows

package cmd

import (
	"os"
	"strconv"
	"syscall"
)

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

func commitShimHandshake() {
	raw := os.Getenv("CRONITOR_SHIM_FD")
	if raw == "" {
		return
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 0 {
		return
	}
	_, _ = syscall.Write(fd, []byte("1\n"))
	syscall.CloseOnExec(fd)
	_ = os.Unsetenv("CRONITOR_SHIM_FD")
}

func signalJob(proc *os.Process, sig os.Signal) {
	if proc == nil || sig == nil {
		return
	}
	_ = proc.Signal(sig)
	if s, ok := sig.(syscall.Signal); ok {
		_ = syscall.Kill(-proc.Pid, s)
	}
}
