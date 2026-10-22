//go:build windows

package cmd

import (
	"os"
	"os/exec"
)

func execDashboard(path string, args, env []string) error {
	cmd := exec.Command(path, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exited, ok := err.(*exec.ExitError); ok {
			exitFn(exited.ExitCode())
		}
		return err
	}
	return nil
}
