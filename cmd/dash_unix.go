//go:build !windows

package cmd

import "syscall"

func execDashboard(path string, args, env []string) error {
	return syscall.Exec(path, append([]string{path}, args...), env)
}
