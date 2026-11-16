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
	"golang.org/x/term"
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

// stdoutIsCrontab is true while `cronitor sync --auto` handles one crontab
// file and will print it on stdout. Verbose logs go to stderr so the pipe
// cannot install log text or a partial crontab.
var stdoutIsCrontab bool

// reserveStdoutForCrontab reports whether this sync should keep stdout for
// the crontab. Flags are already parsed when it runs (including from
// initConfig, before the config file is logged).
func reserveStdoutForCrontab(args []string) bool {
	if runtime.GOOS == "windows" || !isAutoDiscover || isSilent {
		return false
	}
	return len(args) > 0 && !isPathToDirectory(args[0])
}

// printOriginalCrontab prints the crontab for a --auto run on one readable
// file. Sync calls it when it fails before a plan exists. A missing or
// unreadable file prints nothing.
func printOriginalCrontab(args []string) {
	if !reserveStdoutForCrontab(args) {
		return
	}
	info, err := os.Stat(args[0])
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		return
	}
	fmt.Println(text)
}

// redactAPIKeyArgs hides API and ping keys passed on the command line so
// the dry-run reminder does not print them. Forms: -k X, -kX, -k=X,
// --api-key X, --api-key=X, a short cluster such as -vkX or -vk=X, and the
// same forms for -p / --ping-api-key (placeholder <ping-api-key>).
func redactAPIKeyArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if i == 0 {
			out = append(out, a)
			continue
		}
		redacted, next := redactAPIKeyToken(a)
		out = append(out, redacted)
		if next != "" && i+1 < len(args) {
			i++
			out = append(out, next)
		}
	}
	return out
}

// shortFlagTakesValue reports shorthands on sync whose value consumes the
// rest of a combined short-flag cluster. k and p are handled by the caller.
func shortFlagTakesValue(letter byte) bool {
	switch letter {
	case 'c', 'e', 'l', 'n', 'u':
		return true
	default:
		return false
	}
}

// redactAPIKeyToken redacts one argv element. next is the placeholder for
// the following argument, or "" when this token already holds the value.
func redactAPIKeyToken(token string) (string, string) {
	switch {
	case token == "--api-key" || token == "-k":
		return token, "<api-key>"
	case token == "--ping-api-key" || token == "-p":
		return token, "<ping-api-key>"
	case strings.HasPrefix(token, "--api-key="):
		return "--api-key=<api-key>", ""
	case strings.HasPrefix(token, "--ping-api-key="):
		return "--ping-api-key=<ping-api-key>", ""
	case strings.HasPrefix(token, "-k="):
		return "-k=<api-key>", ""
	case strings.HasPrefix(token, "-p="):
		return "-p=<ping-api-key>", ""
	}
	if !strings.HasPrefix(token, "-") || strings.HasPrefix(token, "--") || len(token) < 2 {
		return token, ""
	}
	body := token[1:]
	for i := 0; i < len(body); i++ {
		placeholder := ""
		switch body[i] {
		case 'k':
			placeholder = "<api-key>"
		case 'p':
			placeholder = "<ping-api-key>"
		default:
			if shortFlagTakesValue(body[i]) {
				return token, ""
			}
			continue
		}
		if i+1 >= len(body) {
			return token, placeholder
		}
		// Keep the '=' in a cluster such as -vk=X.
		end := i + 2
		if body[i+1] == '=' {
			end++
		}
		return token[:end] + placeholder, ""
	}
	return token, ""
}

var syncFile string    // Path to YAML/JSON file for bulk monitor import
var syncFormat string  // --format: "", "short" or "exec"
var withMonitored bool // --with-monitored: also ask about jobs that are already monitored

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

