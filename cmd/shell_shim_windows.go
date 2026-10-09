//go:build windows

package cmd

import (
	"fmt"
	"os"
)

// RunShellShim is Unix-only. Windows sync keeps cronitor exec lines.
func RunShellShim(args []string) int {
	fmt.Fprintln(os.Stderr, "shell-shim is unix-only")
	return 1
}
