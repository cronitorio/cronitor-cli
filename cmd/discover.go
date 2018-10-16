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

var importedCrontabs = 0
var excludeFromName []string
var isAutoDiscover bool
var isSilent bool
var noAutoDiscover bool
var saveCrontabFile bool
var timezone lib.TimezoneLocationName
var maxNameLen = 75
var notificationList string

var discoverCmd = &cobra.Command{
	Use:   "discover <optional path>",
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
      > Instead of the user crontab, provide a crontab file (or directory of crontabs) to use

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

		if len(args) > 0 {
			// A supplied argument can be a specific file or a directory
			if isPathToDirectory(args[0]) {
				processDirectory(username, args[0])
			} else {
				processCrontab(lib.CrontabFactory(username, args[0]))
			}
		} else {
			// Without a supplied argument look at the user crontab and the system drop-in directory
			if processCrontab(lib.CrontabFactory(username, "")) {
				importedCrontabs++
			}

			// Only iterate through the drop-in directory when running in interactive mode
			if !isAutoDiscover {
				processDirectory(username, lib.DROP_IN_DIRECTORY)
			}
		}

		if !isAutoDiscover && !saveCrontabFile {
			saveCommand := strings.Join(os.Args, " ")
			fmt.Println()

			label := "crontabs"
			if importedCrontabs == 1 {
				label = "crontab"
			}

			if importedCrontabs > 0 {
				printSuccessText(fmt.Sprintf("► To install the updated %s, run:", label))
				fmt.Println(fmt.Sprintf("%s --auto --save\n", saveCommand))
			}
		}

		printSuccessText("✔ Discover complete")
	},
}

func processDirectory(username, directory string) {
	// Look for crontab files in the system drop-in directory but only prompt to import them
	// if the directory is writable for this user.
	files := lib.EnumerateCrontabFiles(directory)
	if len(files) > 0 {
		testCrontab := lib.CrontabFactory(username, files[0])
		if !testCrontab.IsWritable() {
			return
		}

		label := "crontabs"
		if len(files) == 1 {
			label = "crontab"
		}

		printSuccessText(fmt.Sprintf("\n► Found %d %s in directory %s", len(files), label, directory))

		var result string
		var err error = nil

		if !isAutoDiscover {
			prompt := promptui.Prompt{
				Label: fmt.Sprintf("Would you like to import cron jobs from %s", directory),
				IsConfirm: true,
			}

			result, err = prompt.Run()

			if err == promptui.ErrInterrupt {
				printErrorText("Exited by user signal")
				os.Exit(-1)
			} else if err == promptui.ErrAbort {
				printWarningText(fmt.Sprintf("✔ Skipping %s", directory))
			} else if err != nil {
				printErrorText("Error: " + err.Error() + "\n")
			}
		}

		if isAutoDiscover || result == "y" {
			for _, crontabFile := range files {
				if importedCrontabs > 0 {
					fmt.Println()
				}

				if processCrontab(lib.CrontabFactory(username, crontabFile)) {
					importedCrontabs++
				}
			}
		}
	}
}

