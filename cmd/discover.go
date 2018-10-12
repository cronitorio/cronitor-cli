package cmd

import (
	"fmt"
	"encoding/json"
	"io/ioutil"
	"strings"
	"github.com/spf13/cobra"
	"errors"
	"net/http"
	"os"
	"github.com/spf13/viper"
	"time"
	"bytes"
	"github.com/manifoldco/promptui"
	"github.com/getsentry/raven-go"
	"net/url"
	"os/user"
	"cronitor/lib"
	"github.com/fatih/color"
)

var excludeFromName []string
var interactiveMode = true
var isAutoDiscover bool
var isSilent bool
var noAutoDiscover bool
var saveCrontabFile bool
var crontabPath string
var isUserCrontab bool
var timezone lib.TimezoneLocationName
var maxNameLen = 75
var notificationList string

var TextHeadline = color.New(color.FgHiGreen, color.Bold)
var TextEmphasize = color.New(color.FgHiWhite, color.Underline)
var TextWarning = color.New(color.FgHiYellow, color.Bold)

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
		var username string
		if u, err := user.Current(); err == nil {
			username = u.Username
		}

		count := 0
		if len(args) > 0 {
			processCrontab(username, args[0])
		} else {
			// Process the user crontab and then enumerate crontabs in the drop-in directory
			if processCrontab(username, "") {
				count++
			}

			dropInFiles := lib.EnumerateCrontabFiles()
			for _, crontabFile := range dropInFiles {
				if count > 0 {
					fmt.Println()
				}

				if processCrontab(username, crontabFile) {
					count++
				}
			}
		}

		if !isAutoDiscover && !saveCrontabFile {
			saveCommand := strings.Join(os.Args, " ")
			fmt.Println("\n")

			if count > 0 {
				TextHeadline.Println("► To install the updated crontab(s), run:")
				fmt.Println(fmt.Sprintf("%s --auto --save\n", saveCommand))
			}
		}
	},
}

func processCrontab(username string, filename string) bool {
	crontab := &lib.Crontab{
		User:          username,
		IsUserCrontab: filename == "",
		Filename:      filename,
		MutateAndSave: saveCrontabFile,
	}

	if err, errCode := crontab.Parse(noAutoDiscover); err != nil {
		fatal("Problem: "+err.Error()+"\n", errCode)
	}

	// If a timezone env var is set in the crontab it takes precedence over system tz
	if crontab.TimezoneLocationName != nil {
		timezone = *crontab.TimezoneLocationName
	} else {
		timezone = effectiveTimezoneLocationName()
	}

	// Read crontabLines into map of Monitor structs
	monitors := map[string]*lib.Monitor{}
	allNameCandidates := map[string]bool{}

	if !isAutoDiscover {
		count := 0
		for _, line := range crontab.Lines {
			if line.IsMonitorable() {
				count++
			}
		}

		if count == 0 {
			TextWarning.Printf("► No cron jobs found in %s or /etc/cron.d", crontab.DisplayName())
			return false // Sorry about returning in some random spot, this could use some refactoring
		} else {
			label := "jobs"
			if count == 1 {
				label = "job"
			}
			TextHeadline.Printf("► Found %d cron %s in %s:", count, label, crontab.DisplayName())
			fmt.Println()
		}
	}

	for _, line := range crontab.Lines {
		if !line.IsMonitorable() {
			continue
		}

		rules := []lib.Rule{createRule(line.CronExpression)}
		defaultName := createDefaultName(line.CommandToRun, line.RunAs, line.LineNumber, line.IsAutoDiscoverCommand(), effectiveHostname(), excludeFromName, allNameCandidates)
		key := line.Key(crontab.CanonicalName())
		tags := createTags()
		name := defaultName

		if !isAutoDiscover {
			fmt.Println("\n" + line.FullLine)
			for {
				prompt := promptui.Prompt{
					Label:     "Monitor name",
					Default:   name,
					Validate:  validateNameFormat,
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

		line.Mon = lib.Monitor{
			Name:             name,
			DefaultName:      defaultName,
			Key:              key,
			Rules:            rules,
			Tags:             tags,
			Type:             "heartbeat",
			Code:             line.Code,
			Timezone:         timezone.Name,
			Note:             createNote(line.LineNumber, line.IsAutoDiscoverCommand(), crontab.CanonicalName()),
			Notifications:    notificationListMap,
			NoStdoutPassthru: noStdoutPassthru,
		}

		monitors[key] = &line.Mon
	}

	// Put monitors to Cronitor API
	var err error
	monitors, err = putMonitors(monitors)
	if err != nil {
		fatal(err.Error(), 1)
	}

	// Re-write crontab lines with new/updated monitoring
	updatedCrontabLines := crontab.Write()

	if !isSilent {
		// When running --auto mode, you should be able to pipe or redirect crontab output elsewhere. Skip status-related messages.
		if !isAutoDiscover {
			fmt.Println("")
			TextEmphasize.Println("Crontab with monitoring:")
			fmt.Println()
		}

		fmt.Println(updatedCrontabLines)
	}

	if saveCrontabFile {
		if err := crontab.Save(updatedCrontabLines); err == nil && !isSilent {
			fmt.Println(fmt.Sprintf("Success: %s updated", crontabPath))
		} else if !isSilent {
			fatal("Problem: "+err.Error(), 126)
		}
	}

	return len(monitors) > 0
}

func putMonitors(monitors map[string]*lib.Monitor) (map[string]*lib.Monitor, error) {
	url := apiUrl()
	if isAutoDiscover {
		url = url + "?auto-discover=1"
	}

	monitorsArray := make([]lib.Monitor, 0, len(monitors))
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

	responseMonitors := []lib.Monitor{}
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

func createNote(LineNumber int, IsAutoDiscoverCommand bool, CanonicalName string) string {
	if IsAutoDiscoverCommand {
		return fmt.Sprintf("Watching for schedule changes and new entries in %s", crontabPath)
	}

	return fmt.Sprintf("Discovered in %s L%d", CanonicalName, LineNumber)
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


func createTags() []string {
	var tags []string
	tags = append(tags, "cron-job")
	return tags
}

func createRule(cronExpression string) lib.Rule {
	return lib.Rule{"not_on_schedule", cronExpression, "", 0}
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

func init() {
	RootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().BoolVar(&saveCrontabFile, "save", saveCrontabFile, "Save the updated crontab file")
	discoverCmd.Flags().StringArrayVarP(&excludeFromName, "exclude-from-name", "e", excludeFromName, "Substring to exclude from generated monitor name e.g. $ cronitor discover -e '> /dev/null' -e '/path/to/app'")
	discoverCmd.Flags().BoolVar(&noAutoDiscover, "no-auto-discover", noAutoDiscover, "Do not attach an automatic discover job to this crontab, or remove if already attached.")
	discoverCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes.")
	discoverCmd.Flags().StringVar(&notificationList, "notification-list", notificationList, "Use the provided notification list when creating or updating monitors, or \"default\" list if omitted.")

	discoverCmd.Flags().BoolVar(&isAutoDiscover, "auto", isAutoDiscover, "Do not use an interactive shell. Write updated crontab to stdout.")
}
