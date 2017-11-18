package cmd

import (
	"runtime"
	"fmt"
	"encoding/json"
	"io/ioutil"
	"strings"
	"github.com/spf13/cobra"
	"errors"
	"net/http"
	"crypto/sha1"
	"os"
	"github.com/spf13/viper"
	"regexp"
	"math/rand"
	"time"
	"path/filepath"
	"bytes"
)

type Rule struct {
	RuleType     string `json:"rule_type"`
	Value        string `json:"value"`
	TimeUnit     string `json:"time_unit,omitempty"`
	GraceSeconds uint   `json:"grace_seconds,omitempty"`
}

type Monitor struct {
	Name  		string   `json:"defaultName"`
	Key   		string   `json:"key"`
	Rules 		[]Rule   `json:"rules"`
	Tags  		[]string `json:"tags"`
	Type  		string   `json:"type"`
	Code  		string   `json:"code,omitempty"`
	Timezone	string   `json:"timezone,omitempty"`
	Note  		string   `json:"note,omitempty"`
}

type Line struct {
	Name           string
	FullLine       string
	LineNumber     int
	CronExpression string
	CommandToRun   string
	Code           string
	Mon            Monitor
}

func (l Line) IsMonitorable() bool {
	containsLegacyIntegration := strings.Contains(l.CommandToRun, "cronitor.io") || strings.Contains(l.CommandToRun, "cronitor.link")
	isRebootJob := l.CronExpression == "@reboot"
	return len(l.CronExpression) > 0 && len(l.CommandToRun) > 0 && !containsLegacyIntegration && !isRebootJob
}

func (l Line) IsAutoDiscoverCommand() bool {
	matched, _ := regexp.MatchString(".+cronitor[[:space:]]+discover.+", strings.ToLower(l.CommandToRun))
	return matched
}

var excludeFromName []string
var noAutoDiscover bool
var saveCrontabFile bool
var crontabPath string

