//go:build !windows

package cmd

import "syscall"

// execDashboard replaces this process, so the dashboard keeps the PID,
// signals, and exit status that `cronitor dash` had.
func execDashboard(path string, args, env []string) error {
	return syscall.Exec(path, append([]string{path}, args...), env)
}
