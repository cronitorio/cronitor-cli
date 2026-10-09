package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var monitorCode string
var commandParts []string
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with monitoring",
	Long: `
The supplied command will be executed and Cronitor will be notified of success or failure.

Note: Arguments supplied after the unique monitor key are treated as part of the command to execute. Flags intended for the 'exec' command must be passed before the monitor key.

Example:
  $ cronitor exec d3x0c1 /path/to/command.sh --command-param argument1 argument2
  This command will ping your Cronitor monitor d3x0c1 and execute the command '/path/to/command.sh --command-param argument1 argument2'

Example with no command output send to Cronitor:
  By default, stdout and stderr messages are sent to Cronitor when your job completes. To prevent any output from being sent to cronitor, use the --no-stdout flag:
  $ cronitor exec --no-stdout d3x0c1 /path/to/command.sh --command-param argument1 argument2`,
	Args: func(cmd *cobra.Command, args []string) error {
		// We need to use raw os.Args so we can pass the wrapped command through unparsed
		var foundExec, foundCode bool
		monitorCodeRegex := regexp.MustCompile(`^[\S]{1,128}$`)

		// We need to know all of the flags so we can properly identify the monitor code.
		allFlags := map[string]bool{
			"--": true, // seed with the argument separator
		}
		cmd.Flags().VisitAll(func(flag *flag.Flag) {
			allFlags["--"+flag.Name] = true
			allFlags["-"+flag.Shorthand] = true
		})

		for _, arg := range os.Args {
			arg = strings.TrimSpace(arg)
			// Treat anything that comes after the monitor code as the command to execute
			if foundCode {
				commandParts = append(commandParts, strings.TrimSpace(arg))
				continue
			}

			// After finding "exec" we are looking for a monitor code
			if foundExec && !foundCode {
				if _, is_flag := allFlags[arg]; is_flag {
					continue
				}

				if ret := monitorCodeRegex.FindStringSubmatch(arg); ret == nil {
					continue
				}

				monitorCode = arg
				foundCode = true
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
			return errors.New("A unique monitor key and cli command are required e.g. cronitor exec d3x0c1 /path/to/command.sh")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var subcommand string
		if len(commandParts) == 1 {
			subcommand = commandParts[0]
		} else {
			subcommand = shellquote.Join(commandParts...)
		}
		os.Exit(RunCommand(subcommand, true, true))
	},
}

// RunCommand runs subcommand. explicitShell, when set, is used as-is for
// `shell -c`, including /bin/sh. Cron puts the wrapper in SHELL, so that
// variable is not consulted on the shim path.
func RunCommand(subcommand string, withEnvironment bool, withMonitoring bool, explicitShell ...string) int {
	shell := ""
	if len(explicitShell) > 0 {
		shell = explicitShell[0]
	}
	var monitoringWaitGroup sync.WaitGroup

	startTime := makeStamp()
	series := formatStamp(startTime)
	schedule := ""

	if withMonitoring {
		monitoringWaitGroup.Add(1)
		if runtime.GOOS == "windows" {
			schedule = GetNextRunFromMonitorKey(monitorCode)
		}
		go sendPing("run", monitorCode, subcommand, series, startTime, nil, nil, nil, schedule, &monitoringWaitGroup)
	}

	log(fmt.Sprintf("Running subcommand: %s", subcommand))

	env := os.Environ()
	if !withEnvironment {
		env = makeCronLikeEnv()
	}
	env = append(env, "CRONITOR_EXEC=1")
	env = dropEnv(env, "CRONITOR_SHIM_FD")
	env = dropEnv(env, "CRONITOR_SHIM_SAVED_FD")
	if shell != "" {
		env = setEnv(env, "SHELL", shell)
	}
	execCmd := makeSubcommandExecWithShell(subcommand, shell, env)
	execCmd.Env = env

	// Handle stdin to the subcommand - improved pipe handling
	execCmdStdin, err := execCmd.StdinPipe()
	if err != nil {
		log(fmt.Sprintf("Failed to create stdin pipe: %v", err))
	} else {
		defer execCmdStdin.Close()
		go func() {
			defer func() { recover() }()
			defer execCmdStdin.Close()
			io.Copy(execCmdStdin, os.Stdin)
		}()
	}

	// Proxy and copy the command's stdout if the filesystem is available
	tempFile, err := getTempFile()
	if err == nil {
		defer tempFile.Close()
		execCmd.Stdout = io.MultiWriter(os.Stdout, tempFile)
	} else {
		log(err.Error())
		execCmd.Stdout = os.Stdout
	}

	// Combine stdout and stderr from the command into a single buffer which we'll stream as stdout
	// Alternatively we could pass stderr from the subcommand but I've chosen to only use it for CronitorCLI errors at the moment
	execCmd.Stderr = execCmd.Stdout

	// Invoke subcommand and send a message when it's done
	waitCh := make(chan error, 16)
	var jobs jobSignal
	go func() {
		defer close(waitCh)

		// The byte is the promise that this process runs the job once.
		// It is sent before Start so a crash after Start cannot look like
		// "never started" and make the wrapper run the command again.
		if shell != "" {
			commitShimHandshake()
			if shimAfterHandshake != nil {
				shimAfterHandshake()
			}
		}
		time.Sleep(20 * time.Millisecond)

		if err := execCmd.Start(); err != nil {
			if shell != "" {
				execRealShell(shell, subcommand)
			}
			waitCh <- err
			return
		}
		jobs.started(execCmd.Process)
		waitCh <- execCmd.Wait()
	}()

	// Improved signal handling
	sigChan := make(chan os.Signal, 16)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	for {
		select {
		case sig := <-sigChan:
			jobs.deliver(sig)

		case err := <-waitCh:
			// Stop listening for signals since process has exited
			signal.Stop(sigChan)
			close(sigChan)

			// Send output to Cronitor and clean up after the temp file
			outputForPing := gatherOutput(tempFile, true)
			var metrics map[string]int = nil
			if tempFile != nil {
				logLengthForPing, err2 := getFileSize(tempFile)
				if err2 == nil {
					metrics = map[string]int{
						"length": int(logLengthForPing),
					}
				}
			}

			defer func() {
				if tempFile != nil {
					tempFile.Close()
					os.Remove(tempFile.Name())
				}
			}()

			endTime := makeStamp()
			duration := endTime - startTime
			exitCode := 0

			if err == nil {
				if withMonitoring {
					monitoringWaitGroup.Add(1)
					go sendPing("complete", monitorCode, string(outputForPing), series, endTime, &duration, &exitCode, metrics, schedule, &monitoringWaitGroup)
					if tempFile != nil {
						monitoringWaitGroup.Add(1)
						go shipLogData(tempFile, series, &monitoringWaitGroup)
					}
				}
			} else {
				message := ""
				if !noStdoutPassthru {
					message = strings.TrimSpace(fmt.Sprintf("%s [%s]", outputForPing, err.Error()))
				}

				// This works on both Posix and Windows (syscall.WaitStatus is cross platform).
				// Cribbed from aws-vault.
				if exiterr, ok := err.(*exec.ExitError); ok {
					if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
						exitCode = exitStatusOf(status)
					} else {
						exitCode = 1
					}
				}

				if withMonitoring {
					monitoringWaitGroup.Add(1)
					go sendPing("fail", monitorCode, message, series, endTime, &duration, &exitCode, metrics, schedule, &monitoringWaitGroup)
					if tempFile != nil {
						monitoringWaitGroup.Add(1)
						go shipLogData(tempFile, series, &monitoringWaitGroup)
					}
				}
			}

			if shell != "" {
				// Shim path only. exec keeps its own retry budget.
				waitForTelemetry(&monitoringWaitGroup, 3*time.Second)
			} else {
				monitoringWaitGroup.Wait()
			}
			return exitCode
		}
	}

}