There are three ways to run sync:

  $ cronitor sync
      > Asks about each job that is not monitored yet
      > Checks every monitored job against Cronitor and picks up schedule and timezone changes
      > Shows one summary with each change, then asks once before it sends or writes anything
      > Run it again whenever you add jobs or change a schedule. If nothing changed, it asks nothing.

  $ cronitor sync --with-monitored
      > The same, but also asks about jobs that are already monitored, for example to rename them

  $ cronitor sync --auto
      > Asks nothing. Monitors every new job, sends schedule and timezone changes, and writes the crontab
      > Prints the crontab to stdout even when nothing changed. Once sync has read the file,
        a failure prints the original crontab and exits 1. A direct "| crontab -" pipe can
        still install an empty crontab if sync fails before it can read the file, for example
        when the file is missing or a flag is invalid. Capture the output first:
        out=$(cronitor sync --auto file) && printf '%s\n' "$out" | crontab -
        Do not add --silent to that command: --auto --silent prints no crontab,
        so the pipe would install an empty one.
      > Use it to automate sync, not to pick up one schedule change: it also monitors every new job

  For a job that is already monitored, sync sends only what changed. Names, tags and alert lists set
  in Cronitor are kept. Alert lists change only with --notification-list.

  In the job menu: Enter selects, s skips, and q, Esc, Ctrl+C or Ctrl+D quits. While typing a name,
  s and q type normally, Esc goes back, and Ctrl+C or Ctrl+D quits. If you quit with pending changes,
  sync shows the summary and asks whether to save. "Skip this job, don't ask again" adds
  "# cronitor: ignore" above the line; a monitored job keeps reporting to Cronitor.

  $ cronitor sync /path/to/crontab
      > Instead of the user crontab, provide a crontab file (or directory of crontabs) to use

Example excluding secrets or common text from monitor names:
  $ cronitor sync /path/to/crontab -e "secret-token" -e "/var/common/app/path/"
      > Creates new monitors, excluding the provided snippets from the generated name.
      > Existing monitored names are kept. Use --with-monitored to rename them.
      > Adds Cronitor integration to your crontab and outputs to stdout

  You can run the command as many times as you need, accumulating exclusion params until the job names on your Cronitor dashboard are clear and readable.

Example where you perform a dry-run without any crontab modifications:
  $ cronitor sync /path/to/crontab --dry-run
      > Asks about new jobs and shows what would change
      > On Linux and macOS crontab sync, sends nothing to Cronitor and does not write the crontab
      > Does not apply to --file imports or Windows scheduled-task sync

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

		// Reset the flags for each command execution
		importedCrontabs = 0
		processingMultipleCrontabs = false
		userAbortedSync = false
		jobsNeedingSudo = 0
		stdoutIsCrontab = false

		if len(viper.GetString(varApiKey)) < 10 {
			// Fail before a plan exists. When this is --auto on one readable
			// file, stdout still gets that crontab so a pipe can reinstall it.
			printOriginalCrontab(args)
			fatal(discoverMissingAPIKeyGuidance(), 1)
		}
		if _, err := parseSyncFormat(syncFormat); err != nil {
			printOriginalCrontab(args)
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

		// initConfig may already have set this so the config log stays off
		// stdout. Set it again from the args Run received, and clear it
		// when the command returns.
		if reserveStdoutForCrontab(args) {
			stdoutIsCrontab = true
			defer func() { stdoutIsCrontab = false }()
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
			msg := fmt.Sprintf("Could not load your monitors from Cronitor: %v. Sync cannot tell which jobs changed, so it sends each monitored job's schedule.", err)
			if !isAutoDiscover {
				msg += " It asks about every job."
			}
			syncWarning(msg)
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
			if hint := sudoHint(jobsNeedingSudo); hint != "" {
				syncWarning(hint)
			}
		}

		if userAbortedSync {
			printWarningText("Sync stopped", false)
		} else {
			printDoneText("Sync complete", false)
			printSuccessText("View your dashboard https://cronitor.io/app/dashboard", false)
		}
		if dryRun {
			saveCommand := strings.Join(redactAPIKeyArgs(os.Args), " ")
			saveCommand = strings.Replace(saveCommand, " --dry-run", "", -1)

			if importedCrontabs > 0 {
				msg := "Reminder: This is a DRY-RUN. Integration is not complete."
				next := "To complete integration, run:"
				// Same command without --dry-run. That is `cronitor sync`
				// after an interactive dry run, or `cronitor sync --auto`
				// when --auto was already set. --auto --silent is neither:
				// --auto would monitor every new job, and with --silent it
				// also skips printing the crontab.
				cmd := "      " + saveCommand
				if isAutoDiscover || isSilent {
					fmt.Fprintln(os.Stderr, msg)
					fmt.Fprintln(os.Stderr, next)
					fmt.Fprintln(os.Stderr, cmd)
				} else {
					printWarningText(msg, true)
					printWarningText(next, true)
					fmt.Println(cmd + "\n")
				}
			}
		}
	},
}

