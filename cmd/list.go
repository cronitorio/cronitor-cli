package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"github.com/manifoldco/promptui"
	"os/user"
	"cronitor/lib"
	"github.com/olekukonko/tablewriter"
	"strconv"
)

var listCmd = &cobra.Command{
	Use:   "list <optional path>",
	Short: "List cron jobs and optionally execute them from an interactive shell",
	Long: `
Cronitor discover will parse your crontab and create or update monitors using the Cronitor API.

Note: You must supply your Cronitor API key. This can be passed as a flag, environment variable, or saved in your Cronitor configuration file. See 'help configure' for more details.

Example:
  $ cronitor list
      > List cron jobs in your user crontab and system directory
      > Optionally, execute a job and view its output

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
				crontabs = readCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)
			} else {
				crontabs = readCrontabFromFile(username, "", crontabs)
			}
		} else {
			// Without a supplied argument look at the user crontab and the system drop-in directory
			crontabs = readCrontabFromFile(username, "", crontabs)
			crontabs = readCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)
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
			table.SetHeader([]string{"Line", "Schedule", "Command", "Has Monitoring"})
			table.SetAutoWrapText(false)
			table.SetHeaderAlignment(3)

			for _, line := range crontab.Lines {
				if len(line.CommandToRun) == 0 {
					continue
				}

				monitoring := "No"
				if len(line.Code) > 0 || line.HasLegacyIntegration() {
					monitoring = "Yes"
				}

				table.Append([]string{strconv.Itoa(len(commands)), line.CronExpression, line.CommandToRun, monitoring})
				commands = append(commands, line.CommandToRun)
			}

			printSuccessText(fmt.Sprintf("► Reading %s", crontab.DisplayName()))
			table.Render()
			fmt.Println()
		}

		prompt := promptui.Prompt{
			Label: "To run a cron job interactively, enter a line number",
		}

		if result, err := prompt.Run(); err == nil {
			if selectedCommand, err := strconv.ParseInt(result, 10, 0); err == nil {
				printSuccessText("► Running command: " + commands[selectedCommand])
				fmt.Println()


			} else {
				printSuccessText("✔ Done")
			}
		} else if err == promptui.ErrInterrupt {
			printSuccessText("✔ Done")
			os.Exit(-1)
		} else {
			fmt.Println("Error: " + err.Error() + "\n")
		}
	},
}

func readCrontabsInDirectory(username, directory string, crontabs []*lib.Crontab) []*lib.Crontab {
	files := lib.EnumerateCrontabFiles(directory)
	if len(files) > 0 {
		for _, crontabFile := range files {
			crontab := lib.CrontabFactory(username, crontabFile)
			crontab.Parse(true)
			crontabs = append(crontabs, crontab)
		}
	}

	return crontabs
}

func readCrontabFromFile(username, filename string, crontabs []*lib.Crontab) []*lib.Crontab {
	crontab := lib.CrontabFactory(username, filename)
	crontab.Parse(true)
	crontabs = append(crontabs, crontab)
	return crontabs
}

func init() {
	RootCmd.AddCommand(listCmd)
}