func init() {
	RootCmd.AddCommand(execCmd)
	execCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes")
}

func makeCronLikeEnv() []string {
	env := []string{"SHELL=/bin/sh"}
	if homeValue, hasHome := os.LookupEnv("HOME"); hasHome {
		env = append(env, "HOME="+homeValue)
	}

	return env
}

// makeSubcommandExec starts the monitored command. SHELL comes from env, the
// environment the child receives. An absolute executable other than /bin/sh is
// used with -c; otherwise bash -c if /bin/bash exists, else sh -c. Windows
// uses powershell.exe -Command.
func makeSubcommandExec(subcommand string, env []string) *exec.Cmd {
	name, flag := interpreterForExec(runtime.GOOS, shellFromEnv(env), os.Stat)
	if runtime.GOOS == "windows" {
		return exec.Command(name, flag, subcommand)
	}

	execCmd := exec.Command(name, flag, subcommand)
	execCmd.SysProcAttr = getPlatformSysProcAttr()
	return execCmd
}

// makeSubcommandExecWithShell uses shell -c as given. An empty shell keeps
// makeSubcommandExec, which reads SHELL from the child environment.
func makeSubcommandExecWithShell(subcommand, explicitShell string, env []string) *exec.Cmd {
	if explicitShell == "" {
		return makeSubcommandExec(subcommand, env)
	}
	cmd := exec.Command(explicitShell, "-c", subcommand)
	cmd.SysProcAttr = getPlatformSysProcAttr()
	return cmd
}

