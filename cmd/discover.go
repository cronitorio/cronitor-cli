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
	"os/exec"
	"strconv"
	"github.com/manifoldco/promptui"
	"github.com/getsentry/raven-go"
	"net/url"
	"os/user"
)

type Rule struct {
	RuleType     string `json:"rule_type"`
	Value        string `json:"value"`
	TimeUnit     string `json:"time_unit,omitempty"`
	GraceSeconds uint   `json:"grace_seconds,omitempty"`
}

type Monitor struct {
	Name  			string   `json:"name,omitempty"`
	DefaultName		string   `json:"defaultName"`
	Key   			string   `json:"key"`
	Rules 			[]Rule   `json:"rules"`
	Tags  			[]string `json:"tags"`
	Type  			string   `json:"type"`
	Code			string   `json:"code,omitempty"`
	Timezone		string	 `json:"timezone,omitempty"`
	Note  			string   `json:"defaultNote,omitempty"`
	Notifications	map[string][]string `json:"notifications,omitempty"`
}

type Line struct {
	Name           string
	FullLine       string
	LineNumber     int
	CronExpression string
	CommandToRun   string
	Code           string
	RunAs          string
	Mon            Monitor
}


func (l Line) IsMonitorable() bool {
	containsLegacyIntegration := strings.Contains(l.CommandToRun, "cronitor.io") || strings.Contains(l.CommandToRun, "cronitor.link")
	return len(l.CronExpression) > 0 && len(l.CommandToRun) > 0 && !containsLegacyIntegration
}

func (l Line) IsAutoDiscoverCommand() bool {
	matched, _ := regexp.MatchString(".+discover[[:space:]]+--auto.*", strings.ToLower(l.CommandToRun))
	return matched
}

var excludeFromName []string
var interactiveMode = true
var isAutoDiscover bool
var isSilent bool
var noAutoDiscover bool
var saveCrontabFile bool
var crontabPath string
var isUserCrontab bool
var timezone TimezoneLocationName
var maxNameLen = 75
var notificationList string

