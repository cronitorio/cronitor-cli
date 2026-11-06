//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var shellShimCmd = &cobra.Command{
	Use:                "shell-shim",
	DisableFlagParsing: true,
	Short:              "Crontab SHELL shim (invoked by shell-wrapper)",
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
	settings, job := lib.SplitShimSettings(rest)
	applyShimSettings(settings)
	return RunCommand(job, true, true, shell)
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
	fmt.Fprintln(os.Stderr, "shell-shim: expected: cronitor shell-shim -c 'CRONITOR=<key> <command>'")
}

// applyShimSettings applies CRONITOR_*=value words from a marker line, as the
// matching exec flags would. They are not passed to the job.
func applyShimSettings(settings []string) {
	for _, s := range settings {
		name, value, _ := strings.Cut(s, "=")
		if name == "CRONITOR_NO_STDOUT" {
			noStdoutPassthru = value == "1"
			continue
		}
		viper.Set(name, value)
	}
}
