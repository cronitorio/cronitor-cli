package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"sync"
	"os/exec"
	"errors"
	"strings"
	"os"
)

var monitorCode, command string
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with Cronitor monitoring.",
	Long:  ``,
	Args: func(cmd *cobra.Command, args []string) error {
		// We need to use raw os.Args so we can pass the wrapped command through unparsed
		var foundExec, foundCode bool
		var commandParts []string
		for _, arg := range os.Args {
			if foundExec && foundCode {
				commandParts = append(commandParts, arg)
				continue
			}

			if foundExec && !foundCode {
				monitorCode = arg
				foundCode = true
				continue
			}

			if strings.ToLower(arg) == "exec" {
				foundExec = true
				continue
			}
		}

		command = strings.Join(commandParts, " ")
		if len(monitorCode) < 1 || len(command) < 1 {
			return errors.New("A unique monitor code and cli command are required immediately after 'exec'")
		}
		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup
		wg.Add(1)

		if verbose {
			fmt.Println(fmt.Sprintf("Running command: %s", command))
		}

		go sendPing("run", monitorCode, "", &wg)

		output, err := exec.Command("sh", "-c", command).CombinedOutput()
		if noStdoutPassthru {
			output = []byte{}
		}

		if err == nil {
			wg.Add(1)
			go sendPing("complete", monitorCode, string(output), &wg)
		} else {
			wg.Add(1)
			go sendPing("fail", monitorCode, strings.TrimSpace(fmt.Sprintf("[%s] %s", err.Error(), output)), &wg)
		}

		wg.Wait()
	},
}

func init() {
	RootCmd.AddCommand(execCmd)
}