func processCrontab(crontab *lib.Crontab) bool {
	printSuccessText(fmt.Sprintf("► Reading %s", crontab.DisplayName()))

	// Before going further, ensure we aren't going to run into permissions problems writing the crontab, when applicable
	if saveCrontabFile && !crontab.IsWritable() {
		printWarningText(fmt.Sprintf("► The --save option is supplied but the file at %s is not writeable. Skipping.", crontab.DisplayName()))
		return false
	}

	if err, _ := crontab.Parse(noAutoDiscover); err != nil {
		printWarningText(fmt.Sprintf("► Skipping: %s", err.Error()))
		fmt.Println()
		return false
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
			if line.IsMonitorable() && !line.IsAutoDiscoverCommand() {
				count++
			}
		}

		if count == 0 {
			printWarningText("► No cron jobs found. Skipping.")
			return false // Sorry about returning in some random spot, this could use some refactoring
		} else {
			label := "jobs"
			if count == 1 {
				label = "job"
			}
			printSuccessText(fmt.Sprintf("► Found %d cron %s:", count, label))
		}
	}

	for _, line := range crontab.Lines {
		if !line.IsMonitorable() {
			continue
		}

		rules := []lib.Rule{createRule(line.CronExpression)}
		defaultName := createDefaultName(line, crontab, effectiveHostname(), excludeFromName, allNameCandidates)
		key := line.Key(crontab.CanonicalName())
		tags := createTags()
		name := defaultName

		if !isAutoDiscover && !line.IsAutoDiscoverCommand() {
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
			Note:             createNote(line, crontab),
			Notifications:    notificationListMap,
			NoStdoutPassthru: noStdoutPassthru,
		}

		monitors[key] = &line.Mon
	}

	if !isAutoDiscover {
		fmt.Println()
		printSuccessText("✔ Sending updated crontab to Cronitor:")
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
		fmt.Print(updatedCrontabLines)

		if !isAutoDiscover {
			fmt.Println("\n")
			printSuccessText("✔ Import successful")
		}

		if !isAutoDiscover {
			fmt.Println()
		}
	}

	if saveCrontabFile {
		if err := crontab.Save(updatedCrontabLines); err == nil {
			if !isSilent {
				printSuccessText("✔ Crontab save successful")
			}
		} else {
			if !isSilent {
				printErrorText("Problem saving crontab: " + err.Error())
			}
			return false
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

func createNote(line *lib.Line, crontab *lib.Crontab) string {
	if line.IsAutoDiscoverCommand() {
		return fmt.Sprintf("Watching for schedule changes and new entries in %s", crontab.DisplayName())
	}

	return fmt.Sprintf("Discovered in %s L%d", crontab.DisplayName(), line.LineNumber)
}

func createDefaultName(line *lib.Line, crontab *lib.Crontab, effectiveHostname string, excludeFromName []string, allNameCandidates map[string]bool) string {
	excludeFromName = append(excludeFromName, "2>&1")
	excludeFromName = append(excludeFromName, "/bin/bash -l -c")
	excludeFromName = append(excludeFromName, "/bin/bash -lc")
	excludeFromName = append(excludeFromName, "/bin/bash -c -l")
	excludeFromName = append(excludeFromName, "/bin/bash -cl")

	excludeFromName = append(excludeFromName, "'")
	excludeFromName = append(excludeFromName, "\"")
	excludeFromName = append(excludeFromName, "\\")

	if line.IsAutoDiscoverCommand() {
		return truncateString(fmt.Sprintf("[%s] Auto discover %s", effectiveHostname, strings.TrimSpace(crontab.DisplayName())), maxNameLen)
	}

	// Remove output redirection
	CommandToRun := line.CommandToRun
	for _, redirectionOperator := range []string{">>", ">"} {
		if operatorPosition := strings.LastIndex(line.CommandToRun, redirectionOperator) ; operatorPosition > 0 {
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
	if line.RunAs != "" {
		formattedRunAs = fmt.Sprintf("%s ", line.RunAs)
	}

	formattedHostname := ""
	if effectiveHostname != "" {
		formattedHostname = fmt.Sprintf("[%s] ", effectiveHostname)
	}

	candidate := formattedHostname + formattedRunAs + CommandToRun

	if _, exists := allNameCandidates[candidate]; exists {
		candidate = fmt.Sprintf("%s L%d", candidate, line.LineNumber)
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

func printSuccessText(message string) {
	if isAutoDiscover {
		log(message)
	} else {
		color.New(color.FgHiGreen).Println(message)
	}
}

func printWarningText(message string) {
	if isAutoDiscover {
		log(message)
	} else {
		color.New(color.FgHiYellow).Println(message)
	}
}

func printErrorText(message string) {
	if isAutoDiscover {
		log(message)
	} else {
		color.New(color.FgHiRed, color.Bold).Println(message)
	}
}

func isPathToDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fileInfo.Mode().IsDir()
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