// interpreterForExec selects the program and its command flag.
// stat is os.Stat in production and a fake in tests.
func interpreterForExec(goos, shellEnv string, stat func(string) (os.FileInfo, error)) (name, flag string) {
	if goos == "windows" {
		return "powershell.exe", "-Command"
	}
	if shell, ok := selectableShell(shellEnv, stat); ok {
		return shell, "-c"
	}
	if _, err := stat("/bin/bash"); err == nil {
		return "bash", "-c"
	}
	return "sh", "-c"
}

// selectableShell reports the cleaned SHELL path when it is safe to use.
// /bin/sh is not selectable; see makeSubcommandExec.
func selectableShell(shellEnv string, stat func(string) (os.FileInfo, error)) (string, bool) {
	cleaned := cleanShellPath(shellEnv)
	if !strings.HasPrefix(cleaned, "/") || cleaned == "/bin/sh" {
		return "", false
	}
	info, err := stat(cleaned)
	if err != nil || info.IsDir() {
		return "", false
	}
	if info.Mode().Perm()&0111 == 0 {
		return "", false
	}
	return cleaned, true
}

// shellFromEnv returns the last SHELL= value in env.
func shellFromEnv(env []string) string {
	shell := ""
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok && key == "SHELL" {
			shell = value
		}
	}
	return shell
}

// cleanShellPath trims space and lexically cleans the path so /bin//sh and
// /bin/../bin/sh compare equal to /bin/sh.
func cleanShellPath(shellEnv string) string {
	shellEnv = strings.TrimSpace(shellEnv)
	if shellEnv == "" {
		return ""
	}
	return path.Clean(shellEnv)
}

func getTempFile() (*os.File, error) {
	path := filepath.Join(os.TempDir(), "cronitor")

	// Create directory with restricted permissions
	if err := os.MkdirAll(path, 0777); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Use more secure temp file creation
	file, err := ioutil.TempFile(path, fmt.Sprintf("exec-%s-*.log", monitorCode))
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	// Set restrictive permissions
	if err := file.Chmod(0600); err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, fmt.Errorf("failed to set file permissions: %w", err)
	}

	return file, nil
}

func getFileSize(tempFile *os.File) (int64, error) {
	// Known reasons stat could fail here:
	// 1. temp file was removed by an external process
	// 2. filesystem is no longer available

	stat, err := os.Stat(tempFile.Name())
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func gatherOutput(tempFile *os.File, truncateForPingOutput bool) []byte {
	var outputBytes []byte
	const outputForPingMaxLen int64 = 1000
	const outputForLogUploadMaxLen int64 = 100000000
	if noStdoutPassthru || tempFile == nil {
		outputBytes = []byte{}
	} else {

		if size, err := getFileSize(tempFile); err == nil {
			// In all cases, if we have to truncate, we want to read the END
			// of the log file, because it is more informative.
			if truncateForPingOutput && size > outputForPingMaxLen {
				outputBytes = make([]byte, outputForPingMaxLen)
				tempFile.Seek(outputForPingMaxLen*-1, 2)
			} else if !truncateForPingOutput && size > outputForLogUploadMaxLen {
				outputBytes = make([]byte, outputForLogUploadMaxLen)
				tempFile.Seek(outputForLogUploadMaxLen*-1, 2)
			} else {
				outputBytes = make([]byte, size)
				tempFile.Seek(0, 0)
			}
			tempFile.Read(outputBytes)
		}
	}

	return outputBytes
}

func isStaleFile(file os.FileInfo) bool {
	var timeLimit = 3 * 24 * time.Hour

	if !file.Mode().IsRegular() {
		return false
	}

	return time.Now().Sub(file.ModTime()) > timeLimit
}

func shipLogData(tempFile *os.File, series string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if rec := recover(); rec != nil {
			log(fmt.Sprintf("log upload recovered: %v", rec))
		}
	}()
	outputForLogs := gatherOutput(tempFile, false)
	_, err := lib.SendLogData(viper.GetString(varApiKey), monitorCode, series, string(outputForLogs))
	if err != nil {
		log(fmt.Sprintf("%v", err))
	}
}

func waitForTelemetry(wg *sync.WaitGroup, bound time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(bound):
	}
}

// jobSignal holds a signal that arrives before Start and delivers it once
// the job exists, including to its process group.
type jobSignal struct {
	mu   sync.Mutex
	sig  os.Signal
	proc *os.Process
}

func (j *jobSignal) deliver(sig os.Signal) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.proc == nil {
		j.sig = sig
		return
	}
	signalJob(j.proc, sig)
}

func (j *jobSignal) started(proc *os.Process) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.proc = proc
	if j.sig != nil {
		signalJob(proc, j.sig)
		j.sig = nil
	}
}

func setEnv(env []string, key, value string) []string {
	return append(dropEnv(env, key), key+"="+value)
}

func dropEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}
