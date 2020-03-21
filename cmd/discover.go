package cmd

import (
	"cronitor/lib"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ExistingMonitors struct {
	Monitors    []lib.MonitorSummary
	Names       []string
	CurrentKey  string
	CurrentCode string
}

func (em ExistingMonitors) HasMonitorByName(name string) bool {
	for _, value := range em.Monitors {
		if em.CurrentCode != "" {
			if value.Code == em.CurrentCode {
				continue
			}
		} else {
			if value.Key == em.CurrentKey {
				continue
			}
		}

		if value.Name == name {
			return true
		}
	}

	// We also need to check if the name has been used in this session but not yet persisted
	for _, value := range em.Names {
		if value == name {
			return true
		}
	}

	return false
}

func (em ExistingMonitors) GetNameForCurrent() (string, error) {
	for _, value := range em.Monitors {
		if em.CurrentCode != "" {
			if value.Code == em.CurrentCode {
				return value.Name, nil
			}
		} else {
			if value.Key == em.CurrentKey {
				return value.Name, nil
			}
		}
	}
	return "", errors.New("does not exist")
}

func (em ExistingMonitors) AddName(name string) {
	em.Names = append(em.Names, name)
}

var importedCrontabs = 0
var excludeFromName []string
var isAutoDiscover bool
var isSilent bool
var saveCrontabFile bool
var dryRun bool
var timezone lib.TimezoneLocationName
var maxNameLen = 75
var notificationList string
var existingMonitors = ExistingMonitors{}

// To deprecate this feature we are hijacking this flag that will trigger removal of auto-discover lines from existing user's crontabs.
var noAutoDiscover = true

var discoverCmd = &cobra.Command{
	Use:   "discover <optional path>",
	Short: "Attach monitoring to new cron jobs and watch for schedule updates",
	Long: `
Cronitor discover will parse your crontab and create or update monitors using the Cronitor API.

Note: You must supply your Cronitor API key. This can be passed as a flag, environment variable, or saved in your Cronitor configuration file. See 'help configure' for more details.

Example:
  $ cronitor discover
      > Read user crontab and step through line by line
      > Creates monitors on your Cronitor dashboard for each entry in the crontab. The command string will be used as the monitor name.
      > Adds Cronitor integration to your crontab

  $ cronitor discover /path/to/crontab
      > Instead of the user crontab, provide a crontab file (or directory of crontabs) to use

Example that does not use an interactive shell:
  $ cronitor discover --auto
      > The only output to stdout will be your updated crontab file, suitable for piplines or writing to another crontab.

Example excluding secrets or common text from monitor names:
  $ cronitor discover /path/to/crontab -e "secret-token" -e "/var/common/app/path/"
      > Updates previously discovered monitors or creates new monitors, excluding the provided snippets from the monitor name.
      > Adds Cronitor integration to your crontab and outputs to stdout
      > Names you create yourself in "discover" or from the dashboard are unchanged.

  You can run the command as many times as you need, accumulating exclusion params until the job names on your Cronitor dashboard are clear and readable.

Example where you perform a dry-run without any crontab modifications:
  $ cronitor discover /path/to/crontab --dry-run
      > Steps line by line, creates or updates monitors
      > Checks permissions to ensure integration can be applied later
	`,
	Args: func(cmd *cobra.Command, args []string) error {

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

		printSuccessText("Scanning for cron jobs... (Use Ctrl-C to skip)", false)

		// Fetch list of existing monitor names for easy unique name validation and prompt prefill later on
		existingMonitors.Monitors, _ = getCronitorApi().GetMonitors()

		if len(args) > 0 {
			// A supplied argument can be a specific file or a directory
			if isPathToDirectory(args[0]) {
				processDirectory(username, args[0])
			} else {
				if processCrontab(lib.CrontabFactory(username, args[0])) {
					importedCrontabs++
				}
			}
		} else {
			// Without a supplied argument look at the user crontab, the system crontab and the system drop-in directory
			if processCrontab(lib.CrontabFactory(username, "")) {
				importedCrontabs++
			}

			if systemCrontab := lib.CrontabFactory(username, lib.SYSTEM_CRONTAB); systemCrontab.Exists() {
				if processCrontab(systemCrontab) {
					importedCrontabs++
				}
			}

			processDirectory(username, lib.DROP_IN_DIRECTORY)
		}

		printDoneText("Discover complete", false)
		if dryRun {
			saveCommand := strings.Join(os.Args, " ")
			saveCommand = strings.Replace(saveCommand, " --dry-run", "", -1)

			if importedCrontabs > 0 {
				printWarningText("Reminder: This is a DRY-RUN. Integration is not complete.", true)
				printWarningText("To complete integration, run:", true)
				fmt.Println(fmt.Sprintf("      %s --auto --silent\n", saveCommand))
			}
		}
	},
}

func processDirectory(username, directory string) {
	// Look for crontab files in the system drop-in directory but only prompt to import them
	// if the directory is writable for this user.
	files := lib.EnumerateCrontabFiles(directory)
	if len(files) > 0 {
		testCrontab := lib.CrontabFactory(username, files[0])
		if !testCrontab.IsWritable() {
			printWarningText(fmt.Sprintf("Directory %s is not writable. Re-run command with sudo. Skipping", directory), false)
			return
		}

		for _, crontabFile := range files {
			if importedCrontabs > 0 {
				printLn()
			}

			if processCrontab(lib.CrontabFactory(username, crontabFile)) {
				importedCrontabs++
			}
		}
	}
}

func processCrontab(crontab *lib.Crontab) bool {
	defer printLn()
	printSuccessText(fmt.Sprintf("Checking %s", crontab.DisplayName()), false)

	if !crontab.Exists() {
		printWarningText("This crontab does not exist. Skipping.", true)
		return false
	}

	// This will mostly happen when the crontab is empty
	if err, _ := crontab.Parse(noAutoDiscover); err != nil {
		printWarningText("This crontab is empty. Skipping.", true)
		log(fmt.Sprintf("Skipping %s: %s", crontab.DisplayName(), err.Error()))
		return false
	}

	// Before going further, ensure we aren't going to run into permissions problems writing the crontab later
	if !crontab.IsWritable() {
		printWarningText(fmt.Sprintf("This crontab is not writeable. Re-run command with sudo. Skipping"), true)
		return false
	}

	// If a timezone env var is set in the crontab it takes precedence over system tz
	if crontab.TimezoneLocationName != nil {
		timezone = *crontab.TimezoneLocationName
	} else {
		timezone = effectiveTimezoneLocationName()
	}

	// This is done entirely so we can print a summary line with a count of cron jobs found in this crontab
	if !isAutoDiscover {
		count := 0
		for _, line := range crontab.Lines {
			if line.IsMonitorable() && !line.IsAutoDiscoverCommand() {
				count++
			}
		}

		label := "jobs"
		if count == 1 {
			label = "job"
		}
		printSuccessText(fmt.Sprintf("Found %d cron %s:", count, label), true)
	}

	// Read crontab into map of Monitor structs
	monitors := map[string]*lib.Monitor{}
	allNameCandidates := map[string]bool{}

	for _, line := range crontab.Lines {
		if !line.IsMonitorable() {
			continue
		}

		rules := []lib.Rule{createRule(line.CronExpression)}
		defaultName := createDefaultName(line, crontab, effectiveHostname(), excludeFromName, allNameCandidates)
		tags := createTags()
		key := line.Key(crontab.CanonicalName())
		name := defaultName
		skip := false

		// If we know this monitor exists already, return the name
		existingMonitors.CurrentKey = key
		existingMonitors.CurrentCode = line.Code
		if existingName, err := existingMonitors.GetNameForCurrent(); err == nil {
			name = existingName
		}

		if !isAutoDiscover && !line.IsAutoDiscoverCommand() {
			fmt.Println(fmt.Sprintf("\n    %s  %s", line.CronExpression, line.CommandToRun))
			for {
				prompt := promptui.Prompt{
					Label:     "Job name",
					Default:   name,
					Validate:  validateName,
					AllowEdit: name != defaultName,
					Templates: promptTemplates(),
				}

				if result, err := prompt.Run(); err == nil {
					name = result
				} else if err == promptui.ErrInterrupt {
					printWarningText("Skipped", true)
					skip = true
					break
				} else {
					printErrorText("Error: "+err.Error()+"\n", false)
				}

				break
			}
		}

		if skip {
			continue
		}

		existingMonitors.AddName(name)

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

	printLn()

	if len(monitors) > 0 {
		printDoneText("Sending to Cronitor", true)
	}

	var err error
	monitors, err = getCronitorApi().PutMonitors(monitors)
	if err != nil {
		fatal(err.Error(), 1)
	}

	// Re-write crontab lines with new/updated monitoring
	updatedCrontabLines := crontab.Write()

	if !isSilent && isAutoDiscover {
		// When running --auto mode, you should be able to pipe or redirect crontab output elsewhere. Skip status-related messages.
		fmt.Println(strings.TrimSpace(updatedCrontabLines))
	}

	if !dryRun && len(monitors) > 0 {
		if err := crontab.Save(updatedCrontabLines); err == nil {
			if !isSilent {
				printDoneText("Integration complete", true)
			}
		} else {
			if !isSilent {
				printErrorText("Problem saving crontab: "+err.Error(), true)
			}
			return false
		}
	}

	return len(monitors) > 0
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
	excludeFromName = append(excludeFromName, "/dev/null")

	excludeFromName = append(excludeFromName, "'")
	excludeFromName = append(excludeFromName, "\"")
	excludeFromName = append(excludeFromName, "\\")

	// Limit the visible hostname portion to 21 chars
	formattedHostname := ""
	if effectiveHostname != "" {
		if len(effectiveHostname) > 21 {
			effectiveHostname = fmt.Sprintf("%s...%s", effectiveHostname[:9], effectiveHostname[len(effectiveHostname)-9:])
		}
		formattedHostname = fmt.Sprintf("[%s] ", effectiveHostname)
	}

	if line.IsAutoDiscoverCommand() {
		return truncateString(fmt.Sprintf("%sAuto discover %s", formattedHostname, strings.TrimSpace(crontab.DisplayName())), maxNameLen)
	}

	// Remove output redirection
	CommandToRun := line.CommandToRun
	for _, redirectionOperator := range []string{">>", ">"} {
		if operatorPosition := strings.LastIndex(line.CommandToRun, redirectionOperator); operatorPosition > 0 {
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
	lineNumSuffix := fmt.Sprintf(" L%d", line.LineNumber)
	if line.RunAs != "" {
		formattedRunAs = fmt.Sprintf("%s ", line.RunAs)
	}

	candidate := formattedHostname + formattedRunAs + CommandToRun

	if _, exists := allNameCandidates[candidate]; exists {
		candidate = fmt.Sprintf("%s%s", candidate, lineNumSuffix)
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
	commandSuffixLen := maxNameLen - len(lineNumSuffix) - commandPrefixLen - len(separator)
	return fmt.Sprintf(
		"%s%s%s%s",
		strings.TrimSpace(candidate[:commandPrefixLen]),
		separator,
		strings.TrimSpace(candidate[len(candidate)-commandSuffixLen:]), lineNumSuffix)
}

func createTags() []string {
	var tags []string
	tags = append(tags, "cron-job")
	return tags
}

func createRule(cronExpression string) lib.Rule {
	return lib.Rule{"not_on_schedule", cronExpression, "", 0}
}

func validateName(candidateName string) error {
	candidateName = strings.TrimSpace(candidateName)
	if candidateName == "" {
		return errors.New("A unique name is required")
	}

	if existingMonitors.HasMonitorByName(candidateName) {
		return errors.New("Sorry, you already have a monitor with this name. A unique name is required")
	}

	if inputLen := len(candidateName); inputLen > maxNameLen {
		return errors.New(fmt.Sprintf("Sorry, name is too long: %d of maximum %d chars", inputLen, maxNameLen))
	}

	return nil
}

func promptTemplates() *promptui.PromptTemplates {
	bold := promptui.Styler(promptui.FGBold)
	faint := promptui.Styler(promptui.FGFaint)
	return &promptui.PromptTemplates{
		Prompt:          fmt.Sprintf("    %s {{ . | bold }}%s ", bold(promptui.IconInitial), bold(":")),
		Valid:           fmt.Sprintf("    %s {{ . | bold }}%s ", bold(promptui.IconGood), bold(":")),
		Invalid:         fmt.Sprintf("    %s {{ . | bold }}%s ", bold(promptui.IconBad), bold(":")),
		Success:         fmt.Sprintf("    {{ . | faint }}%s ", faint(":")),
		ValidationError: `    {{ ">>" | red }} {{ . | red }}`,
	}
}

func init() {
	RootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().BoolVar(&saveCrontabFile, "save", saveCrontabFile, "Save the updated crontab file")
	discoverCmd.Flags().BoolVar(&dryRun, "dry-run", dryRun, "Import crontab into Cronitor without applying necessary integration")
	discoverCmd.Flags().StringArrayVarP(&excludeFromName, "exclude-from-name", "e", excludeFromName, "Substring to exclude from auto-generated monitor name e.g. $ cronitor discover -e '> /dev/null' -e '/path/to/app'")
	discoverCmd.Flags().BoolVar(&noAutoDiscover, "no-auto-discover", noAutoDiscover, "Do not attach an automatic discover job to this crontab, or remove if already attached.")
	discoverCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes.")
	discoverCmd.Flags().StringVar(&notificationList, "notification-list", notificationList, "Use the provided notification list when creating or updating monitors, or \"default\" list if omitted.")
	discoverCmd.Flags().BoolVar(&isAutoDiscover, "auto", isAutoDiscover, "Do not use an interactive shell. Write updated crontab to stdout.")

	discoverCmd.Flags().BoolVar(&isSilent, "silent", isSilent, "")
	discoverCmd.Flags().MarkHidden("silent")

	// Since 23.0 save is deprecated
	discoverCmd.Flags().MarkDeprecated("save", "save will now happen automatically when the --dry-run flag is not used")
	discoverCmd.Flags().MarkHidden("save")

	// Since 24.0 no auto discover is deprecated
	discoverCmd.Flags().MarkDeprecated("no-auto-discover", "the auto-discover feature has been removed")
	discoverCmd.Flags().MarkHidden("no-auto-discover")

}
