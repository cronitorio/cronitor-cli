package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// getJobLabel returns the appropriate term for a job based on the platform
// Returns "scheduled task" on Windows, "cron job" on other platforms
func getJobLabel() string {
	if runtime.GOOS == "windows" {
		return "scheduled task"
	}
	return "cron job"
}

type ExistingMonitors struct {
	Loaded      bool // Monitors is the full list from Cronitor
	Monitors    []lib.Monitor
	Names       []string
	CurrentKey  string
	CurrentCode string
}

// matches reports whether m is the monitor for a line. A line's code is the
// monitor's generated code, or its key when the line was written by hand
// (CRONITOR=prune-events). A line without a code matches by its hash key.
func monitorMatches(m lib.Monitor, key, code string) bool {
	if code != "" {
		return m.Attributes.Code == code || m.Attributes.Key == code
	}
	return key != "" && m.Attributes.Key == key
}

func (em ExistingMonitors) HasMonitorByName(name string) bool {
	for _, value := range em.Monitors {
		if monitorMatches(value, em.CurrentKey, em.CurrentCode) {
			continue
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

func (em ExistingMonitors) Get(key string, code string) lib.Monitor {
	m, _ := em.Find(key, code)
	return m
}

// Find returns the monitor for a line and whether it exists.
func (em ExistingMonitors) Find(key string, code string) (lib.Monitor, bool) {
	for _, value := range em.Monitors {
		if monitorMatches(value, key, code) {
			return value, true
		}
	}
	return lib.Monitor{}, false
}

func (em ExistingMonitors) GetNameForCurrent() (string, error) {
	for _, value := range em.Monitors {
		if monitorMatches(value, em.CurrentKey, em.CurrentCode) {
			return value.Name, nil
		}
	}
	return "", errors.New("does not exist")
}

func (em *ExistingMonitors) AddName(name string) {
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
var processingMultipleCrontabs = false
var userAbortedSync = false // Set to true when user presses Ctrl+D to abort sync entirely
var syncFile string         // Path to YAML/JSON file for bulk monitor import
var syncFormat string       // --format: "", "short" or "exec"
var withMonitored bool      // --with-monitored: also ask about jobs that are already monitored

// To deprecate this feature we are hijacking this flag that will trigger removal of auto-discover lines from existing user's crontabs.
var noAutoDiscover = true

func discoverMissingAPIKeyGuidance() string {
	return fmt.Sprintf("\n%s\n\n%s Run %s to sign in with device authorization (%s is an alias).\n\n%s Copy an SDK key from https://cronitor.io/app/settings/api and save it with %s\n\n",
		color.New(color.FgRed, color.Bold).Sprint("Add your API key before running sync."),
		lipgloss.NewStyle().Bold(true).Render("New user?"),
		lipgloss.NewStyle().Italic(true).Render("cronitor auth login"),
		lipgloss.NewStyle().Italic(true).Render("cronitor signup"),
		lipgloss.NewStyle().Bold(true).Render("Existing user?"),
		lipgloss.NewStyle().Italic(true).Render("export CRONITOR_API_KEY and run cronitor configure"))
}

var discoverCmd = &cobra.Command{
	Use:     "sync <optional path>",
	Aliases: []string{"discover"},
	Short:   "Add monitoring to new cron jobs and sync changes to existing jobs",
	Long: `
Cronitor sync will parse your crontab and create or update monitors using the Cronitor API.

Note: You must supply your Cronitor API key. This can be passed as a flag, environment variable, or saved in your Cronitor configuration file. See 'help configure' for more details.

Example:
  $ cronitor sync
      > Read user crontab and step through line by line
      > Creates monitors on your Cronitor dashboard for each entry in the crontab. The command string will be used as the monitor name.
      > Adds Cronitor integration to your crontab

  $ cronitor sync /path/to/crontab
      > Instead of the user crontab, provide a crontab file (or directory of crontabs) to use

Example that does not use an interactive shell:
  $ cronitor sync --auto
      > The only output to stdout will be your updated crontab file, suitable for piplines or writing to another crontab.

Example excluding secrets or common text from monitor names:
  $ cronitor sync /path/to/crontab -e "secret-token" -e "/var/common/app/path/"
      > Updates previously discovered monitors or creates new monitors, excluding the provided snippets from the monitor name.
      > Adds Cronitor integration to your crontab and outputs to stdout
      > Names you create yourself in "sync" or from the dashboard are unchanged.

  You can run the command as many times as you need, accumulating exclusion params until the job names on your Cronitor dashboard are clear and readable.

Example where you perform a dry-run without any crontab modifications:
  $ cronitor sync /path/to/crontab --dry-run
      > Asks about new jobs and shows what would change
      > Sends nothing to Cronitor and does not write the crontab

On Unix, new jobs use a SHELL shim (CRONITOR=<key>; MONITORIO=<key> is also read). --format=short also converts cronitor exec lines; --no-stdout, --env, --api-key, --ping-api-key and --hostname become CRONITOR_* settings on the line. --format=exec writes cronitor exec and restores SHELL. Windows stays exec.
	`,
	Args: func(cmd *cobra.Command, args []string) error {

		// If this is being run by cronitor exec, don't write anything to stdout
		if os.Getenv("CRONITOR_EXEC") == "1" {
			isAutoDiscover = true
			isSilent = true
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {

		// Reset the flag for each command execution
		processingMultipleCrontabs = false

		if len(viper.GetString(varApiKey)) < 10 {
			fatal(discoverMissingAPIKeyGuidance(), 1)
		}
		if _, err := parseSyncFormat(syncFormat); err != nil {
			fatal(err.Error(), 1)
		}
		if runtime.GOOS == "windows" && syncFormat != "" {
			printWarningText("The shell shim is Unix-only. Sync keeps cronitor exec lines.", false)
		}

		// Handle --file flag for bulk monitor import from YAML/JSON file
		if syncFile != "" {
			importMonitorsFromFile(syncFile)
			return
		}

		var username string
		if u, err := user.Current(); err == nil {
			username = u.Username
		}

		printSuccessText(fmt.Sprintf("Scanning for %ss...", getJobLabel()), false)

		// Fetch list of existing monitor names for easy unique name validation and prompt prefill later on
		monitors, err := getCronitorApi().GetMonitors()
		existingMonitors.Monitors = monitors
		existingMonitors.Loaded = err == nil
		if err != nil {
			syncWarning(fmt.Sprintf("Could not load your monitors from Cronitor: %v. Sync cannot tell which jobs changed, so it asks about every job and sends each schedule.", err))
		}

		if runtime.GOOS == "windows" {
			if processWindowsTaskScheduler() {
				importedCrontabs++
			}
		} else {
			var plans []*crontabPlan
			if len(args) > 0 {
				// A supplied argument can be a specific file or a directory
				if isPathToDirectory(args[0]) {
					processingMultipleCrontabs = true
					collectDirectory(&plans, username, args[0])
				} else {
					collectCrontab(&plans, lib.CrontabFactory(username, args[0]))
				}
			} else {
				// Without a supplied argument look at user crontabs, the system crontab and the system drop-in directory
				processingMultipleCrontabs = true

				// Process crontabs for all configured users
				users := parseUsers()
				if len(users) == 0 {
					// Default to current user if no users configured
					users = []string{username}
				}

				for _, user := range users {
					if userAbortedSync {
						break
					}
					collectCrontab(&plans, lib.CrontabFactory(user, fmt.Sprintf("user:%s", user)))
				}

				if !userAbortedSync {
					if systemCrontab := lib.CrontabFactory(username, lib.SYSTEM_CRONTAB); systemCrontab.Exists() {
						collectCrontab(&plans, systemCrontab)
					}
				}

				if !userAbortedSync {
					collectDirectory(&plans, username, lib.DROP_IN_DIRECTORY)
				}
			}
			importedCrontabs += finishSync(plans)
		}

		if userAbortedSync {
			printWarningText("Sync aborted", false)
		} else {
			printDoneText("Sync complete", false)
			printSuccessText("View your dashboard https://cronitor.io/app/dashboard", false)
		}
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

// collectDirectory plans every crontab in a directory, if this user can
// write to it.
func collectDirectory(plans *[]*crontabPlan, username, directory string) {
	files := lib.EnumerateFiles(directory)
	if len(files) == 0 {
		return
	}
	if !lib.CrontabFactory(username, files[0]).IsWritable() {
		printWarningText(fmt.Sprintf("Directory %s is not writable. Re-run command with sudo. Skipping", directory), false)
		return
	}
	for _, crontabFile := range files {
		if userAbortedSync {
			break
		}
		collectCrontab(plans, lib.CrontabFactory(username, crontabFile))
	}
}

// importMonitorsFromFile imports monitors from a YAML or JSON file
// YAML files are sent with Content-Type: application/yaml
// JSON files are sent with Content-Type: application/json
func importMonitorsFromFile(filePath string) {
	printSuccessText(fmt.Sprintf("Importing monitors from %s...", filePath), false)

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fatal(fmt.Sprintf("Failed to read file: %s", err.Error()), 1)
	}

	// Determine content type based on file extension
	ext := strings.ToLower(filepath.Ext(filePath))
	var contentType string

	switch ext {
	case ".yaml", ".yml":
		contentType = "application/yaml"
		// Basic validation - try to parse YAML
		var yamlData interface{}
		if err := yaml.Unmarshal(data, &yamlData); err != nil {
			fatal(fmt.Sprintf("Failed to parse YAML: %s", err.Error()), 1)
		}
	case ".json":
		contentType = "application/json"
		// Basic validation - try to parse JSON
		var jsonData interface{}
		if err := json.Unmarshal(data, &jsonData); err != nil {
			fatal(fmt.Sprintf("Failed to parse JSON: %s", err.Error()), 1)
		}
	default:
		fatal(fmt.Sprintf("Unsupported file format: %s (use .yaml, .yml, or .json)", ext), 1)
	}

	// Send to the API with the appropriate content type
	printDoneText("Sending to Cronitor...", false)

	response, err := getCronitorApi().PutRawMonitors(data, contentType)
	if err != nil {
		fatal(fmt.Sprintf("API error: %s", err.Error()), 1)
	}

	// Try to parse response to show results
	// Response format may vary based on input format
	var result struct {
		Monitors []struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		} `json:"monitors"`
	}
	if err := json.Unmarshal(response, &result); err == nil && len(result.Monitors) > 0 {
		printDoneText(fmt.Sprintf("Successfully synced %d monitor(s)", len(result.Monitors)), false)
		for _, m := range result.Monitors {
			name := m.Name
			if name == "" {
				name = m.Key
			}
			printSuccessText(fmt.Sprintf("  • %s", name), false)
		}
	} else {
		// For YAML responses or other formats, just show success
		printDoneText("Monitors synced successfully", false)
	}

	printSuccessText("View your dashboard: https://cronitor.io/app/dashboard", false)
}

type syncFormatMode int

const (
	formatDefault syncFormatMode = iota
	formatShort
	formatExec
)

func parseSyncFormat(value string) (syncFormatMode, error) {
	switch value {
	case "":
		return formatDefault, nil
	case "short":
		return formatShort, nil
	case "exec":
		return formatExec, nil
	}
	return formatDefault, fmt.Errorf("--format must be short or exec, not %q", value)
}

// configureShellShim picks how the crontab is written. While planning, it
// prints the shim notices and installs nothing, so sync can see in advance
// whether the text changes. When applying, it installs the shim if needed.
func configureShellShim(crontab *lib.Crontab, planning bool) error {
	if runtime.GOOS == "windows" {
		crontab.WriteMode = lib.WriteModeLegacy
		return nil
	}
	format, _ := parseSyncFormat(syncFormat)
	switch format {
	case formatExec:
		crontab.WriteMode = lib.WriteModeExec
	default:
		crontab.WriteMode = lib.WriteModeShim
		crontab.ConvertExec = format == formatShort
	}
	if noStdoutPassthru {
		crontab.RewriteShimToExec = true
	}
	stop, notices := crontab.ShimNotices()
	for _, notice := range notices {
		if planning {
			fmt.Fprintln(os.Stderr, notice)
		}
	}
	if stop {
		crontab.WriteMode = lib.WriteModeLegacy
		if !noStdoutPassthru {
			crontab.RewriteShimToExec = false
		}
		return nil
	}
	if crontab.WriteMode == lib.WriteModeShim && syncFlagsBlockShim() {
		crontab.BlockNewShim = true
	}
	if !crontab.EmitsShim() {
		return nil
	}
	path := lib.ResolveShimInstallPath()
	crontab.ShimShellPath = path
	if dryRun || planning {
		return nil
	}
	if _, err := lib.InstallShimWrapper(path, cronitorExecutable()); err != nil {
		return fmt.Errorf("cannot install shell shim at %s: %w", path, err)
	}
	return nil
}

func syncFlagsBlockShim() bool {
	for _, name := range []string{"api-key", "hostname", "env"} {
		if f := RootCmd.PersistentFlags().Lookup(name); f != nil && f.Changed {
			return true
		}
	}
	return noStdoutPassthru || viper.GetString("CRONITOR_ENV") != ""
}

func cronitorExecutable() string {
	return invokedExecutable(os.Args[0], exec.LookPath, os.Getwd)
}

func invokedExecutable(arg0 string, lookPath func(string) (string, error), getwd func() (string, error)) string {
	slash := strings.Contains(arg0, "/") || strings.Contains(arg0, string(filepath.Separator))
	if slash {
		if filepath.IsAbs(arg0) {
			return arg0
		}
		wd, err := getwd()
		if err != nil {
			return arg0
		}
		return filepath.Join(wd, arg0)
	}
	if p, err := lookPath(arg0); err == nil && p != "" {
		return p
	}
	return arg0
}

// processCrontab plans one crontab and finishes the sync for it.
func processCrontab(crontab *lib.Crontab) bool {
	defer printLn()
	plan := planCrontab(crontab)
	if plan == nil {
		return false
	}
	return finishSync([]*crontabPlan{plan}) > 0
}

// collectCrontab plans a crontab and keeps the plan for finishSync.
func collectCrontab(plans *[]*crontabPlan, crontab *lib.Crontab) {
	if plan := planCrontab(crontab); plan != nil {
		*plans = append(*plans, plan)
	}
}

// confirm asks a yes or no question on the terminal. Tests replace it.
var confirm = func(question string, defaultYes bool) bool {
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	fmt.Printf("%s %s ", question, hint)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && answer == "" {
		fmt.Println()
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" {
		return defaultYes
	}
	return answer == "y" || answer == "yes"
}

// planCounts counts the planned actions of one crontab.
type planCounts struct {
	new, updated, unchanged, skipped, ignored, missing, comments int
}

func (p *crontabPlan) counts() planCounts {
	var c planCounts
	for _, l := range p.lines {
		if l.commentUpdated {
			c.comments++
		}
		switch l.action {
		case actionNew:
			c.new++
		case actionUpdate:
			c.updated++
		case actionUnchanged:
			c.unchanged++
		case actionSkip:
			c.skipped++
		case actionIgnore:
			c.ignored++
		case actionMissing:
			c.missing++
		}
	}
	return c
}

// hasChanges reports whether applying the plan sends or writes anything.
func (p *crontabPlan) hasChanges() bool {
	c := p.counts()
	return c.new > 0 || c.updated > 0 || c.ignored > 0 || p.rewrites
}

func (p *crontabPlan) summary() string {
	c := p.counts()
	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(c.new, "new")
	add(c.updated, "updated")
	add(c.unchanged, "unchanged")
	add(c.skipped, "skipped")
	add(c.ignored, "ignored")
	add(c.missing, "missing in Cronitor")
	add(c.comments, "# Name: comments updated")
	line := strings.Join(parts, " · ")
	if p.rewrites && c.new == 0 && c.ignored == 0 && c.comments == 0 {
		if line != "" {
			line += " · "
		}
		line += "crontab lines rewritten"
	}
	return line
}

// finishSync shows one summary for all planned crontabs, asks once, then
// applies the plans. It returns the number of crontabs that changed.
func finishSync(plans []*crontabPlan) int {
	var todo []*crontabPlan
	for _, p := range plans {
		if !p.aborted && p.hasChanges() {
			todo = append(todo, p)
		}
	}
	if userAbortedSync {
		return 0
	}
	interactive := !isAutoDiscover && !isSilent
	if len(todo) == 0 {
		if interactive {
			printDoneText("Nothing to change", false)
		}
		return 0
	}

	if interactive {
		fmt.Println()
		for _, p := range plans {
			if p.aborted {
				continue
			}
			fmt.Println("  " + p.crontab.DisplayName())
			fmt.Println("    " + p.summary())
		}
		fmt.Println()
	}
	if dryRun {
		printWarningText("Dry run: nothing was sent to Cronitor or written to the crontab", false)
		return len(todo)
	}
	if interactive {
		if !confirm("Apply these changes?", true) {
			printWarningText("Nothing was changed", false)
			return 0
		}
	}

	changed := 0
	for _, p := range todo {
		if applyPlan(p) {
			changed++
		}
	}
	return changed
}

// syncAction is what sync does with one crontab line.
type syncAction int

const (
	actionNew       syncAction = iota // create a monitor: send line.Mon
	actionUpdate                      // send only the changed fields of a monitor
	actionUnchanged                   // the monitor is up to date: send nothing
	actionMissing                     // the line has a key but Cronitor has no monitor for it
	actionSkip                        // the user skipped the line
	actionIgnore                      // the user skipped the line for good
)

type plannedLine struct {
	line    *lib.Line
	key     string // hash key of the line, see Line.Key
	action  syncAction
	payload *lib.Monitor // what to send for actionNew and actionUpdate
	// commentUpdated is set when the "# Name:" comment was changed to the
	// monitor's name in Cronitor.
	commentUpdated bool
}

// crontabPlan holds every decision for one crontab. planCrontab fills it
// without calling the API or writing anything; applyPlan carries it out.
type crontabPlan struct {
	crontab  *lib.Crontab
	lines    []plannedLine
	aborted  bool // the user quit
	rewrites bool // the crontab text changes even if no monitor does
}

type promptChoice int

const (
	choiceMonitor promptChoice = iota
	choiceSkip
	choiceIgnore // skip and write "# cronitor: ignore" so sync never asks again
	choiceQuit
)

type promptResult struct {
	choice promptChoice
	name   string
}

// askAboutJob shows one job and asks what to do with it. Tests replace it.
var askAboutJob = func(line *lib.Line, name string, monitored bool) promptResult {
	printSuccessText(fmt.Sprintf("Line %d:", line.LineNumber), true)
	fmt.Println(renderJobInfoBox(name, line.CronExpression, line.CommandToRun, monitored))

	result, err := tea.NewProgram(initialNameInputModel(name)).Run()
	if err != nil {
		printErrorText("Error: "+err.Error()+"\n", false)
		return promptResult{choice: choiceSkip}
	}
	finalModel := result.(nameInputModel)
	if finalModel.exited {
		printWarningText("Exiting", true)
		return promptResult{choice: choiceQuit}
	}
	if finalModel.ignore {
		printWarningText("Skipped. Sync will not ask about this job again.", true)
		return promptResult{choice: choiceIgnore}
	}
	if !finalModel.done {
		printWarningText("Skipped", true)
		return promptResult{choice: choiceSkip}
	}
	return promptResult{choice: choiceMonitor, name: finalModel.textInput.Value()}
}

// planCrontab reads a crontab and decides what to do with each job. It
// returns nil when the crontab cannot be used.
func planCrontab(crontab *lib.Crontab) *crontabPlan {
	if !crontab.Exists() {
		printWarningText("This crontab does not exist. Skipping.", true)
		return nil
	}

	// This will mostly happen when the crontab is empty
	if err, _ := crontab.Parse(noAutoDiscover); err != nil {
		printWarningText("This crontab is empty. Skipping.", true)
		log(fmt.Sprintf("Skipping %s: %s", crontab.DisplayName(), err.Error()))
		return nil
	}

	// Before going further, ensure we aren't going to run into permissions problems writing the crontab later
	if !crontab.IsWritable() {
		printWarningText(fmt.Sprintf("This crontab is not writeable. Re-run command with sudo. Skipping"), true)
		return nil
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
		printSuccessText(fmt.Sprintf("Found %d %s in %s", count, label, crontab.DisplayName()), true)
	}

	plan := &crontabPlan{crontab: crontab}
	allNameCandidates := map[string]bool{}

	for _, line := range crontab.Lines {
		if !line.IsMonitorable() {
			continue
		}

		// Automatically skip jobs marked with "# cronitor: ignore"
		if line.Ignored {
			continue
		}

		// Automatically skip jobs that are commented out (disabled)
		if line.IsComment {
			continue
		}

		defaultName := createDefaultName(line, crontab, effectiveHostname(), excludeFromName, allNameCandidates)
		key := line.Key(crontab.CanonicalName())

		monitored := line.Code != ""
		existing, found := existingMonitors.Find(key, line.Code)
		if monitored && !found && existingMonitors.Loaded {
			applyNoStdout(line)
			syncWarning(fmt.Sprintf("Line %d of %s: monitor %s no longer exists in Cronitor. Sync does not create it again.",
				line.LineNumber, crontab.DisplayName(), line.Code))
			plan.lines = append(plan.lines, plannedLine{line: line, key: key, action: actionMissing})
			continue
		}

		// The name to show: the monitor's name in Cronitor, then the
		// "# Name:" comment, then a generated name.
		currentName := line.Name
		if found && existing.Name != "" {
			currentName = existing.Name
		}
		name := currentName
		if name == "" {
			name = defaultName
		}

		// Ask about new jobs. Monitored jobs are asked about only with
		// --with-monitored, or when sync cannot load the monitor list.
		ask := !monitored || withMonitored || !existingMonitors.Loaded
		if ask && !isAutoDiscover && !line.IsAutoDiscoverCommand() {
			answer := askAboutJob(line, name, monitored)
			if answer.choice == choiceQuit {
				userAbortedSync = true
				plan.aborted = true
				break
			}
			if answer.choice == choiceIgnore {
				line.Ignored = true
				plan.lines = append(plan.lines, plannedLine{line: line, key: key, action: actionIgnore})
				continue
			}
			if answer.choice == choiceSkip {
				plan.lines = append(plan.lines, plannedLine{line: line, key: key, action: actionSkip})
				continue
			}
			name = answer.name
		}

		existingMonitors.AddName(name)

		if monitored {
			applyNoStdout(line)
			// The name in Cronitor wins. Keep an existing "# Name:" comment
			// in step with it; do not add one.
			commentUpdated := false
			if found && existing.Name != "" && line.Name != "" && line.Name != existing.Name {
				line.Name = existing.Name
				commentUpdated = true
			}
			planned := planUpdate(line, key, existing, found, currentName, name)
			planned.commentUpdated = commentUpdated && line.Name == existing.Name
			plan.lines = append(plan.lines, planned)
			continue
		}

		// Only clear the name if it was the auto-generated default name
		// Preserve Line.Name (from "# Name:" comments)
		if name == defaultName && line.Name == "" {
			name = ""
		}

		line.Mon = lib.Monitor{
			Name:             name,
			DefaultName:      defaultName,
			Key:              key,
			Tags:             createTags(),
			Schedules:        &[]string{line.CronExpression},
			Type:             "job",
			Platform:         lib.CRON,
			Timezone:         timezone.Name,
			Note:             createNote(line, crontab),
			Notify:           notifyList(),
			NoStdoutPassthru: noStdoutPassthru,
		}
		plan.lines = append(plan.lines, plannedLine{line: line, key: key, action: actionNew, payload: &line.Mon})
	}

	preview := *crontab
	if configureShellShim(&preview, true) == nil {
		plan.rewrites = preview.Write() != crontab.Original()
	}

	printLn()
	return plan
}

// planUpdate compares a monitored line with its monitor and returns only the
// fields that changed. Tags, alert lists and notes set in Cronitor are kept.
// When the monitor list could not be loaded (found is false), the schedule is
// sent because sync cannot compare it.
func planUpdate(line *lib.Line, key string, existing lib.Monitor, found bool, currentName, name string) plannedLine {
	payload := lib.Monitor{Key: line.Code, Type: "job"}
	changed := false

	if !found || !sameSchedule(existing.Schedules, line.CronExpression) {
		payload.Schedules = &[]string{line.CronExpression}
		payload.Timezone = timezone.Name
		changed = true
	}
	if name != "" && name != currentName {
		payload.Name = name
		line.Name = name
		changed = true
	}
	if notify := notifyList(); notify != nil {
		payload.Notify = notify
		changed = true
	}

	if !changed {
		return plannedLine{line: line, key: key, action: actionUnchanged}
	}
	return plannedLine{line: line, key: key, action: actionUpdate, payload: &payload}
}

// applyNoStdout marks a monitored line for --no-stdout. A flag already on
// the line is kept.
func applyNoStdout(line *lib.Line) {
	if noStdoutPassthru {
		line.Mon.NoStdoutPassthru = true
	}
}

func sameSchedule(schedules *[]string, expression string) bool {
	return schedules != nil && len(*schedules) == 1 && (*schedules)[0] == expression
}

// notifyList is the alert list to send, or nil to leave it to Cronitor: the
// default list for a new monitor, the current list for an existing one.
func notifyList() []string {
	if notificationList == "" {
		return nil
	}
	return []string{notificationList}
}

// syncWarning prints a warning. Without the interactive UI it goes to stderr,
// so --auto output on stdout stays a clean crontab.
func syncWarning(message string) {
	if isAutoDiscover || isSilent {
		fmt.Fprintln(os.Stderr, message)
		return
	}
	printWarningText(message, true)
}

// applyPlan sends the planned monitors to Cronitor and writes the crontab.
func applyPlan(plan *crontabPlan) bool {
	crontab := plan.crontab
	monitors := map[string]*lib.Monitor{}
	for _, p := range plan.lines {
		if p.payload != nil {
			monitors[p.key] = p.payload
		}
	}

	if len(monitors) > 0 {
		printDoneText("Sending to Cronitor", true)
		sent, err := getCronitorApi().PutMonitors(monitors)
		if err != nil {
			fatal(err.Error(), 1)
		}
		// New monitors get their code from the response. Existing lines keep
		// the key they have: it may be a hand-written key, not the code.
		for _, p := range plan.lines {
			if p.action != actionNew {
				continue
			}
			if updated, ok := sent[p.key]; ok {
				p.line.ApplyDiscoveredMonitor(*updated)
			}
		}
	}

	if err := configureShellShim(crontab, false); err != nil {
		if !isSilent {
			printErrorText(err.Error(), true)
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		return false
	}

	// Re-write crontab lines with new/updated monitoring
	updatedCrontabLines := crontab.Write()

	if !isSilent && isAutoDiscover && !processingMultipleCrontabs {
		// When running --auto mode with a single crontab, you should be able to pipe or redirect crontab output elsewhere. Skip status-related messages.
		fmt.Println(strings.TrimSpace(updatedCrontabLines))
	}

	changed := updatedCrontabLines != crontab.Original()
	if !dryRun && changed {
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

	return len(monitors) > 0 || changed
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

type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

// renderJobInfoBox renders job information in a styled box for interactive sync mode
func renderJobInfoBox(name string, schedule string, command string, monitored bool) string {
	// Define styles - use full terminal width minus margin
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		MarginLeft(2).
		Width(100)

	nameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")) // Bright white - readable on blue PowerShell backgrounds

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(12)

	yesStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")) // Green

	noStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")) // Gray

	// Build content lines
	var lines []string

	lines = append(lines, nameStyle.Render(name))
	lines = append(lines, "")

	if schedule != "" {
		lines = append(lines, labelStyle.Render("Schedule:")+"  "+schedule)
	}

	// Show full command - box width will accommodate long names/commands
	lines = append(lines, labelStyle.Render("Command:")+"  "+command)

	// Show monitoring status
	monitoredStr := noStyle.Render("No")
	if monitored {
		monitoredStr = yesStyle.Render("Yes")
	}
	lines = append(lines, labelStyle.Render("Monitored:")+"  "+monitoredStr)

	content := strings.Join(lines, "\n")
	return boxStyle.Render(content)
}

type nameInputModel struct {
	list        list.Model
	textInput   textinput.Model
	defaultName string
	err         error
	done        bool
	exited      bool   // True if user pressed Ctrl+D to exit entirely
	ignore      bool   // True if the user chose to skip the job for good
	state       string // "choosing" or "naming"
	width       int    // Add width field to store terminal width
}

func initialNameInputModel(defaultName string) nameInputModel {
	// Setup list items
	items := []list.Item{
		item{title: UseDefaultName, desc: defaultName},
		item{title: EnterCustomName, desc: "Add a friendly, unique name for this job"},
		item{title: SkipJob, desc: fmt.Sprintf("Do not monitor this %s. Sync asks again next time.", getJobLabel())},
		item{title: IgnoreJob, desc: "Adds # cronitor: ignore above the line. Delete that comment to undo."},
	}

	// Setup list with height of 3 to show all items
	l := list.New(items, list.NewDefaultDelegate(), 0, 3)
	l.Title = "What would you like to do with this job?"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().MarginLeft(1).Bold(false)
	l.Styles.TitleBar = lipgloss.NewStyle().MarginLeft(2)

	// Update the style names to match the current API
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.MarginLeft(1)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.MarginLeft(1)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.MarginLeft(1).Italic(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.MarginLeft(1).Italic(true)

	l.SetDelegate(delegate)

	// Setup text input
	ti := textinput.New()
	ti.Placeholder = "Enter monitor name"
	ti.Focus()
	ti.CharLimit = maxNameLen

	return nameInputModel{
		list:        l,
		textInput:   ti,
		defaultName: defaultName,
		state:       "choosing",
		width:       80, // Default width if we don't get window size
	}
}

func (m nameInputModel) Init() tea.Cmd {
	return nil
}

func (m nameInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			// Skip this job
			return m, tea.Quit
		case tea.KeyCtrlD:
			// Exit interactive sync entirely
			m.exited = true
			return m, tea.Quit
		case tea.KeyEnter:
			if m.state == "choosing" {
				// Handle selection based on list choice
				switch m.list.SelectedItem().(item).title {
				case EnterCustomName:
					m.state = "naming"
					m.textInput.SetValue("")
					return m, textinput.Blink
				case UseDefaultName:
					m.textInput.SetValue(m.defaultName)
					m.done = true
					return m, tea.Quit
				case SkipJob:
					m.done = false
					return m, tea.Quit
				case IgnoreJob:
					m.ignore = true
					return m, tea.Quit
				}
			} else if m.state == "naming" {
				if err := validateName(m.textInput.Value()); err != nil {
					m.err = err
					return m, nil
				}
				m.done = true

				// Empty line for more legibile output
				printLn()

				return m, tea.Quit
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.textInput.Width = msg.Width - 8 // Subtract some padding for the margin
		m.list.SetWidth(msg.Width)        // Fixed: Use SetWidth method instead of direct assignment
	}

	if m.state == "choosing" {
		m.list, cmd = m.list.Update(msg)
	} else {
		m.textInput, cmd = m.textInput.Update(msg)
	}
	return m, cmd
}

func (m nameInputModel) View() string {
	if m.state == "choosing" {
		return "\n" + m.list.View()
	}

	var sb strings.Builder
	sb.WriteString("\n    " + m.textInput.View() + "\n")
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("    Error: %s\n", m.err))
	}
	return sb.String()
}

const (
	UseDefaultName  = "Monitor this job - Use this name:"
	EnterCustomName = "Monitor this job - Change the name"
	SkipJob         = "Skip this job"
	IgnoreJob       = "Skip this job, don't ask again"
)

func init() {
	RootCmd.AddCommand(discoverCmd)

	discoverCmd.Flags().BoolVar(&saveCrontabFile, "save", saveCrontabFile, "Save the updated crontab file")
	discoverCmd.Flags().BoolVar(&dryRun, "dry-run", dryRun, "Show what sync would change. Sends nothing to Cronitor and does not write the crontab.")
	discoverCmd.Flags().StringArrayVarP(&excludeFromName, "exclude-from-name", "e", excludeFromName, "Substring to exclude from auto-generated monitor name e.g. $ cronitor sync -e '> /dev/null' -e '/path/to/app'")
	discoverCmd.Flags().BoolVar(&noAutoDiscover, "no-auto-discover", noAutoDiscover, "Do not attach an automatic discover job to this crontab, or remove if already attached.")
	discoverCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes.")
	discoverCmd.Flags().StringVar(&notificationList, "notification-list", notificationList, "Use the provided notification list when creating or updating monitors, or \"default\" list if omitted.")
	discoverCmd.Flags().BoolVar(&isAutoDiscover, "auto", isAutoDiscover, "Do not use an interactive shell. Write updated crontab to stdout.")
	discoverCmd.Flags().StringVar(&syncFormat, "format", "", "Line format on Unix. short: write every line as CRONITOR=<key> <command>, converting cronitor exec lines (lines with other exec flags stay as exec). exec: write every line as cronitor exec <key> <command> and restore the prior SHELL. Default: new lines short, existing lines unchanged.")
	discoverCmd.Flags().BoolVar(&withMonitored, "with-monitored", false, "Also ask about jobs that are already monitored, for example to rename them. Ignored with --auto.")
	discoverCmd.Flags().StringVar(&syncFile, "file", "", "Path to YAML or JSON file containing monitor definitions for bulk import")

	discoverCmd.Flags().BoolVar(&isSilent, "silent", isSilent, "")
	discoverCmd.Flags().MarkHidden("silent")

	// Since 23.0 save is deprecated
	discoverCmd.Flags().MarkDeprecated("save", "save will now happen automatically when the --dry-run flag is not used")
	discoverCmd.Flags().MarkHidden("save")

	// Since 24.0 no auto discover is deprecated
	discoverCmd.Flags().MarkDeprecated("no-auto-discover", "the auto-discover feature has been removed")
	discoverCmd.Flags().MarkHidden("no-auto-discover")

}
