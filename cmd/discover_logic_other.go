//go:build !windows
// +build !windows

// This file provides stubs for Windows-only functions that will not be called on non-Windows architectures.
// Any function from here used in other files should be surrounded by:
// if runtime.GOOS == "windows" { }

package cmd

func processWindowsTaskScheduler() bool {
	return false
}
