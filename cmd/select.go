package cmd

import (
	"cronitor/lib"
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"os"
	"os/user"
)

var selectCmd = &cobra.Command{
	Use:   "select <optional path>",
	Short: "Select a cron job to run interactively",
	Long: `
Cronitor select starts by scanning your system (or your supplied path) for cron jobs. Use your arrow keys to select and execute a job from the list.

Example:
  $ cronitor select
      > List cron jobs in your user crontab, system crontab and crontab drop-in directory
      > Optionally, execute a job and view its output

  $ cronitor select /path/to/crontab
      > List cron jobs found in the provided path
      > Optionally, execute a job and view its output
	`,

	Run: func(cmd *cobra.Command, args []string) {
		var username string
		if u, err := user.Current(); err == nil {
			username = u.Username
		}

		crontabs := []*lib.Crontab{}
		commands := []string{}
		monitorCodes := map[string]string{}

		if len(args) > 0 {
			// A supplied argument can be a specific file or a directory
			if isPathToDirectory(args[0]) {
				crontabs = lib.ReadCrontabsInDirectory(username, args[0], crontabs)
			} else {
				crontabs = lib.ReadCrontabFromFile(username, args[0], crontabs)
			}
		} else {
			// Without a supplied argument look at the user crontab, system crontab and the system drop-in directory
			crontabs = lib.ReadCrontabFromFile(username, "", crontabs)
			crontabs = lib.ReadCrontabFromFile(username, lib.SYSTEM_CRONTAB, crontabs)
			crontabs = lib.ReadCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)
		}

		if len(crontabs) == 0 {
			printWarningText("No crontab files found", false)
			return
		}

		fmt.Println()
		for _, crontab := range crontabs {
			if len(crontab.Lines) == 0 {
				continue
			}

			for _, line := range crontab.Lines {
				if len(line.CommandToRun) == 0 {
					continue
				}

				commands = append(commands, line.CommandToRun)
				if len(line.Code) > 0 {
					monitorCodes[line.CommandToRun] = line.Code
				}
			}
		}

		prompt := promptui.Select{
			Label: "Select job to run",
			Items: unique(commands),
			Size:  20,
		}
		if _, result, err := prompt.Run(); err == nil {
			if result != "" {

				if _, exists := monitorCodes[result]; exists {
					monitorCode = monitorCodes[result]
				}

				printSuccessText("Running command: "+result, false)
				fmt.Println()

				startTime := makeStamp()
				exitCode := RunCommand(result, false, len(monitorCode) > 0)
				duration := formatStamp(makeStamp() - startTime)

				if exitCode == 0 {
					fmt.Println()
					printSuccessText(fmt.Sprintf("✔ Command successful    Elapsed time %ss", duration), false)
				} else {
					printErrorText(fmt.Sprintf("✗ Command failed    Elapsed time %ss    Exit code %d", duration, exitCode), false)
				}

				fmt.Println()
			} else {
				printDoneText("Done", false)
			}
		} else if err == promptui.ErrInterrupt {
			fmt.Println("Exited by user signal")
			os.Exit(-1)
			os.Exit(-1)
		} else {
			fmt.Println("Error: " + err.Error() + "\n")
		}
	},
}

func init() {
	RootCmd.AddCommand(selectCmd)
}

func unique(strings []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range strings {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}