var discoverCmd = &cobra.Command{
	Use:   "discover <optional crontab>",
	Short: "Attach monitoring to new cron jobs and watch for schedule updates",
	Long:  `
Cronitor discover will parse your crontab and create or update monitors using the Cronitor API.

Note: You must supply your Cronitor API key. This can be passed as a flag, environment variable, or saved in your Cronitor configuration file. See 'help configure' for more details.

Example:
  $ cronitor discover
      > Read user crontab and step through line by line
      > Creates monitors on your Cronitor dashboard for each entry in the crontab. The command string will be used as the monitor name.
      > Makes no changes to your crontab, add a --save param or use "cronitor discover --auto --save" later when you are ready to apply changes.

  $ cronitor discover /path/to/crontab
      > Instead of the user crontab, provide a crontab file to use

Example that does not use an interactive shell:
  $ cronitor discover --auto"
      > The only output to stdout will be your crontab file with monitoring, suitable for piplines or writing to another crontab.

Example using exclusion text to remove secrets or boilerplate:
  $ cronitor discover /path/to/crontab -e "secret-token" -e "/var/common/app/path/"
      > Updates previously discovered monitors or creates new monitors, excluding the provided snippets from the monitor name.
      > Adds Cronitor integration to your crontab and outputs to stdout
      > Names you create yourself in "discover" or from the dashboard are unchanged.

  You can run the command as many times as you need, accumulating exclusion params until the job names on your Cronitor dashboard are clear and readable.

Example where your crontab is updated in place:
  $ cronitor discover /path/to/crontab --save
      > Steps line by line, creates or updates monitors
      > Adds Cronitor integration to your crontab and saves the file in place.


In all of these examples, auto discover is enabled by adding 'cronitor discover --auto' to your crontab as an hourly task. Auto discover will push schedule changes
to Cronitor and alert if you if new jobs are added to your crontab without monitoring."
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		// If this is being run from a script, cron, etc, it probably wont have a PS1
		if os.Getenv("PS1") == "" {
			isAutoDiscover = true
		}

		// If this is being run by cronitor exec, don't write anything to stdout
		if os.Getenv("CRONITOR_EXEC") == "1" {
			isAutoDiscover = true
			isSilent = true
		}

		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("you must provide a valid API key with this command or save a key using 'cronitor configure'")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		crontabStrings, errCode, err := readCrontab(args)
		if err != nil {
			fatal("Problem: " + err.Error() + "\n", errCode)
		}

		crontabLines, parsedTimezoneLocationName := parseCrontab(crontabStrings)

		// If a timezone env var is set in the crontab it takes precedence over system tz
		if parsedTimezoneLocationName != nil {
			timezone = *parsedTimezoneLocationName
		} else {
			timezone = effectiveTimezoneLocationName()
		}

		// Read crontabLines into map of Monitor structs
		monitors := map[string]*Monitor{}
		allNameCandidates := map[string]bool{}

		if !isAutoDiscover {
			count := 0
			for _, line := range crontabLines {
				if line.IsMonitorable() {
					count++
				}
			}

			fmt.Printf("Found %d cron jobs in %s:\n", count, crontabPath)
		}

		for _, line := range crontabLines {
			if !line.IsMonitorable() {
				continue
			}

			rules := []Rule{createRule(line.CronExpression)}
			defaultName := createDefaultName(line.CommandToRun, line.RunAs, line.LineNumber, line.IsAutoDiscoverCommand(), effectiveHostname(), excludeFromName, allNameCandidates)
			key := createKey(line.CommandToRun, line.CronExpression, line.RunAs, line.IsAutoDiscoverCommand(), getCrontabPath())
			tags := createTags()
			name := defaultName

			if !isAutoDiscover {
				fmt.Println("\n" + line.FullLine)
				for {
					prompt := promptui.Prompt{
						Label: "Monitor name",
						Default: name,
						Validate: validateNameFormat,
						AllowEdit: name != defaultName,
					}

					if result, err := prompt.Run(); err == nil {
						name = result
						if err := validateNameUniqueness(result, key); err != nil {
							fmt.Println("Sorry! You are already using this name. Choose a unique name.\n")
							continue
						}
					} else if err == promptui.ErrInterrupt {
						fmt.Println("Exited by user signal")
						os.Exit(-1)
					} else {
						fmt.Println("Error: " + err.Error() + "\n")
					}

					break
				}
			}

			if name == defaultName {
				name = ""
			}

			notificationListMap := map[string][]string{}
			if notificationList != "" {
				notificationListMap = map[string][]string{"templates": {notificationList}}
			}

			line.Mon = Monitor{
				name,
				defaultName,
				key,
				rules,
				tags,
				"heartbeat",
				line.Code,
				timezone.Name,
				createNote(line.LineNumber, line.IsAutoDiscoverCommand()),
				notificationListMap,
			}


			monitors[key] = &line.Mon
		}

		// Put monitors to Cronitor API
		monitors, err = putMonitors(monitors)
		if err != nil {
			fatal(err.Error(), 1)
		}

		// Re-write crontab lines with new/updated monitoring
		var cl []string
		for _, line := range crontabLines {
			cl = append(cl, createCrontabLine(line))
		}

		updatedCrontabLines := strings.Join(cl, "\n") + "\n"

		if !isSilent {
			// When running --auto mode, you should be able to pipe or redirect crontab output elsewhere. Skip status-related messages.
			if !isAutoDiscover {
				fmt.Println("\n\nCrontab with monitoring:")
			}

			fmt.Println(updatedCrontabLines)

			if !isAutoDiscover && !saveCrontabFile {
				saveCommand := strings.Join(os.Args, " ")
				fmt.Println("\n\nTo install the updated crontab, use:")
				fmt.Println(fmt.Sprintf("%s --auto --save\n", saveCommand))
			}
		}

		if saveCrontabFile {
			if err := saveCrontab(updatedCrontabLines, isUserCrontab); err == nil && !isSilent {
				fmt.Println(fmt.Sprintf("Success: %s updated", crontabPath))
			} else if !isSilent {
				fatal("Problem: " + err.Error(), 126)
			}
		}

	},
}

func readCrontab(args []string) ([]string, int, error) {

	var crontabBytes []byte

	// If no crontab path was provided, attempt to load user crontab
	if len(args) == 0 {
		crontabPath = "user crontab"
		isUserCrontab = true
		if runtime.GOOS == "windows" {
			return nil, 126, errors.New("on Windows, a crontab path argument is required")
		}

		if u, err := user.Current(); err == nil {
			crontabPath = fmt.Sprintf("user \"%s\" crontab", u.Username)
		}

		cmd := exec.Command("crontab", "-l")
		if b, err := cmd.Output(); err == nil {
			crontabBytes = b
		} else {
			return nil, 126, errors.New("your user crontab file doesn't exist or couldn't be read. Try passing a crontab path instead")
		}
	} else {
		crontabPath = args[0]
		isUserCrontab = false
		if _, err := os.Stat(crontabPath); os.IsNotExist(err) {
			return nil, 66, errors.New(fmt.Sprintf("the file %s does not exist", crontabPath))
		}

		if b, err := ioutil.ReadFile(crontabPath); err == nil {
			crontabBytes = b
		} else {
			return nil, 126, errors.New(fmt.Sprintf("the crontab file at %s could not be read; check permissions and try again", crontabPath))
		}

		// When the save flag is passed, attempt to write the file back to itself to ensure we have proper permissions before going further
		if saveCrontabFile {
			if ioutil.WriteFile(crontabPath, crontabBytes, 0644) != nil {
				return nil, 126, errors.New(fmt.Sprintf("the --save option is supplied but the file at %s could not be written; check permissions and try again", crontabPath))
			}
		}
	}

	if len(crontabBytes) == 0 {
		return nil, 126, errors.New("the crontab file is empty")
	}

	return strings.Split(string(crontabBytes), "\n"), 0, nil
}

func saveCrontab(crontabLines string, isUserCrontab bool) error {
	if crontabLines == "" {
		// Shouldn't be possible..
		return errors.New("the --save option is supplied but updated crontab file is empty")
	}

	if isUserCrontab {
		cmd := exec.Command("crontab", "-")

		// crontab will use whatever $EDITOR is set. Temporarily just cat it out.
		cmd.Env = []string{"EDITOR=/bin/cat"}
		cmdStdin, _ := cmd.StdinPipe()
		cmdStdin.Write([]byte(crontabLines))
		cmdStdin.Close()
		if _, err := cmd.Output(); err != nil {
			return errors.New("The --save option is supplied but crontab update failed: " + err.Error())
		}

	} else {
		if ioutil.WriteFile(crontabPath, []byte(crontabLines), 0644) != nil {
			return errors.New(fmt.Sprintf("The --save option is supplied but the file at %s could not be written; check permissions and try again", crontabPath))
		}
	}

	return nil
}

func putMonitors(monitors map[string]*Monitor) (map[string]*Monitor, error) {
	url := apiUrl()
	if isAutoDiscover {
		url = url + "?auto-discover=1"
	}

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

	buf.Truncate(0)
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
	lineParts = append(lineParts, line.RunAs)

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

	return strings.Replace(strings.Join(lineParts, " "), "  ", " ", -1)
}

func parseCrontab(lines []string) ([]*Line, *TimezoneLocationName) {
	// returns
	var crontabLines []*Line
	var autoDiscoverLine *Line
	var usesSixFieldCronExpression bool
	var timezoneLocationName *TimezoneLocationName

	for lineNumber, fullLine := range lines {
		var cronExpression string
		var command []string
		var runAs string

		fullLine = strings.TrimSpace(fullLine)

		// Do not attempt to parse the current line if it's a comment
		// Otherwise split on any whitespace and parse
		if !strings.HasPrefix(fullLine, "#") {
			splitLine := strings.Fields(fullLine)
			splitLineLen := len(splitLine)
			if splitLineLen == 1 && strings.Contains(splitLine[0], "=") {
				// Handling for environment variables... we're looking for timezone declarations
				if splitExport := strings.Split(splitLine[0], "="); splitExport[0] == "TZ" || splitExport[0] == "CRON_TZ" {
					timezoneLocationName = &TimezoneLocationName{splitExport[1]}
				}
			} else if splitLineLen > 0 && strings.HasPrefix(splitLine[0], "@") {
				// Handling for special cron @keyword
				cronExpression = splitLine[0]
				command = splitLine[1:]
			} else if splitLineLen >= 6 {
				// Handling for javacron-style 6 item cron expressions
				usesSixFieldCronExpression = splitLineLen >= 7 && isSixFieldCronExpression(splitLine)

				if usesSixFieldCronExpression {
					cronExpression = strings.Join(splitLine[0:6], " ")
					command = splitLine[6:]
				} else {
					cronExpression = strings.Join(splitLine[0:5], " ")
					command = splitLine[5:]
				}
			}
		}

		// Try to determine if the command begins with a "run as" user designation
		// Basically, just see if the first word of the command is a valid user name. This is how vixie cron does it.
		// https://github.com/rhuitl/uClinux/blob/master/user/vixie-cron/entry.c#L224
		if runtime.GOOS != "windows" && len(command) > 1 {
			idOrError, _ := exec.Command("id", "-u", command[0]).CombinedOutput()
			if _, err := strconv.Atoi(strings.TrimSpace(string(idOrError))); err == nil {
			    runAs = command[0]
			    command = command[1:]
			}
		}

		// Create a Line struct with details for this line so we can re-create it later
		line := Line{}
		line.CronExpression = cronExpression
		line.FullLine = fullLine
		line.LineNumber = lineNumber
		line.RunAs = runAs

		// If this job is already being wrapped by the Cronitor client, read current code.
		// Expects a wrapped command to look like: cronitor exec d3x0 /path/to/cmd.sh
		if len(command) > 1 && strings.HasSuffix(command[0], "cronitor") && command[1] == "exec" {
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

	return crontabLines, timezoneLocationName
}

func createAutoDiscoverLine(usesSixFieldCronExpression bool) *Line {
	cronExpression := fmt.Sprintf("%d * * * *", randomMinute())
	if usesSixFieldCronExpression {
		cronExpression = fmt.Sprintf("* %s", cronExpression)
	}

	// Normalize the command so it can be run reliably from crontab.
	commandToRun := strings.Join(os.Args, " ")
	commandToRun = strings.Replace(commandToRun, "--save", "", -1)
	commandToRun = strings.Replace(commandToRun, "--verbose", "", -1)
	commandToRun = strings.Replace(commandToRun, "-v", "", -1)
	commandToRun = strings.Replace(commandToRun, "--interactive", "", -1)
	commandToRun = strings.Replace(commandToRun, "-i", "", -1)
	commandToRun = strings.Replace(commandToRun, crontabPath, getCrontabPath(), -1)

	// Remove existing --auto flag before adding a new one to prevent doubling up
	commandToRun = strings.Replace(commandToRun, "--auto", "", -1)
	commandToRun = strings.Replace(commandToRun, " discover ", " discover --auto ", -1)

	line := Line{}
	line.CronExpression = cronExpression
	line.CommandToRun = commandToRun
	line.FullLine = fmt.Sprintf("%s %s", line.CronExpression, line.CommandToRun)
	return &line
}

func createNote(LineNumber int, IsAutoDiscoverCommand bool) string {
	if IsAutoDiscoverCommand {
		return fmt.Sprintf("Watching for schedule changes and new entries in %s", crontabPath)
	}

	return fmt.Sprintf("Discovered in %s L%d", getCrontabPath(), LineNumber)
}

func createDefaultName(CommandToRun string, RunAs string, LineNumber int, IsAutoDiscoverCommand bool, effectiveHostname string, excludeFromName []string, allNameCandidates map[string]bool) string {
	excludeFromName = append(excludeFromName, "2>&1")
	excludeFromName = append(excludeFromName, "/bin/bash -l -c")
	excludeFromName = append(excludeFromName, "/bin/bash -lc")
	excludeFromName = append(excludeFromName, "/bin/bash -c -l")
	excludeFromName = append(excludeFromName, "/bin/bash -cl")

	excludeFromName = append(excludeFromName, "'")
	excludeFromName = append(excludeFromName, "\"")
	excludeFromName = append(excludeFromName, "\\")

	if IsAutoDiscoverCommand {
		return truncateString(fmt.Sprintf("[%s] Auto discover %s", effectiveHostname, strings.TrimSpace(crontabPath)), maxNameLen)
	}

	// Remove output redirection
	for _, redirectionOperator := range []string{">>", ">"} {
		if operatorPosition := strings.LastIndex(CommandToRun, redirectionOperator) ; operatorPosition > 0 {
			CommandToRun = CommandToRun[:operatorPosition]
			break
		}
	}

	// Remove exclusion text
	for _, substr := range excludeFromName {
		CommandToRun = strings.Replace(CommandToRun, substr, "", -1)
	}

	CommandToRun = strings.Join(strings.Fields(CommandToRun), " ")

	// Assemble the candidate name.
	// Ensure we don't produce dupe names if the user has same command on multiple lines.
	formattedRunAs := ""
	if RunAs != "" {
		formattedRunAs = fmt.Sprintf("%s ", RunAs)
	}

	formattedHostname := ""
	if effectiveHostname != "" {
		formattedHostname = fmt.Sprintf("[%s] ", effectiveHostname)
	}

	candidate := formattedHostname + formattedRunAs + CommandToRun

	if _, exists := allNameCandidates[candidate]; exists {
		candidate = fmt.Sprintf("%s L%d", candidate, LineNumber)
	} else {
		allNameCandidates[candidate] = true
	}

	// Return if short, truncate if necessary.
	if maxNameLen >= len(candidate) {
		return candidate
	}

	// Keep the first and last portion of the command
	separator := "..."
	commandPrefixLen := 20 + len(formattedHostname) + len(formattedRunAs)
	commandSuffixLen := maxNameLen - commandPrefixLen - len(separator)
	return fmt.Sprintf("%s%s%s", strings.TrimSpace(candidate[:commandPrefixLen]), separator, strings.TrimSpace(candidate[len(candidate) - commandSuffixLen:]))
}

func createKey(CommandToRun string, CronExpression string, RunAs string, IsAutoDiscoverCommand bool, crontabPath string) string {
	if IsAutoDiscoverCommand {
		// Go out of our way to prevent making a duplicate monitor for an auto-discovery command.
		CommandToRun = "auto discover " + crontabPath
		RunAs = ""
		CronExpression = ""
	}

	// Always use os.Hostname when creating a key so the key does not change when a user modifies their hostname using param/var
	hostname, _ := os.Hostname()
	data := []byte(fmt.Sprintf("%s-%s-%s-%s", hostname, CommandToRun, CronExpression, RunAs))
	return fmt.Sprintf("%x", sha1.Sum(data))
}

func createTags() []string {
	var tags []string
	tags = append(tags, "cron-job")
	return tags
}

func createRule(cronExpression string) Rule {
	return Rule{"not_on_schedule", cronExpression, "", 0}
}

func sendHttpPut(url string, body string) ([]byte, error) {
	client := &http.Client{
		Timeout: 120 * time.Second,
	}
	request, err := http.NewRequest("PUT", url, strings.NewReader(body))
	request.SetBasicAuth(viper.GetString(varApiKey), "")
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
		raven.CaptureErrorAndWait(err, nil)
		return nil, err
	}

	return contents, nil
}

func validateNameUniqueness(candidateName string, key string) error {
	client := &http.Client{
		Timeout: 3 * time.Second,
	}
	url := fmt.Sprintf("%s/%s", apiUrl(), url.QueryEscape(candidateName))
	request, err := http.NewRequest("GET", url, strings.NewReader(""))
	request.SetBasicAuth(viper.GetString(varApiKey), "")
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("User-Agent", userAgent)
	request.ContentLength = 0
	response, err := client.Do(request)

	if err != nil || response.StatusCode == http.StatusNotFound {
		return nil
	}

	defer response.Body.Close()

	contents, err := ioutil.ReadAll(response.Body)

	responseMonitor := struct{Key string}{}
	if err = json.Unmarshal(contents, &responseMonitor); err != nil {
		log("Could not verify uniqueness: " + err.Error())
		log(string(contents))
		return nil
	}

	if responseMonitor.Key == key {
		return nil
	}

	return errors.New("name already exists")
}

func validateNameFormat(candidateName string) error {
	candidateName = strings.TrimSpace(candidateName)
	if candidateName == "" {
		return errors.New("A unique name is required")
	}

	if inputLen := len(candidateName); inputLen > maxNameLen {
		return errors.New(fmt.Sprintf("Name is too long: %d of %d chars", inputLen, maxNameLen))
	}

	return nil
}

func randomMinute() int {
	rand.Seed(time.Now().Unix())
	return rand.Intn(59)
}

func getCrontabPath() string {
	if isUserCrontab {
		return crontabPath
	}

	if absoluteCronPath, err := filepath.Abs(crontabPath); err == nil {
		return absoluteCronPath
	}

	return crontabPath
}

func isSixFieldCronExpression(splitLine []string) bool {
	matchDigitOrWildcard, _ := regexp.MatchString("^[-,?*/0-9]+$", splitLine[5])
	matchDayOfWeekStringRange, _ := regexp.MatchString("^(Mon|Tue|Wed|Thr|Fri|Sat|Sun)(-(Mon|Tue|Wed|Thr|Fri|Sat|Sun))?$", splitLine[5])
	matchDayOfWeekStringList, _ := regexp.MatchString("^((Mon|Tue|Wed|Thr|Fri|Sat|Sun),?)+$", splitLine[5])
	return matchDigitOrWildcard || matchDayOfWeekStringRange || matchDayOfWeekStringList
}

func init() {
	RootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().BoolVar(&saveCrontabFile, "save", saveCrontabFile, "Save the updated crontab file")
	discoverCmd.Flags().StringArrayVarP(&excludeFromName, "exclude-from-name", "e", excludeFromName, "Substring to exclude from generated monitor name e.g. $ cronitor discover -e '> /dev/null' -e '/path/to/app'")
	discoverCmd.Flags().BoolVar(&noAutoDiscover, "no-auto-discover", noAutoDiscover, "Do not attach an automatic discover job to this crontab, or remove if already attached.")
	discoverCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes.")
	discoverCmd.Flags().StringVar(&notificationList, "notification-list", notificationList, "Use the provided notification list when creating or updating monitors, or \"default\" list if omitted.")

	discoverCmd.Flags().BoolVar(&isAutoDiscover, "auto", isAutoDiscover, "Do not use an interactive shell. Write updated crontab to stdout.")
}
