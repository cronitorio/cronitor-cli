package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"os/user"
	"cronitor/lib"
	"github.com/olekukonko/tablewriter"
)

var listCmd = &cobra.Command{
	Use:   "list <optional path>",
	Short: "Search for and list all cron jobs",
	Long: `
Cronitor list scans your system (or your supplied path) for cron jobs and displays them in an easy to read table

Example:
  $ cronitor list
      > List cron jobs in your user crontab and system directory

  $ cronitor list /path/to/crontab
      > Instead of the user crontab, list the jobs in a provided a crontab file (or directory of crontabs)
	`,
	Args: func(cmd *cobra.Command, args []string) error {

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var username string
		if u, err := user.Current(); err == nil {
			username = u.Username
		}

		crontabs := []*lib.Crontab{}
		commands := []string{}

		if len(args) > 0 {
			// A supplied argument can be a specific file or a directory
			if isPathToDirectory(args[0]) {
				crontabs = lib.ReadCrontabsInDirectory(username, args[0], crontabs)
			} else {
				crontabs = lib.ReadCrontabFromFile(username, args[0], crontabs)
			}
		} else {
			// Without a supplied argument look at the user crontab and the system drop-in directory
			crontabs = lib.ReadCrontabFromFile(username, "", crontabs)
			crontabs = lib.ReadCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)
		}

		if len(crontabs) == 0 {
			printWarningText("No crontab files found")
			return
		}

		fmt.Println()
		for _, crontab := range crontabs {
			if len(crontab.Lines) == 0 {
				continue
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Schedule", "Command"})
			table.SetAutoWrapText(true)
			table.SetHeaderAlignment(3)
			table.SetColMinWidth(0, 17)
			table.SetColMinWidth(1, 100)

			for _, line := range crontab.Lines {
				if len(line.CommandToRun) == 0 {
					continue
				}

				table.Append([]string{line.CronExpression, line.CommandToRun})
				commands = append(commands, line.CommandToRun)
			}

			printSuccessText(fmt.Sprintf("► Reading %s", crontab.DisplayName()))
			table.Render()
			fmt.Println()
		}
	},
}

func init() {
	RootCmd.AddCommand(listCmd)
}