// crontabWritable reports whether sync can write a crontab. Tests replace it.
var crontabWritable = func(c *lib.Crontab) bool { return c.IsWritable() }

// crontabFileReadable reports whether a file crontab can be opened. A user
// crontab is not a path. Callers use this after Parse fails, to tell an
// unreadable file from an empty one.
func crontabFileReadable(c *lib.Crontab) bool {
	f, err := os.Open(c.Filename)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// jobsNeedingSudo counts jobs in crontabs this user cannot write.
var jobsNeedingSudo int

// countJobs counts the jobs sync would look at in a parsed crontab.
func countJobs(c *lib.Crontab) int {
	n := 0
	for _, l := range c.Lines {
		if l.IsMonitorable() && !l.Ignored && !l.IsComment && !l.IsAutoDiscoverCommand() {
			n++
		}
	}
	return n
}

// sudoHint is the one hint about crontabs sync could not write, or "".
func sudoHint(jobs int) string {
	switch jobs {
	case 0:
		return ""
	case 1:
		return "1 job in system crontabs was not checked. Run sync with sudo to include it."
	}
	return fmt.Sprintf("%d jobs in system crontabs were not checked. Run sync with sudo to include them.", jobs)
}

// collectDirectory plans every crontab in a directory.
func collectDirectory(plans *[]*crontabPlan, username, directory string) {
	for _, crontabFile := range lib.EnumerateFiles(directory) {
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

// printPlanSummaries writes one summary per crontab. --auto --dry-run sends
// it to stderr so stdout stays a clean crontab.
func printPlanSummaries(plans []*crontabPlan, toStderr bool) {
	w := os.Stdout
	if toStderr {
		w = os.Stderr
	}
	fmt.Fprintln(w)
	for _, p := range plans {
		fmt.Fprintln(w, "  "+p.crontab.DisplayName())
		fmt.Fprintln(w, "    "+p.summary())
		for _, l := range p.lines {
			if l.action == actionUpdate {
				fmt.Fprintf(w, "    L%d  %s: %s\n", l.line.LineNumber, l.label, strings.Join(l.changes, "; "))
			}
		}
	}
	fmt.Fprintln(w)
}

// finishSync shows one summary for all planned crontabs, asks once, then
// applies the plans. It returns the number of crontabs that changed.
func finishSync(plans []*crontabPlan) int {
	// When running --auto mode with a single crontab, you should be able to pipe or redirect crontab
	// output elsewhere, so print it whether or not anything changed. Print first, then
	// exit, so a failed apply still writes the original crontab before exitFn.
	// applyPlan must return on failure. os.Exit skips this print.
	// A missing or unwritable file is the same kind of failure: exit 1, and
	// print the original only when there is one to print.
	applyFailed := false
	unusable := false
	for _, p := range plans {
		if p.unusable {
			unusable = true
			applyFailed = true
		}
	}
	defer func() {
		if !isSilent && isAutoDiscover && !processingMultipleCrontabs {
			for _, p := range plans {
				// A missing file has no crontab. A blank line would still
				// be installed by `... | crontab -`.
				if p.unusable && strings.TrimSpace(p.output) == "" {
					continue
				}
				fmt.Println(strings.TrimSpace(p.output))
			}
		}
		if applyFailed {
			exitFn(1)
		}
	}()
	if unusable {
		return 0
	}
	var todo []*crontabPlan
	for _, p := range plans {
		if p.hasChanges() {
			todo = append(todo, p)
		}
	}
	// --auto skips questions. --silent only hides status text; without --auto
	// it still confirms before applying, so a schedule change cannot land
	// with no prompt.
	ask := !isAutoDiscover

	quit := userAbortedSync
	if len(todo) == 0 {
		if quit {
			printWarningText("Nothing was changed", false)
		} else if ask {
			printDoneText("Nothing to change", false)
		} else if dryRun && !isSilent {
			fmt.Fprintln(os.Stderr, "Nothing to change")
		}
		return 0
	}

	if ask || (dryRun && isAutoDiscover && !isSilent) {
		printPlanSummaries(plans, isAutoDiscover)
	}
	if dryRun {
		msg := "Dry run: nothing was sent to Cronitor or written to the crontab"
		if isAutoDiscover || isSilent {
			fmt.Fprintln(os.Stderr, msg)
		} else {
			printWarningText(msg, false)
		}
		return len(todo)
	}
	if ask {
		// After a quit, the summary shows the answers given so far, and the
		// question defaults to no.
		question, defaultYes := "Apply these changes?", true
		if quit {
			question, defaultYes = "Save what you've done so far?", false
		}
		if !confirm(question, defaultYes) {
			printWarningText("Nothing was changed", false)
			return 0
		}
	}

	changed := 0
	for _, p := range todo {
		if applyPlan(p) {
			changed++
		} else {
			applyFailed = true
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
	label          string   // the job's name, for the summary
	changes        []string // what actionUpdate changes, for the summary
}

// crontabPlan holds every decision for one crontab. planCrontab fills it
// without calling the API or writing anything; applyPlan carries it out.
type crontabPlan struct {
	crontab  *lib.Crontab
	lines    []plannedLine
	aborted  bool   // the user quit
	rewrites bool   // the crontab text changes even if no monitor does
	output   string // the crontab text --auto prints: as planned, or as written
	unusable bool   // missing or not writable: do not apply; finishSync exits 1
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
	fmt.Println(renderJobInfoBox(name, line.CronExpression, line.CommandToRun, monitored, terminalWidth()))

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
// returns nil when a crontab is skipped. One named file that does not exist
// or is not writable is an unusable plan: sync prints the original crontab
// when it has one, and exits 1.
func planCrontab(crontab *lib.Crontab) *crontabPlan {
	if !crontab.Exists() {
		// One named file that is not there is a failure. Scanning many
		// crontabs still skips a user who has none.
		if !processingMultipleCrontabs {
			reportError(fmt.Sprintf("%s does not exist", crontab.DisplayName()))
			return &crontabPlan{crontab: crontab, unusable: true}
		}
		printWarningText("This crontab does not exist. Skipping.", true)
		return nil
	}

	// This will mostly happen when the crontab is empty
	if err, _ := crontab.Parse(noAutoDiscover); err != nil {
		// A named file that cannot be read is a failure. There is nothing
		// to print. An empty crontab is still a skip: installing it changes
		// nothing. Scanning many crontabs keeps the old skip.
		if !processingMultipleCrontabs && !crontab.IsUserCrontab && !crontabFileReadable(crontab) {
			reportError(err.Error())
			return &crontabPlan{crontab: crontab, unusable: true}
		}
		// While scanning, an empty or unreadable system crontab is only logged.
		if !processingMultipleCrontabs || crontab.IsUserCrontab {
			printWarningText("This crontab is empty. Skipping.", true)
		}
		log(fmt.Sprintf("Skipping %s: %s", crontab.DisplayName(), err.Error()))
		return nil
	}

	// Before going further, ensure we aren't going to run into permissions problems writing the crontab later.
	// While scanning many crontabs, count the jobs and give one hint at the end.
	if !crontabWritable(crontab) {
		if processingMultipleCrontabs {
			jobsNeedingSudo += countJobs(crontab)
			return nil
		}
		// Readable (Parse succeeded) but not writable. Same as an API, shim
		// or save failure: stdout gets the original crontab when this run
		// prints one, and the process exits 1.
		reportError(fmt.Sprintf("%s is not writable. Run sync with sudo.", crontab.DisplayName()))
		return &crontabPlan{crontab: crontab, output: crontab.Original(), unusable: true}
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
			// A generated name is not a rename. Without the monitor list,
			// currentName is empty, and sending the generated name would
			// rename the monitor and add a "# Name:" comment.
			newName := name
			if newName == defaultName && currentName == "" {
				newName = ""
			}
			planned := planUpdate(line, key, existing, found, currentName, newName)
			if planned.label == "" {
				planned.label = defaultName
			}
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
	plan.output = crontab.Original()
	if configureShellShim(&preview, true) == nil {
		plan.output = preview.Write()
		plan.rewrites = plan.output != crontab.Original()
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
	var changes []string

	if !found {
		payload.Schedules = &[]string{line.CronExpression}
		payload.Timezone = timezone.Name
		changes = append(changes, "schedule "+line.CronExpression)
	} else if !sameSchedule(existing.Schedules, line.CronExpression) {
		payload.Schedules = &[]string{line.CronExpression}
		payload.Timezone = timezone.Name
		changes = append(changes, fmt.Sprintf("schedule %s → %s", scheduleText(existing.Schedules), line.CronExpression))
	} else if existing.Timezone != "" && existing.Timezone != timezone.Name {
		// Same schedule, but the crontab's timezone (CRON_TZ, TZ or the server's) changed.
		payload.Timezone = timezone.Name
		changes = append(changes, fmt.Sprintf("timezone %s → %s", existing.Timezone, timezone.Name))
	}
	if name != "" && name != currentName {
		payload.Name = name
		line.Name = name
		changes = append(changes, fmt.Sprintf("name %q → %q", currentName, name))
	}
	if notify := notifyList(); notify != nil {
		payload.Notify = notify
		changes = append(changes, "alert list → "+strings.Join(notify, ", "))
	}

	label := name
	if label == "" {
		label = currentName
	}
	if len(changes) == 0 {
		return plannedLine{line: line, key: key, action: actionUnchanged, label: label}
	}
	return plannedLine{line: line, key: key, action: actionUpdate, payload: &payload, label: label, changes: changes}
}

func scheduleText(schedules *[]string) string {
	if schedules == nil || len(*schedules) == 0 {
		return "(none)"
	}
	return strings.Join(*schedules, ", ")
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
			// os.Exit here skips finishSync's deferred print, so
			// `cronitor sync --auto file | crontab -` installs an empty
			// crontab. Same as a shim or save failure: stdout gets the
			// original crontab (not the planned rewrite) and the process
			// exits 1.
			plan.output = crontab.Original()
			reportError(err.Error())
			return false
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
		// The file is unchanged. --auto stdout must not print the planned
		// crontab (SHELL= would point at a wrapper that was never installed).
		// Print the original so a pipe to crontab reinstalls a working
		// crontab instead of an empty one; finishSync then exits 1.
		plan.output = crontab.Original()
		if isAutoDiscover || isSilent {
			fmt.Fprintln(os.Stderr, err.Error())
		} else {
			printErrorText(err.Error(), true)
		}
		return false
	}

	// Re-write crontab lines with new/updated monitoring
	updatedCrontabLines := crontab.Write()

	changed := updatedCrontabLines != crontab.Original()
	if !dryRun && changed {
		if err := crontab.Save(updatedCrontabLines); err == nil {
			plan.output = updatedCrontabLines
			if !isSilent {
				printDoneText("Integration complete", true)
			}
		} else {
			plan.output = crontab.Original()
			if isAutoDiscover || isSilent {
				fmt.Fprintln(os.Stderr, "Problem saving crontab: "+err.Error())
			} else {
				printErrorText("Problem saving crontab: "+err.Error(), true)
			}
			return false
		}
	} else {
		plan.output = updatedCrontabLines
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

// terminalWidth is the width of the terminal on stdout, or 100 when it is
// not a terminal.
func terminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 100
}

// renderJobInfoBox renders job information in a styled box for interactive
// sync mode. The box fits a terminal of the given width, up to 100 columns;
// long values wrap inside the box.
func renderJobInfoBox(name string, schedule string, command string, monitored bool, width int) string {
	const margin, labelWidth = 2, 12
	boxWidth := width - margin - 1
	if boxWidth > 100 {
		boxWidth = 100
	}
	if boxWidth < 40 {
		boxWidth = 40
	}
	// The border takes 2 columns and the padding 2 more.
	inner := boxWidth - 4
	valueWidth := inner - labelWidth - 2

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		MarginLeft(margin).
		Width(boxWidth - 2)

	nameStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")). // Bright white - readable on blue PowerShell backgrounds
		Width(inner)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Width(labelWidth)

	valueStyle := lipgloss.NewStyle().Width(valueWidth)

	row := func(label, value string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, labelStyle.Render(label), "  ", valueStyle.Render(value))
	}

	yesStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("10")) // Green

	noStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")) // Gray

	var lines []string
	lines = append(lines, nameStyle.Render(name))
	lines = append(lines, "")
	if schedule != "" {
		lines = append(lines, row("Schedule:", schedule))
	}
	lines = append(lines, row("Command:", command))

	monitoredStr := noStyle.Render("No")
	if monitored {
		monitoredStr = yesStyle.Render("Yes")
	}
	lines = append(lines, row("Monitored:", monitoredStr))

	return boxStyle.Render(strings.Join(lines, "\n"))
}

type nameInputModel struct {
	list        list.Model
	textInput   textinput.Model
	defaultName string
	err         error
	done        bool
	exited      bool // True if the user quit (q, esc, ctrl+c or ctrl+d)
	ignore      bool // True if the user chose to skip the job for good
	// windows keeps the old keys for the Windows task flow, which has no save
	// prompt and cannot write "# cronitor: ignore": q, esc and ctrl+c skip one
	// task, ctrl+d stops.
	windows bool
	state   string // "choosing" or "naming"
	width   int    // Add width field to store terminal width
}

func initialNameInputModel(defaultName string) nameInputModel {
	return newNameInputModel(defaultName, false)
}

// initialWindowsNameInputModel is the menu for Windows scheduled tasks.
func initialWindowsNameInputModel(defaultName string) nameInputModel {
	return newNameInputModel(defaultName, true)
}

func newNameInputModel(defaultName string, windows bool) nameInputModel {
	// Setup list items
	items := []list.Item{
		item{title: UseDefaultName, desc: defaultName},
		item{title: EnterCustomName, desc: "Add a friendly, unique name for this job"},
		item{title: SkipJob, desc: fmt.Sprintf("Do not monitor this %s. Sync asks again next time.", getJobLabel())},
	}
	if !windows {
		items = append(items, item{title: IgnoreJob, desc: "Adds # cronitor: ignore above the line. A monitored job keeps reporting. Delete the comment to undo."})
	}

	// Each item is two lines (title and description) with no spacing, plus
	// two lines for the title bar, so every item shows at once.
	l := list.New(items, list.NewDefaultDelegate(), 0, len(items)*2+2)
	l.Title = "What would you like to do with this job?"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false) // the footer in View lists the keys
	// The list's own keys (q and esc quit, ? help, / filter) are off. Update
	// handles enter, s, q, esc, ctrl+c and ctrl+d itself.
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)
	l.KeyMap.ShowFullHelp.SetEnabled(false)
	l.KeyMap.CloseFullHelp.SetEnabled(false)
	l.KeyMap.Filter.SetEnabled(false)
	l.Styles.Title = lipgloss.NewStyle().MarginLeft(1).Bold(false)
	l.Styles.TitleBar = lipgloss.NewStyle().MarginLeft(2)

	// Update the style names to match the current API
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.MarginLeft(1)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.MarginLeft(1)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.MarginLeft(1).Italic(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.MarginLeft(1).Italic(true)
	delegate.SetSpacing(0)

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
		windows:     windows,
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
		case tea.KeyCtrlC, tea.KeyCtrlD:
			// Stop asking. Sync then offers to save the answers so far.
			// On Windows, ctrl+c skips the task, as before.
			m.exited = !(m.windows && msg.Type == tea.KeyCtrlC)
			return m, tea.Quit
		case tea.KeyEsc:
			if m.state == "naming" {
				m.state = "choosing"
				m.err = nil
				return m, nil
			}
			m.exited = !m.windows
			return m, tea.Quit
		case tea.KeyRunes:
			if m.state == "choosing" {
				switch string(msg.Runes) {
				case "s":
					m.done = false
					return m, tea.Quit
				case "q":
					m.exited = !m.windows
					return m, tea.Quit
				}
			}
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

var promptFooterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).MarginLeft(4)

func (m nameInputModel) View() string {
	if m.state == "choosing" {
		footer := "enter select · s skip · q quit"
		if m.windows {
			footer = "enter select · s skip · q skip · ctrl+d stop"
		}
		return "\n" + m.list.View() + "\n" + promptFooterStyle.Render(footer) + "\n"
	}

	var sb strings.Builder
	sb.WriteString("\n    " + m.textInput.View() + "\n")
	if m.err != nil {
		sb.WriteString(fmt.Sprintf("    Error: %s\n", m.err))
	}
	namingFooter := "enter save · esc back · ctrl+c quit · ctrl+d quit"
	if m.windows {
		namingFooter = "enter save · esc back · ctrl+c skip · ctrl+d stop"
	}
	sb.WriteString(promptFooterStyle.Render(namingFooter) + "\n")
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
	discoverCmd.Flags().BoolVar(&dryRun, "dry-run", dryRun, "Show what Linux/macOS crontab sync would change. Sends nothing to Cronitor and does not write the crontab. Does not apply to --file or Windows scheduled-task sync.")
	discoverCmd.Flags().StringArrayVarP(&excludeFromName, "exclude-from-name", "e", excludeFromName, "Substring to exclude from auto-generated monitor name e.g. $ cronitor sync -e '> /dev/null' -e '/path/to/app'")
	discoverCmd.Flags().BoolVar(&noAutoDiscover, "no-auto-discover", noAutoDiscover, "Do not attach an automatic discover job to this crontab, or remove if already attached.")
	discoverCmd.Flags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes.")
	discoverCmd.Flags().StringVar(&notificationList, "notification-list", notificationList, "Set this alert list on new and existing monitors. Without it, sync sends no alert list: new monitors get Cronitor's default list and existing monitors keep theirs.")
	discoverCmd.Flags().BoolVar(&isAutoDiscover, "auto", isAutoDiscover, "Ask nothing: monitor every new job, send schedule and timezone changes, write the crontab, and print it to stdout even when nothing changed.")
	discoverCmd.Flags().StringVar(&syncFormat, "format", "", "Line format on Unix. short: write every line as CRONITOR=<key> <command>, converting cronitor exec lines (lines with other exec flags stay as exec). exec: write every line as cronitor exec <key> <command> and restore the prior SHELL. Default: new lines short, existing lines unchanged.")
	discoverCmd.Flags().BoolVar(&withMonitored, "with-monitored", false, "Also ask about jobs that are already monitored, for example to rename them. Ignored with --auto.")
	discoverCmd.Flags().StringVar(&syncFile, "file", "", "Path to YAML or JSON file containing monitor definitions for bulk import")

	discoverCmd.Flags().BoolVar(&isSilent, "silent", isSilent, "Hide status messages. Does not skip the Apply prompt unless --auto is also set. With --auto, also skip printing the crontab.")
	discoverCmd.Flags().MarkHidden("silent")

	// Since 23.0 save is deprecated
	discoverCmd.Flags().MarkDeprecated("save", "save will now happen automatically when the --dry-run flag is not used")
	discoverCmd.Flags().MarkHidden("save")

	// Since 24.0 no auto discover is deprecated
	discoverCmd.Flags().MarkDeprecated("no-auto-discover", "the auto-discover feature has been removed")
	discoverCmd.Flags().MarkHidden("no-auto-discover")

}