var discoverCmd = &cobra.Command{
	Use:   "discover [crontab]",
	Short: "Identify new cron jobs and attach Cronitor monitoring. When no crontab argument is provided /etc/crontab is used.",
	Long:  `
Cronitor discover will parse your crontab file and create or update monitors using the Cronitor API.

Note: You must supply your Cronitor API key. This can be passed as a flag, environment variable, or saved in your Cronitor configuration file. See 'help configure' for more details.

Example:

  $ cronitor discover
    ... Create monitors on your Cronitor dashboard for each entry in /etc/crontab. The command string will be used as a name.
    ... Add Cronitor integration to your crontab and output to stdout

Example with customized monitor names:

  $ cronitor discover -e "/var/app/code/path/" -e "/var/app/bin/" -e "> /dev/null"
    ... Update previously discovered monitors or create new monitors, excluding the provided snippets from the monitor name.
    ... Add Cronitor integration to your crontab and output to stdout

  You can run the command as many times as you need with additional exclusion text until the job names on your Cronitor Dashboard are clear and readable.

Example with an arbitrary crontab file:

  $ cronitor discover /path/to/crontab
    ... Create monitors on your Cronitor dashboard for each entry in /path/to/crontab.
    ... Add Cronitor integration to your crontab and output to stdout

  When a monitor is created its crontab file location is added as a default note.

Example where your Crontab file is updated in place:

  $ cronitor discover --save
    ... Create or update monitors
    ... Add Cronitor integration to your crontab and save the file


In all of these examples, auto discover is enabled by adding 'cronitor discover' to your crontab as an hourly task. If a --save param is provided, auto discover
will ensure that any entries added to your crontab are automatically integrated with Cronitor. Even without --save, auto discover will push schedule changes
to Cronitor to keep your monitoring in sync with your Crontab.
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString("CRONITOR-API-KEY")) < 10 {
			return errors.New("you must provide an API key with this command or save a key using 'cronitor configure'")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 && runtime.GOOS == "windows" {
			fatal("A crontab file argument is required on this platform", 1)
		}

		crontabPath = "/etc/crontab"
		if len(args) > 0 {
			crontabPath = args[0]
		}

		crontabStrings, errCode, err := readCrontab(crontabPath)
		if err != nil {
			fatal(err.Error(), errCode)
		}

		crontabLines := parseCrontab(crontabStrings)
		timezone := effectiveTimezoneLocationName()

		// Read crontabLines into map of Monitor structs
		monitors := map[string]*Monitor{}
		for _, line := range crontabLines {
			if !line.IsMonitorable() {
				continue
			}

			rules := []Rule{createRule(line.CronExpression)}
			name := createName(line.CommandToRun, line.IsAutoDiscoverCommand())
			key := createKey(line.CommandToRun, line.CronExpression, line.IsAutoDiscoverCommand())
			tags := createTags()

			line.Mon = Monitor{
				name,
				key,
				rules,
				tags,
				"heartbeat",
				line.Code,
				timezone,
				createNote(line.LineNumber, line.IsAutoDiscoverCommand()),
			}

			monitors[key] = &line.Mon
		}

		// Put monitors to Cronitor API
		monitors, err = putMonitors(monitors)
		if err != nil {
			fatal(err.Error(), 1)
		}

		// Re-write crontab lines with new/updated monitoring
		var crontabOutput []string
		for _, line := range crontabLines {
			crontabOutput = append(crontabOutput, createCrontabLine(line))
		}

		updatedCrontabLines := strings.Join(crontabOutput, "\n")

		if saveCrontabFile {
			if ioutil.WriteFile(crontabPath, []byte(updatedCrontabLines), 0644) != nil {
				fatal(fmt.Sprintf("The --save option is supplied but the file at %s could not be written; check permissions and try again", crontabPath), 126)
			}

			fmt.Println(fmt.Sprintf("Crontab %s updated", crontabPath))
		} else {
			fmt.Println(updatedCrontabLines)
		}
	},
}

func readCrontab(crontabPath string) ([]string, int, error) {
	if _, err := os.Stat(crontabPath); os.IsNotExist(err) {
		return nil, 66, errors.New(fmt.Sprintf("the file %s does not exist", crontabPath))
	}

	crontabBytes, err := ioutil.ReadFile(crontabPath)
	if err != nil {
		return nil, 126, errors.New(fmt.Sprintf("the crontab file at %s could not be read; check permissions and try again", crontabPath))
	}

	// When the save flag is passed, attempt to write the file back to itself to ensure we have proper permissions before going further
	if saveCrontabFile {
		if ioutil.WriteFile(crontabPath, crontabBytes, 0644) != nil {
			return nil, 126, errors.New(fmt.Sprintf("the --save option is supplied but the file at %s could not be written; check permissions and try again", crontabPath))
		}
	}

	return strings.Split(string(crontabBytes), "\n"), 0, nil
}

func putMonitors(monitors map[string]*Monitor) (map[string]*Monitor, error) {
	url := effectiveApiUrl()
	monitorsArray := make([]Monitor, 0, len(monitors))
	for _, v := range monitors {
		monitorsArray = append(monitorsArray, *v)
	}

	jsonBytes, _ := json.Marshal(monitorsArray)
	jsonString := string(jsonBytes)

	buf := new(bytes.Buffer)
	json.Indent(buf, jsonBytes, "", "  ")
	log("\nRequest:")
	log(buf.String() + "\n")

	response, err := sendHttpPut(url, jsonString)
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Request to %s failed: %s", url, err))
	}

	json.Indent(buf, response, "", "  ")
	log("\nResponse:")
	log(buf.String() + "\n")

	responseMonitors := []Monitor{}
	if err = json.Unmarshal(response, &responseMonitors); err != nil {
		return nil, errors.New(fmt.Sprintf("Error from %s: %s", url, response))
	}

	for _, value := range responseMonitors {
		// We only need to update the Monitor struct with a code if this is a new monitor.
		// For updates the monitor code is sent as well as the key and that takes precedence.
		if _, ok := monitors[value.Key]; ok {
			monitors[value.Key].Code = value.Code
		}

	}

	return monitors, nil
}

func createCrontabLine(line *Line) string {
	if !line.IsMonitorable() || len(line.Code) > 0 {
		// If a cronitor integration already existed on the line we have nothing else here to change
		return line.FullLine
	}

	var lineParts []string
	lineParts = append(lineParts, line.CronExpression)

	if len(line.Mon.Code) > 0 {
		lineParts = append(lineParts, "cronitor")
		if noStdoutPassthru {
			lineParts = append(lineParts, "--no-stdout")
		}
		lineParts = append(lineParts, "exec")
		lineParts = append(lineParts, line.Mon.Code)
	}

	if len(line.CommandToRun) > 0 {
		lineParts = append(lineParts, line.CommandToRun)
	}

	return strings.Join(lineParts, " ")
}

func parseCrontab(lines []string) []*Line {
	var crontabLines []*Line
	var autoDiscoverLine *Line
	var usesSixFieldCronExpression bool

	for lineNumber, fullLine := range lines {
		var cronExpression string
		var command []string

		fullLine = strings.TrimSpace(fullLine)

		// Do not attempt to parse the current line if it's a comment
		// Otherwise split on any whitespace and parse
		if !strings.HasPrefix(fullLine, "#") {
			splitLine := strings.Fields(fullLine)
			if len(splitLine) > 0 && strings.HasPrefix(splitLine[0], "@") {
				cronExpression = splitLine[0]
				command = splitLine[1:]
			} else if len(splitLine) >= 6 {
				// Handle javacron-style 6 item cron expressions
				// If there are at least 7 items, and the 6th looks like a cron expression, assume it is one
				match, _ := regexp.MatchString("^[-,?*/0-9]+$", splitLine[5])
				usesSixFieldCronExpression = match && len(splitLine) >= 7
				if usesSixFieldCronExpression {
					cronExpression = strings.Join(splitLine[0:6], " ")
					command = splitLine[6:]
				} else {
					cronExpression = strings.Join(splitLine[0:5], " ")
					command = splitLine[5:]
				}
			}
		}

		// Create a Line struct with details for this line so we can re-create it later
		line := Line{}
		line.CronExpression = cronExpression
		line.FullLine = fullLine
		line.LineNumber = lineNumber

		// If this job is already being wrapped by the Cronitor client, read current code.
		// Expects a wrapped command to look like: cronitor exec d3x0 /path/to/cmd.sh
		if len(command) > 1 && command[0] == "cronitor" && command[1] == "exec" {
			line.Code = command[2]
			command = command[3:]
		}

		line.CommandToRun = strings.Join(command, " ")

		if line.IsAutoDiscoverCommand() {
			autoDiscoverLine = &line
			if noAutoDiscover {
				continue // remove the auto-discover line from the crontab
			}
		}

		crontabLines = append(crontabLines, &line)
	}

	// If we do not have an auto-discover line but we should, add one now
	if autoDiscoverLine == nil && !noAutoDiscover {
		crontabLines = append(crontabLines, createAutoDiscoverLine(usesSixFieldCronExpression))
	}

	return crontabLines
}

func createAutoDiscoverLine(usesSixFieldCronExpression bool) *Line {
	cronExpression := fmt.Sprintf("%d * * * *", randomMinute())
	if usesSixFieldCronExpression {
		cronExpression = fmt.Sprintf("* %s", cronExpression)
	}

	// Normalize the command so it can be run reliably from crontab
	commandToRun := strings.Join(os.Args, " ")
	commandToRun = strings.Replace(commandToRun, "--save ", "", -1)
	commandToRun = strings.Replace(commandToRun, "--verbose ", "", -1)
	commandToRun = strings.Replace(commandToRun, "-v ", "", -1)
	commandToRun = strings.Replace(commandToRun, crontabPath, getCrontabPath(), -1)

	line := Line{}
	line.CronExpression = cronExpression
	line.CommandToRun = commandToRun
	return &line
}

func createNote(LineNumber int, IsAutoDiscoverCommand bool) string {
	if IsAutoDiscoverCommand {
		return fmt.Sprintf("Watching for schedule changes and new entries in %s", crontabPath)
	}

	return fmt.Sprintf("Discovered in %s:%d", getCrontabPath(), LineNumber)
}

func createName(CommandToRun string, IsAutoDiscoverCommand bool) string {
	excludeFromName = append(excludeFromName, "> /dev/null")
	excludeFromName = append(excludeFromName, "2>&1")
	excludeFromName = append(excludeFromName, "/bin/bash -l -c")
	excludeFromName = append(excludeFromName, "/bin/bash -lc")
	excludeFromName = append(excludeFromName, "/bin/bash -c -l")
	excludeFromName = append(excludeFromName, "/bin/bash -cl")

	if IsAutoDiscoverCommand {
		return truncateString(fmt.Sprintf("[%s] Auto discover %s", effectiveHostname(), strings.TrimSpace(crontabPath)), 100)
	}

	for _, substr := range excludeFromName {
		CommandToRun = strings.Replace(CommandToRun, substr, "", -1)
	}

	CommandToRun = strings.TrimSpace(truncateString(CommandToRun, 100))
	CommandToRun = strings.Trim(CommandToRun, ">'\"")
	return truncateString(fmt.Sprintf("[%s] %s", effectiveHostname(), strings.TrimSpace(CommandToRun)), 100)
}

func createKey(CommandToRun string, CronExpression string, IsAutoDiscoverCommand bool) string {
	if IsAutoDiscoverCommand {
		// Go out of our way to prevent making a duplicate monitor for an auto-discovery command.
		// Ensure that tinkering with params does not change key
		normalizedCommand := []string{}
		for _, field := range strings.Fields(CommandToRun) {
			if !strings.HasPrefix(CommandToRun, "-") {
				normalizedCommand = append(normalizedCommand, field)
			}
		}

		CommandToRun = strings.Join(normalizedCommand, " ")
		CronExpression = "" // The schedule is randomized for auto discover, so just ignore it
	}

	data := []byte(fmt.Sprintf("%s-%s-%s", effectiveHostname(), CommandToRun, CronExpression))
	return fmt.Sprintf("%x", sha1.Sum(data))
}

func createTags() []string {
	var tags []string
	tags = append(tags, "cron-job")
	return tags
}

func createRule(cronExpression string) Rule {
	var rule Rule
	if strings.HasPrefix(cronExpression, "@yearly") {
		rule = Rule{"complete_ping_not_received", "365", "days", 86400}
	} else if strings.HasPrefix(cronExpression, "@monthly") {
		rule = Rule{"complete_ping_not_received", "31", "days", 86400}
	} else if strings.HasPrefix(cronExpression, "@weekly") {
		rule = Rule{"complete_ping_not_received", "7", "days", 86400}
	} else if strings.HasPrefix(cronExpression, "@daily") {
		rule = Rule{"complete_ping_not_received", "24", "hours", 3600}
	} else if strings.HasPrefix(cronExpression, "@hourly") {
		rule = Rule{"complete_ping_not_received", "1", "hours", 600}
	} else {
		rule = Rule{"not_on_schedule", cronExpression, "", 0}
	}

	return rule
}

func sendHttpPut(url string, body string) ([]byte, error) {
	client := &http.Client{}
	request, err := http.NewRequest("PUT", url, strings.NewReader(body))
	request.SetBasicAuth(viper.GetString("CRONITOR-API-KEY"), "")
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", userAgent)
	request.ContentLength = int64(len(body))
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return contents, nil
}

func randomMinute() int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(59)
}

func getCrontabPath() string {
	if absoluteCronPath, err := filepath.Abs(crontabPath); err == nil {
		return absoluteCronPath
	}

	return crontabPath
}

func init() {
	RootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().BoolVar(&saveCrontabFile, "save", saveCrontabFile, "Save the updated crontab file")
	discoverCmd.Flags().StringArrayVarP(&excludeFromName, "exclude-from-name", "e", excludeFromName, "Substring to exclude from generated monitor name e.g. $ cronitor discover -e '> /dev/null' -e '/path/to/app'")
	discoverCmd.Flags().BoolVar(&noAutoDiscover, "no-auto-discover", noAutoDiscover, "Do not attach an automatic discover job to this crontab, or remove if already attached.")
}
