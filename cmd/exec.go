package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"sync"
	"os/exec"
	"errors"
	"strings"
	"os"
	"github.com/kballard/go-shellquote"
	"syscall"
)

var monitorCode string
var commandParts []string
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with Cronitor monitoring.",
	Long:  `
The supplied command will be executed and Cronitor will be notified of success or failure.

NB: Arguments supplied after the unique monitor code are treated as part of the command to execute. Flags intended for the 'exec' command must be passed before the monitor code.

Example:
  $ cronitor exec d3x0c1 /path/to/command.sh --command-param argument1 argument2
  This command will ping your Cronitor monitor d3x0c1 and execute the command '/path/to/command.sh --command-param argument1 argument2'

Example with no command output send to Cronitor:
  By default, stdout and stderr messages are sent to Cronitor when your job completes. To prevent any stdout output from being sent to cronitor, use the --no-stdout flag:
  $ cronitor exec --no-stdout d3x0c1 /path/to/command.sh --command-param argument1 argument2`,
	Args: func(cmd *cobra.Command, args []string) error {
		// We need to use raw os.Args so we can pass the wrapped command through unparsed
		var foundExec, foundCode bool
		for _, arg := range os.Args {
			// Treat anything that comes after the monitor code as the command to execute
			if foundCode {
				commandParts = append(commandParts,  strings.TrimSpace(arg))
				continue
			}

			// After finding "exec" we are looking for a monitor code, ignoring any flags
			if foundExec && !foundCode && !strings.HasPrefix(arg, "-") {
				monitorCode = arg
				foundCode = true
				continue
			}

			if strings.ToLower(arg) == "exec" {
				foundExec = true
				continue
			}
		}

		// Earlier in the application a `--` is parsed into the args after the `exec` command to
		// ensure that any flags passed to this command are not interpreted as flags to the cronitor app.
		// Remove that.
		if len(commandParts) > 0 && commandParts[0] == "--" {
			commandParts = commandParts[1:]
		}

		if len(monitorCode) < 1 || len(commandParts) < 1 {
			return errors.New("A unique monitor code and cli command are required immediately after 'exec'")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup
		wg.Add(1)

		startTime := makeStamp()
		formattedStartTime := formatStamp(startTime)
		go sendPing("run", monitorCode, "", formattedStartTime, startTime, nil, nil, &wg)

		subcommand := shellquote.Join(commandParts...)
		log(fmt.Sprintf("Running subcommand: %s", subcommand))

		outputForStdout, err := exec.Command("sh", "-c", subcommand).CombinedOutput()
		outputForPing := outputForStdout
		if noStdoutPassthru {
			outputForPing = []byte{}
		}

		endTime := makeStamp()
		duration := endTime - startTime
		exitCode := 0
		if err == nil {
			wg.Add(1)
			go sendPing("complete", monitorCode, string(outputForPing), formattedStartTime, endTime, &duration, &exitCode, &wg)
		} else {
			wg.Add(1)
			message := strings.TrimSpace(fmt.Sprintf("[%s] %s", err.Error(), outputForPing))

			// This works on both Unix and Windows. Although package syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has an ExitStatus() method with the same signature.
			// https://stackoverflow.com/questions/10385551/get-exit-code-go
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					exitCode = status.ExitStatus()
				} else {
					exitCode = 1
				}
			}

			go sendPing("fail", monitorCode, message, formattedStartTime, endTime, &duration, &exitCode, &wg)
		}

		fmt.Println(string(outputForStdout))
		wg.Wait()
	},
}

func init() {
	RootCmd.AddCommand(execCmd)
}
