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
	"runtime"
	"io/ioutil"
	"regexp"
	"os/signal"
	"bytes"
	"bufio"
	"io"
)


var monitorCode string
var commandParts []string
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with monitoring",
	Long:  `
The supplied command will be executed and Cronitor will be notified of success or failure.

Note: Arguments supplied after the unique monitor code are treated as part of the command to execute. Flags intended for the 'exec' command must be passed before the monitor code.

Example:
  $ cronitor exec d3x0c1 /path/to/command.sh --command-param argument1 argument2
  This command will ping your Cronitor monitor d3x0c1 and execute the command '/path/to/command.sh --command-param argument1 argument2'

Example with no command output send to Cronitor:
  By default, stdout and stderr messages are sent to Cronitor when your job completes. To prevent any output from being sent to cronitor, use the --no-stdout flag:
  $ cronitor exec --no-stdout d3x0c1 /path/to/command.sh --command-param argument1 argument2`,
	Args: func(cmd *cobra.Command, args []string) error {
		// We need to use raw os.Args so we can pass the wrapped command through unparsed
		var foundExec, foundCode bool
		monitorCodeRegex := regexp.MustCompile(`^[A-Za-z0-9]{3,12}$`)

		for _, arg := range os.Args {
			// Treat anything that comes after the monitor code as the command to execute
			if foundCode {
				commandParts = append(commandParts,  strings.TrimSpace(arg))
				continue
			}

			// After finding "exec" we are looking for a monitor code
			if foundExec && !foundCode {
				if ret := monitorCodeRegex.FindStringSubmatch(strings.TrimSpace(arg)); ret != nil {
					monitorCode = arg
					foundCode = true
				}

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
			return errors.New("A unique monitor code and cli command are required e.g. cronitor exec d3x0c1 /path/to/command.sh")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		subcommand := shellquote.Join(commandParts...)
		os.Exit(RunCommand(subcommand, true, true))
	},
}

func RunCommand(subcommand string, withEnvironment bool, withMonitoring bool) int {
	var wg sync.WaitGroup

	startTime := makeStamp()
	formattedStartTime := formatStamp(startTime)

	if withMonitoring {
		wg.Add(1)
		go sendPing("run", monitorCode, subcommand, formattedStartTime, startTime, nil, nil, &wg)
	}

	log(fmt.Sprintf("Running subcommand: %s", subcommand))

	execCmd := makeSubcommandExec(subcommand)
	if withEnvironment {
		execCmd.Env = makeSubcommandEnv(os.Environ())
	} else {
		execCmd.Env = makeSubcommandEnv([]string{})
	}

	// Handle stdin to the subcommand
	execCmdStdin, _ := execCmd.StdinPipe()
	defer execCmdStdin.Close()
	if stdinStat, err := os.Stdin.Stat(); err == nil && stdinStat.Size() > 0 {
		execStdIn, _ := ioutil.ReadAll(os.Stdin)
		execCmdStdin.Write(execStdIn)
	}

	// Combine stdout and stderr from the command into a single buffer which we'll stream as stdout
	// Alternatively we could pass stderr from the subcommand but I've chosen to only use it for CronitorCLI errors at the moment
	var combinedOutput bytes.Buffer
	var maxBufferSize = 2000
	if stdoutPipe, err := execCmd.StdoutPipe(); err == nil {
		streamAndAggregateOutput(&stdoutPipe, &combinedOutput, maxBufferSize)
		execCmd.Stderr = execCmd.Stdout
	}

	// Invoke subcommand and send a message when it's done
	waitCh := make(chan error, 1)
	go func() {
		defer close(waitCh)
		if err := execCmd.Start(); err != nil {
		    waitCh <- err
		} else {
			waitCh <- execCmd.Wait()
		}
	}()

	// Relay incoming signals to the subprocess
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan)

	for {
		select {
		case sig := <-sigChan:
			if err := execCmd.Process.Signal(sig); err != nil {
				// Ignoring because the only time I've seen an err is when child process has already exited after kill was sent to pgroup
			}
		case err := <-waitCh:
			// Handle stdout from subcommand
			outputForStdout := bytes.TrimRight(combinedOutput.Bytes(), "\n")
			outputForPing := outputForStdout
			if noStdoutPassthru {
				outputForPing = []byte{}
			}

			endTime := makeStamp()
			duration := endTime - startTime
			exitCode := 0
			if err == nil {
				if withMonitoring {
					wg.Add(1)
					go sendPing("complete", monitorCode, string(outputForPing), formattedStartTime, endTime, &duration, &exitCode, &wg)
				}
			} else {
				message := strings.TrimSpace(fmt.Sprintf("[%s] %s", err.Error(), outputForPing))

				// This works on both Unix and Windows (syscall.WaitStatus is cross platform).
				// Cribbed from aws-vault.
				if exiterr, ok := err.(*exec.ExitError); ok {
					if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
						exitCode = status.ExitStatus()
					} else {
						exitCode = 1
					}
				}

				if withMonitoring {
					wg.Add(1)
					go sendPing("fail", monitorCode, message, formattedStartTime, endTime, &duration, &exitCode, &wg)
				}
			}

			wg.Wait()
			return exitCode
		}
	}

}

func init() {
	RootCmd.AddCommand(execCmd)
	execCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes")
}

func makeSubcommandEnv(env []string) []string {
	env = append(env, "CRONITOR_EXEC=1")

	if homeValue, hasHome := os.LookupEnv("HOME") ; hasHome {
		env = append(env, "HOME=" + homeValue)
	}

	if pathDirs, hasPath := os.LookupEnv("PATH") ; hasPath {
		env = append(env, "PATH=" + pathDirs)
	}

	return env
}

func makeSubcommandExec(subcommand string) *exec.Cmd {
	var execCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		execCmd = exec.Command("cmd", "/c", subcommand)
	} else {
		execCmd = exec.Command("sh", "-c", subcommand)
	}

	return execCmd
}

func streamAndAggregateOutput(pipe *io.ReadCloser, outputBuffer *bytes.Buffer, maxOutputBufferSize int) {
	scanner := bufio.NewScanner(*pipe)
	go func() {
		for scanner.Scan() {
			fmt.Println(scanner.Text())
			// Ideally we would keep the last n bytes of output but keeping first n bytes easier and acceptable trade off for now..
			if len(scanner.Bytes()) + outputBuffer.Len() <= maxOutputBufferSize {
				outputBuffer.Write(append(scanner.Bytes(), "\n"...))
			}
		}
	}()
}
