//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/cronitorio/cronitor-cli/lib"
)

// RunShellShim monitors a marked command exactly once. A panic before the
// handshake leaves the wrapper to run the command; telemetry recovers on its own.
func RunShellShim(args []string) int {
	command, ok := shellShimDashC(args)
	shell := lib.ResolveRealShell(os.Getenv(lib.RealShellEnv))
	if !ok {
		shellShimUsage()
		return 127
	}
	key, rest, monitored := lib.ParseMonitorMarker(command)
	if !monitored {
		return execRealShell(shell, command)
	}
	monitorCode = key
	code := RunCommandWithShell(rest, true, true, shell)
	if shellShimAfterJob != nil {
		shellShimAfterJob()
	}
	return code
}

// execRealShell replaces this process with `shell -c command`.
func execRealShell(shell, command string) int {
	argv := []string{shell, "-c", command}
	err := syscall.Exec(shell, argv, os.Environ())
	fmt.Fprintf(os.Stderr, "shell-shim: exec %s: %v\n", shell, err)
	return 127
}
