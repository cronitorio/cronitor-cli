package cmd

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/kballard/go-shellquote"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
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

func RunCommand(subcommand string, withEnvironment bool, withMonitoring bool) int {
	var monitoringWaitGroup sync.WaitGroup

	startTime := makeStamp()
	series := formatStamp(startTime)

	if withMonitoring {
		monitoringWaitGroup.Add(1)
		go sendPing("run", monitorCode, subcommand, series, startTime, nil, nil, nil, &monitoringWaitGroup)
	}

	log(fmt.Sprintf("Running subcommand: %s", subcommand))

	execCmd := makeSubcommandExec(subcommand)
	if withEnvironment {
		execCmd.Env = os.Environ()
	} else {
		execCmd.Env = makeCronLikeEnv()
	}
	execCmd.Env = append(execCmd.Env, "CRONITOR_EXEC=1")

	// Handle stdin to the subcommand
	execCmdStdin, _ := execCmd.StdinPipe()
	defer execCmdStdin.Close()
	if stdinStat, err := os.Stdin.Stat(); err == nil && stdinStat.Size() > 0 {
		execStdIn, _ := ioutil.ReadAll(os.Stdin)
		execCmdStdin.Write(execStdIn)
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
	go func() {
		defer close(waitCh)

		// Brief pause to allow gochannel selects
		time.Sleep(20 * time.Millisecond)

		if err := execCmd.Start(); err != nil {
			waitCh <- err
		} else {
			waitCh <- execCmd.Wait()
		}
	}()

	// Relay incoming signals to the subprocess
	sigChan := make(chan os.Signal, 16)
	signal.Notify(sigChan)

	for {
		select {
		case sig := <-sigChan:
			if execCmd.Process != nil {
				if err := execCmd.Process.Signal(sig); err != nil {
					// Ignoring because the only time I've seen an err is when child process has already exited after kill was sent to pgroup
				}
			}
		case err := <-waitCh:

			// Send output to Cronitor and clean up after the temp file
			outputForPing := gatherOutput(tempFile, true)
			var metrics map[string]int = nil
			logLengthForPing, err := getFileSize(tempFile)
			if err == nil {
				metrics = map[string]int{
					"length": int(logLengthForPing),
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
					go sendPing("complete", monitorCode, string(outputForPing), series, endTime, &duration, &exitCode, metrics, &monitoringWaitGroup)
					monitoringWaitGroup.Add(1)
					go func(wg *sync.WaitGroup) {
						outputForLogs := gatherOutput(tempFile, false)
						_, err = shipLogData(monitorCode, series, string(outputForLogs))
						if err != nil {
							fmt.Printf("%v", err)
						}
						wg.Done()
					}(&monitoringWaitGroup)
				}
			} else {
				message := strings.TrimSpace(fmt.Sprintf("[%s] %s", err.Error(), outputForPing))

				// This works on both Posix and Windows (syscall.WaitStatus is cross platform).
				// Cribbed from aws-vault.
				if exiterr, ok := err.(*exec.ExitError); ok {

					if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
						exitCode = status.ExitStatus()
					} else {
						exitCode = 1
					}
				}

				if withMonitoring {
					monitoringWaitGroup.Add(1)
					go sendPing("fail", monitorCode, message, series, endTime, &duration, &exitCode, metrics, &monitoringWaitGroup)
					monitoringWaitGroup.Add(1)
					go func(wg *sync.WaitGroup) {
						outputForLogs := gatherOutput(tempFile, false)
						_, err = shipLogData(monitorCode, series, string(outputForLogs))
						wg.Done()
					}(&monitoringWaitGroup)
				}
			}

			monitoringWaitGroup.Wait()
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

func makeSubcommandExec(subcommand string) *exec.Cmd {
	var execCmd *exec.Cmd
	if runtime.GOOS == "windows" {
		execCmd = exec.Command("powershell.exe", "-Command", subcommand)
	} else if _, err := os.Stat("/bin/bash"); err == nil {
		execCmd = exec.Command("bash", "-c", subcommand)
	} else {
		execCmd = exec.Command("sh", "-c", subcommand)
	}

	return execCmd
}

func getTempFile() (*os.File, error) {
	// Before we create a new temp file be cautious and ensure we don't have stale files that should be cleaned up
	// This could happen if `exec` crashed in a previous run.
	var cleanupError error
	path := fmt.Sprintf("%s%s%s", os.TempDir(), string(os.PathSeparator), "cronitor")
	os.MkdirAll(path, os.ModePerm)

	if tempFiles, cleanupError := ioutil.ReadDir(path); cleanupError == nil {
		for _, file := range tempFiles {
			if isStaleFile(file) {
				cleanupError = os.Remove(fmt.Sprintf("%s%s%s", path, string(os.PathSeparator), file.Name()))
			}
		}
	}

	// If we can't clean up then stop writing new files...
	if cleanupError != nil {
		return nil, errors.New(fmt.Sprintf("Cannot capture output to temp file, cleanup failed: %s", cleanupError.Error()))
	}

	if file, err := ioutil.TempFile(path, fmt.Sprintf("exec-%s-*", monitorCode)); err == nil {
		return file, nil
	} else {
		return nil, errors.New(fmt.Sprintf("Cannot capture output to temp file: %s", err.Error()))
	}
}

func getFileSize(tempFile *os.File) (int64, error) {
	// Known reasons stat could fail here:
	// 1. temp file was removed by an external process
	// 2. filesystem is no longer available

	stat, err := os.Stat(tempFile.Name())
	return stat.Size(), err
}

func gatherOutput(tempFile *os.File, truncateForPingOutput bool) []byte {
	var outputBytes []byte
	const outputForPingMaxLen int64 = 2000
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

func gzipLogData(logData string) *bytes.Buffer {
	var b bytes.Buffer
	if len(logData) < 1 {
		return &b
	}

	gz := gzip.NewWriter(&b)
	if _, err := gz.Write([]byte(logData)); err != nil {
		log("error writing gzip")
		return nil
	}
	if err := gz.Close(); err != nil {
		log("error closing gzip")
		return nil
	}
	return &b
}

func getPresignedUrl(postBody []byte) ([]byte, error) {
	url := "https://cronitor.io/api/logs/presign"

	client := &http.Client{Timeout: 120 * time.Second}
	request, err := http.NewRequest("POST", url, strings.NewReader(string(postBody)))
	if err != nil {
		return nil, errors.Wrap(err, "could not create request for URL presign")
	}
	request.SetBasicAuth(apiKey, "")
	request.Header.Add("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "error requesting presigned url")
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return nil, fmt.Errorf("error response code %d returned", response.StatusCode)
	}

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	response.Body = ioutil.NopCloser(bytes.NewBuffer(contents))
	return contents, nil
}

func shipLogData(jobKey string, seriesID string, outputLogs string) ([]byte, error) {
	gzippedLogs := gzipLogData(outputLogs)
	jsonBytes, err := json.Marshal(map[string]string{
		"job_key": jobKey,
		"series":  seriesID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "couldn't encode job and series IDs to JSON")
	}
	var responseJson struct {
		Url string `json:"url"`
	}
	response, err := getPresignedUrl(jsonBytes)
	if err != nil {
		return nil, errors.Wrap(err, "error generating presign url for log uploading")
	}
	if err := json.Unmarshal(response, &responseJson); err != nil {
		return nil, err
	}
	s3LogPutUrl := responseJson.Url
	if len(s3LogPutUrl) == 0 {
		return nil, errors.New("no presigned S3 url returned. Something is wrong")
	}
	req, err := http.NewRequest("PUT", s3LogPutUrl, gzippedLogs)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	response2, err := client.Do(req)
	if err != nil || response == nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error putting logs: %v", response2))
	}
	if response2.StatusCode < 200 || response2.StatusCode >= 300 {
		return nil, fmt.Errorf("error response code %d returned", response2.StatusCode)
	}
	body, err := ioutil.ReadAll(response2.Body)
	if err != nil {
		return nil, err
	}
	defer response2.Body.Close()
	log(fmt.Sprintf("logs shipped for series %s", seriesID))
	return body, nil
}
