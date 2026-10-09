package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// shellShimCmd is what the wrapper execs: cronitor shell-shim -c '<command>'.
var shellShimCmd = &cobra.Command{
	Use:                "shell-shim",
	DisableFlagParsing: true,
	Short:              "Crontab SHELL shim (invoked by cronitor-shell)",
	Long: `Run one crontab command the way cron invokes a SHELL.

The wrapper execs this for a command that starts with MONITORIO=<key>.
That job is monitored like "cronitor exec" and run under $CRONITOR_REAL_SHELL,
or /bin/sh when that is unset, including when the real shell is /bin/sh.
A failure before the job starts still runs the job once. Ping and config
failures do not change the command's exit code.`,
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(RunShellShim(os.Args))
	},
}

func init() {
	RootCmd.AddCommand(shellShimCmd)
}

// shellShimAfterJob, when set, runs after a monitored job returns.
// Tests panic here to prove the job is not started a second time.
var shellShimAfterJob func()

func shellShimDashC(args []string) (string, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-c" {
			return args[i+1], true
		}
	}
	return "", false
}

func shellShimUsage() {
	fmt.Fprintln(os.Stderr, "shell-shim: expected: cronitor shell-shim -c '<command field>'")
}
