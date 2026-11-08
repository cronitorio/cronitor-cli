package cmd

import (
	"github.com/spf13/cobra"
)

// jobsCmd groups the commands that read or change cron jobs on this machine.
// sync, list, shell, and dash are only available here. The root command does
// not keep those names as aliases or hidden forwarders.
var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Sync, list, and run the cron jobs on this machine",
	Long: `Commands for the cron jobs on this machine.

  $ cronitor jobs sync
      > Add monitoring to new cron jobs and sync changes

  $ cronitor jobs list
      > List cron jobs

  $ cronitor jobs shell
      > Run commands from a cron-like shell

  $ cronitor jobs dash
      > Start the web dashboard

sync accepts the alias discover:
  $ cronitor jobs discover`,
	// Cobra 1.2 only reports an unknown command for the root. A parent with
	// subcommands and no Run prints help and exits 0. NoArgs runs once Run
	// makes the command runnable, so `jobs <typo>` fails the way a root typo does.
	// Bare `jobs` still prints help.
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	RootCmd.AddCommand(jobsCmd)
}
