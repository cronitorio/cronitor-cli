package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"github.com/manifoldco/promptui"
	"strings"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Run commands from a cron-like shell",
	Long: `
Cronitor shell allows you to run commands like cron does. Commands run from the prompt start from your home directory, with reduced shell functionality and no shared environment variables.

Example:
  $ cronitor shell
  ~ $ <enter any command here>
	`,

	Run: func(cmd *cobra.Command, args []string) {

		templates := &promptui.PromptTemplates{
			Prompt:  "{{ . }} ",
			Valid:   "{{ . }} ",
			Invalid: "{{ . | red }} ",
			Success: "{{ . }} ",
		}

		prompt := promptui.Prompt{
			Label:     "~ $",
			Templates: templates,
		}

		for {

			if result, err := prompt.Run(); err == nil {
				result = strings.TrimSpace(result)

				if result == "exit" {
					os.Exit(0)
				} else if result == "" {
					continue
				} else {
					startTime := makeStamp()

					// Cron runs from the home directory, so imply the same
					exitCode := RunCommand("cd ~ ; " + result, false,false)
					duration := formatStamp(makeStamp() - startTime)

					if exitCode == 0 {
						fmt.Println()
						printSuccessText(fmt.Sprintf("✔ Command successful    Elasped time %ss", duration))
					} else {
						printErrorText(fmt.Sprintf("✗ Command failed    Elapsed time %ss    Exit code %d", duration, exitCode))
					}
				}
				fmt.Println()

			} else if err == promptui.ErrInterrupt {
				fmt.Println("Exited by user signal")
				os.Exit(-1)
			} else {
				fmt.Println("Error: " + err.Error() + "\n")
			}

			break
		}

	},
}

func init() {
	RootCmd.AddCommand(shellCmd)
}
