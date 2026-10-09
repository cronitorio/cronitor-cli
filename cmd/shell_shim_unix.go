//go:build !windows

package cmd

import (
	"fmt"
	"os"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var shellShimCmd = &cobra.Command{
	Use:                "shell-shim",
	DisableFlagParsing: true,
	Short:              "Crontab SHELL shim (invoked by cronitor-shell)",
	Args:               cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(RunShellShim(os.Args))
	},
}

func init() {
	RootCmd.AddCommand(shellShimCmd)
}

func RunShellShim(args []string) int {
	command, ok := shellShimDashC(args)
	shell := lib.ResolveRealShell(os.Getenv(lib.RealShellEnv))
	if !ok {
		shellShimUsage()
		return 127
	}
	key, rest, monitored := lib.ParseMonitorMarker(command)
	if !monitored {
		shellShimUsage()
		return 127
	}
	monitorCode = key
	code := RunCommand(rest, true, true, shell)
	if shellShimAfterJob != nil {
		shellShimAfterJob()
	}
	return code
}

func shellShimDashC(args []string) (string, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" {
			return args[i+1], true
		}
	}
	return "", false
}

func shellShimUsage() {
	fmt.Fprintln(os.Stderr, "shell-shim: expected: cronitor shell-shim -c 'MONITORIO=<key> <command>'")
}
