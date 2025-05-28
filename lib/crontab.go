package lib

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const DROP_IN_DIRECTORY = "/etc/cron.d"
const SYSTEM_CRONTAB = "/etc/crontab"

type TimezoneLocationName struct {
	Name string
}

type Crontab struct {
	User                    string                `json:"-"`
	IsUserCrontab           bool                  `json:"isUserCrontab"`
	IsSaved                 bool                  `json:"-"`
	Filename                string                `json:"filename"`
	Lines                   []*Line               `json:"-"`
	TimezoneLocationName    *TimezoneLocationName `json:"timezone,omitempty"`
	UsesSixFieldExpressions bool                  `json:"-"`
}

func (c *Crontab) Parse(noAutoDiscover bool) (error, int) {
	lines, errCode, err := c.load()
	if err != nil {
		return err, errCode
	}

	if len(c.Lines) > 0 {
		panic("Cannot read into non-empty crontab struct")
	}

	var autoDiscoverLine *Line
	var name string

	for lineNumber, fullLine := range lines {
		var cronExpression string
		var command []string
		var runAs string

		fullLine = strings.TrimSpace(fullLine)
		isComment := false

		// Check for special Name: comment, handle other comments normally
		if nameMatch := regexp.MustCompile(`^#\s*Name:\s*(.+)$`).FindStringSubmatch(fullLine); nameMatch != nil {
			name = strings.TrimSpace(nameMatch[1])
			continue
		}

		// If the line is a comment, see if it is a disabled job
		if strings.HasPrefix(fullLine, "#") {
			// Remove the # and trim whitespace before splitting
			fullLine = strings.TrimSpace(strings.TrimPrefix(fullLine, "#"))
			isComment = true
		}

		splitLine := strings.Fields(fullLine)
		splitLineLen := len(splitLine)
		if splitLineLen == 1 && strings.Contains(splitLine[0], "=") {
			// Handling for environment variables... we're looking for timezone declarations
			if splitExport := strings.Split(splitLine[0], "="); splitExport[0] == "TZ" || splitExport[0] == "CRON_TZ" {
				c.TimezoneLocationName = &TimezoneLocationName{splitExport[1]}
			}
		} else if splitLineLen > 0 && strings.HasPrefix(splitLine[0], "@") {
			// Handling for special cron @keyword
			cronExpression = splitLine[0]
			command = splitLine[1:]
		} else if splitLineLen >= 6 {
			// Handling for javacron-style 6 item cron expressions
			c.UsesSixFieldExpressions = splitLineLen >= 7 && isSixFieldCronExpression(splitLine)

			if c.UsesSixFieldExpressions {
				cronExpression = strings.Join(splitLine[0:6], " ")
				command = splitLine[6:]
			} else {
				cronExpression = strings.Join(splitLine[0:5], " ")
				command = splitLine[5:]
			}
		}

		// Try to determine if the command begins with a "run as" user designation. This is required for system-level crontabs.
		// Basically, just see if the first word of the command is a valid user name. This is how vixie cron does it.
		// https://github.com/rhuitl/uClinux/blob/master/user/vixie-cron/entry.c#L224
		if runtime.GOOS != "windows" && len(command) > 1 && c.IsRoot() {
			idOrError, _ := exec.Command("id", "-u", command[0]).CombinedOutput()
			if _, err := strconv.Atoi(strings.TrimSpace(string(idOrError))); err == nil {
				runAs = command[0]
				command = command[1:]
			}
		}

		// Create a Line struct with details for this line so we can re-create it later
		line := Line{
			IsComment:      isComment,
			IsJob:          len(command) > 0 && len(cronExpression) > 0,
			Name:           name,
			CronExpression: cronExpression,
			FullLine:       fullLine,
			LineNumber:     lineNumber,
			RunAs:          runAs,
			Crontab:        *c,
		}

		// If this job is already being wrapped by the Cronitor client, read current code.
		// Expects a wrapped command to look like: cronitor exec d3x0 /path/to/cmd.sh
		if len(command) > 1 && strings.HasSuffix(command[0], "cronitor") && command[1] == "exec" {
			line.Code = command[2]
			command = command[3:]
		}

		line.CommandToRun = strings.Join(command, " ")

		// If the command appears to be quoted (starts and ends with quotes), unquote it
		if strings.HasPrefix(line.CommandToRun, "\"") && strings.HasSuffix(line.CommandToRun, "\"") {
			line.CommandToRun = strings.Trim(line.CommandToRun, "\"")
			line.CommandToRun = strings.Replace(line.CommandToRun, "\\\"", "\"", -1)
		}

		if line.IsAutoDiscoverCommand() {
			autoDiscoverLine = &line
			if noAutoDiscover {
				continue // remove the auto-discover line from the crontab if --no-auto-discover flag is passed
			}
		}

		// Reset the name for the next line after we've found its command line
		if line.CronExpression != "" {
			name = ""
		}

		c.Lines = append(c.Lines, &line)
	}

	// If we do not have an auto-discover line but we should, add one now
	if autoDiscoverLine == nil && !noAutoDiscover {
		c.Lines = append(c.Lines, createAutoDiscoverLine(c))
	}

	return nil, 0
}

func (c Crontab) Write() string {
	var cl []string
	for _, line := range c.Lines {
		cl = append(cl, line.Write())
	}

	return strings.Join(cl, "\n")
}

func (c Crontab) Save(crontabLines string) error {
	if c.IsUserCrontab {
		cmd := exec.Command("crontab", "-")

		// crontab will use whatever $EDITOR is set. Temporarily just cat it out.
		cmd.Env = []string{"EDITOR=/bin/cat"}
		cmdStdin, _ := cmd.StdinPipe()
		cmdStdin.Write([]byte(crontabLines))
		cmdStdin.Close()
		if output, err := cmd.CombinedOutput(); err != nil {
			return errors.New("cannot write user crontab: " + err.Error() + " " + string(output))
		}
	} else {
		if ioutil.WriteFile(c.Filename, []byte(crontabLines), 0644) != nil {
			return errors.New(fmt.Sprintf("cannot write crontab at %s; check permissions and try again", c.Filename))
		}
	}

	c.IsSaved = true
	return nil
}

func (c Crontab) DisplayName() string {
	if c.IsUserCrontab {
		if strings.HasPrefix(c.Filename, "user:") {
			username := strings.TrimPrefix(c.Filename, "user:")
			return fmt.Sprintf("user \"%s\" crontab", username)
		}
		return "user crontab"
	}

	return c.Filename
}

func (c Crontab) CanonicalName() string {
	if c.IsUserCrontab {
		return c.DisplayName()
	}

	if absoluteCronPath, err := filepath.Abs(c.Filename); err == nil {
		return absoluteCronPath
	}

	return c.DisplayName()
}

func (c Crontab) IsWritable() bool {
	if c.IsUserCrontab {
		return true
	}

	file, err := os.OpenFile(c.Filename, os.O_WRONLY, 0666)
	defer file.Close()
	if err != nil {
		return false
	}
	return true
}

func (c Crontab) IsRoot() bool {
	return !c.IsUserCrontab || c.User == "root"
}

func (c Crontab) Exists() bool {

	if c.Filename != "" {
		if _, err := os.Stat(c.Filename); os.IsNotExist(err) {
			return false
		}
	} else {
		cmd := exec.Command("crontab", "-l")
		if _, err := cmd.CombinedOutput(); err != nil {
			return false
		}
	}

	return true
}

func (c Crontab) load() ([]string, int, error) {

	var crontabBytes []byte

	if c.IsUserCrontab {
		if runtime.GOOS == "windows" {
			return nil, 126, errors.New("on Windows, a crontab path argument is required")
		}

		cmd := exec.Command("crontab", "-l")
		if b, err := cmd.CombinedOutput(); err == nil {
			crontabBytes = b
		} else {
			if strings.Contains(string(b), "no crontab") {
				return nil, 126, errors.New("no crontab for this user")
			} else {
				return nil, 126, errors.New("user crontab couldn't be read")
			}
		}
	} else {
		if _, err := os.Stat(c.Filename); os.IsNotExist(err) {
			return nil, 66, errors.New(fmt.Sprintf("the file %s does not exist", c.Filename))
		}

		if b, err := ioutil.ReadFile(c.Filename); err == nil {
			crontabBytes = b
		} else {
			return nil, 126, errors.New(fmt.Sprintf("the crontab file at %s could not be read; check permissions and try again", c.Filename))
		}
	}

	if len(crontabBytes) == 0 {
		return nil, 126, errors.New("the crontab file is empty")
	}

	return strings.Split(string(crontabBytes), "\n"), 0, nil
}

// MarshalJSON implements custom JSON marshaling to include both Filename and DisplayName
func (c Crontab) MarshalJSON() ([]byte, error) {
	type Alias Crontab
	timezone := ""
	if c.TimezoneLocationName != nil {
		timezone = c.TimezoneLocationName.Name
	}
	return json.Marshal(&struct {
		Alias
		DisplayName string  `json:"display_name"`
		Timezone    string  `json:"timezone,omitempty"`
		Lines       []*Line `json:"lines"`
	}{
		Alias:       Alias(c),
		DisplayName: c.DisplayName(),
		Timezone:    timezone,
		Lines:       c.Lines,
	})
}

type Line struct {
	IsComment      bool
	IsJob          bool
	Name           string
	FullLine       string
	LineNumber     int
	CronExpression string
	CommandToRun   string
	Code           string
	RunAs          string
	Mon            Monitor
	Crontab        Crontab
}

func (l Line) IsMonitorable() bool {
	// Users don't want to see "plumbing" cron jobs on their dashboard...
	return l.IsJob && !l.IsMetaCronJob() && !l.HasLegacyIntegration()
}

func (l Line) IsAutoDiscoverCommand() bool {
	matched, _ := regexp.MatchString(".+discover[[:space:]]+--auto.*", strings.ToLower(l.CommandToRun))
	return matched
}

func (l Line) HasLegacyIntegration() bool {
	return strings.Contains(l.CommandToRun, "cronitor.io") || strings.Contains(l.CommandToRun, "cronitor.link")
}

func (l Line) IsMetaCronJob() bool {
	return strings.Contains(l.CommandToRun, "cron.hourly") || strings.Contains(l.CommandToRun, "cron.daily") || strings.Contains(l.CommandToRun, "cron.weekly") || strings.Contains(l.CommandToRun, "cron.monthly")
}

func (l Line) CommandIsComplex() bool {
	return strings.Contains(l.CommandToRun, ";") || strings.Contains(l.CommandToRun, "|") || strings.Contains(l.CommandToRun, "&&") || strings.Contains(l.CommandToRun, "||")
}

func (l Line) Write() string {
	var outputLines []string
	var lineParts []string

	// Add the name comment if present
	if len(l.Name) > 0 {
		outputLines = append(outputLines, fmt.Sprintf("# Name: %s", l.Name))
	}

	if !l.IsMonitorable() {
		lineParts = append(lineParts, l.FullLine)
	} else {
		// If this line is marked as a comment, ensure it is commented out in the crontab
		if l.IsComment {
			lineParts = append(lineParts, "#")
		}

		lineParts = append(lineParts, l.CronExpression)

		if !l.Crontab.IsUserCrontab {
			lineParts = append(lineParts, l.RunAs)
		}

		if code := l.GetCode(); code != "" {
			lineParts = append(lineParts, "cronitor")
			if l.Mon.NoStdoutPassthru {
				lineParts = append(lineParts, "--no-stdout")
			}
			lineParts = append(lineParts, "exec")
			lineParts = append(lineParts, code)

			if len(l.CommandToRun) > 0 {
				if l.CommandIsComplex() {
					lineParts = append(lineParts, "\""+strings.Replace(l.CommandToRun, "\"", "\\\"", -1)+"\"")
				} else {
					lineParts = append(lineParts, l.CommandToRun)
				}
			}
		} else {
			lineParts = append(lineParts, l.CommandToRun)
		}
	}

	outputLines = append(outputLines, strings.TrimSpace(strings.Replace(strings.Join(lineParts, " "), "  ", " ", -1)))
	return strings.Join(outputLines, "\n")
}

func (l Line) Key(CanonicalPath string) string {
	var CommandToRun, RunAs, CronExpression string
	if l.IsAutoDiscoverCommand() {
		// Go out of our way to prevent making a duplicate monitor for an auto-discovery command.
		CommandToRun = "auto discover " + CanonicalPath
		RunAs = ""
		CronExpression = ""
	} else {
		CommandToRun = l.CommandToRun
		RunAs = l.RunAs
		CronExpression = l.CronExpression
	}

	// Always use os.Hostname when creating a key so the key does not change when a user modifies their hostname using param/var
	hostname, _ := os.Hostname()
	data := []byte(fmt.Sprintf("%s-%s-%s-%s", hostname, CommandToRun, CronExpression, RunAs))
	return fmt.Sprintf("%x", sha1.Sum(data))
}

func (l Line) GetCode() string {
	// Existing integrations will have a code already in the Line struct
	if l.Code != "" {
		return l.Code
	}

	// New integrations will get it from the Monitor struct
	if l.Mon.Code != "" {
		return l.Mon.Code
	}

	return ""
}

// MarshalJSON implements custom JSON marshaling to expose all necessary fields
func (l Line) MarshalJSON() ([]byte, error) {
	type Alias Line

	// Base structure that all lines will have
	base := struct {
		Alias
		IsJob           bool   `json:"is_job"`
		IsComment       bool   `json:"is_comment"`
		IsEnvVar        bool   `json:"is_env_var"`
		Name            string `json:"name"`
		LineText        string `json:"line_text"`
		LineNumber      int    `json:"line_number"`
		CronExpression  string `json:"cron_expression"`
		CommandToRun    string `json:"command_to_run"`
		Code            string `json:"code"`
		RunAs           string `json:"run_as"`
		EnvVarKey       string `json:"env_var_key,omitempty"`
		EnvVarValue     string `json:"env_var_value,omitempty"`
		Key             string `json:"key,omitempty"`
		CrontabFilename string `json:"crontab_filename,omitempty"`
		DefaultName     string `json:"default_name,omitempty"`
	}{
		Alias:          Alias(l),
		IsJob:          l.IsJob,
		IsComment:      l.IsComment,
		IsEnvVar:       l.IsEnvVar(),
		Name:           l.Name,
		LineText:       l.FullLine,
		LineNumber:     l.LineNumber,
		CronExpression: l.CronExpression,
		CommandToRun:   l.CommandToRun,
		Code:           l.Code,
		RunAs:          l.RunAs,
		EnvVarKey:      l.GetEnvVarKey(),
		EnvVarValue:    l.GetEnvVarValue(),
	}

	// If this is a job, add additional fields
	if l.IsJob {
		base.Key = l.Key(l.Crontab.CanonicalName())
		base.CrontabFilename = l.Crontab.Filename

		// Generate default name
		defaultName := ""
		excludeFromName := []string{
			"2>&1",
			"/bin/bash -l -c",
			"/bin/bash -lc",
			"/bin/bash -c -l",
			"/bin/bash -cl",
			"/dev/null",
			"'",
			"\"",
			"\\",
		}

		// Limit the visible hostname portion to 21 chars
		hostname, _ := os.Hostname()
		formattedHostname := ""
		if hostname != "" {
			if len(hostname) > 21 {
				hostname = fmt.Sprintf("%s...%s", hostname[:9], hostname[len(hostname)-9:])
			}
			formattedHostname = fmt.Sprintf("[%s] ", hostname)
		}

		if l.IsAutoDiscoverCommand() {
			defaultName = fmt.Sprintf("%sAuto discover %s", formattedHostname, strings.TrimSpace(l.Crontab.DisplayName()))
			if len(defaultName) > 100 {
				defaultName = defaultName[:97] + "..."
			}
		} else {
			// Remove output redirection
			commandToRun := l.CommandToRun
			for _, redirectionOperator := range []string{">>", ">"} {
				if operatorPosition := strings.LastIndex(l.CommandToRun, redirectionOperator); operatorPosition > 0 {
					commandToRun = commandToRun[:operatorPosition]
					break
				}
			}

			// Remove exclusion text
			for _, substr := range excludeFromName {
				commandToRun = strings.Replace(commandToRun, substr, "", -1)
			}

			commandToRun = strings.Join(strings.Fields(commandToRun), " ")

			// Assemble the candidate name
			formattedRunAs := ""
			if l.RunAs != "" {
				formattedRunAs = fmt.Sprintf("%s ", l.RunAs)
			}

			defaultName = formattedHostname + formattedRunAs + commandToRun

			// If too long, truncate
			if len(defaultName) > 100 {
				// Keep the first and last portion of the command
				separator := "..."
				commandPrefixLen := 20 + len(formattedHostname) + len(formattedRunAs)
				lineNumSuffix := fmt.Sprintf(" L%d", l.LineNumber)
				commandSuffixLen := 100 - len(lineNumSuffix) - commandPrefixLen - len(separator)
				defaultName = fmt.Sprintf(
					"%s%s%s%s",
					strings.TrimSpace(defaultName[:commandPrefixLen]),
					separator,
					strings.TrimSpace(defaultName[len(defaultName)-commandSuffixLen:]),
					lineNumSuffix)
			}
		}

		base.DefaultName = defaultName

		// Now create a Job structure
		type Job struct {
			Key                string        `json:"key"`
			Code               string        `json:"code"`
			Name               string        `json:"name"`
			DefaultName        string        `json:"default_name"`
			Command            string        `json:"command"`
			Expression         string        `json:"expression"`
			CrontabFilename    string        `json:"crontab_filename"`
			CrontabDisplayName string        `json:"crontab_display_name"`
			LineNumber         int           `json:"line_number"`
			RunAsUser          string        `json:"run_as_user"`
			Timezone           string        `json:"timezone"`
			Monitored          bool          `json:"monitored"`
			Suspended          bool          `json:"suspended"`
			Instances          []interface{} `json:"instances"`
			IsDraft            bool          `json:"is_draft"`
		}

		timezone := "UTC"
		if l.Crontab.TimezoneLocationName != nil {
			timezone = l.Crontab.TimezoneLocationName.Name
		}

		job := Job{
			Key:                base.Key,
			Code:               l.Code,
			Name:               l.Name,
			DefaultName:        defaultName,
			Command:            l.CommandToRun,
			Expression:         l.CronExpression,
			CrontabFilename:    l.Crontab.Filename,
			CrontabDisplayName: l.Crontab.DisplayName(),
			LineNumber:         l.LineNumber + 1, // 1-indexed for UI
			RunAsUser:          l.RunAs,
			Timezone:           timezone,
			Monitored:          len(l.Code) > 0,
			Suspended:          l.IsComment,
			Instances:          []interface{}{}, // Empty array for now
			IsDraft:            false,
		}

		// Include the job in the JSON output
		return json.Marshal(&struct {
			IsJob           bool   `json:"is_job"`
			IsComment       bool   `json:"is_comment"`
			IsEnvVar        bool   `json:"is_env_var"`
			Name            string `json:"name"`
			LineText        string `json:"line_text"`
			LineNumber      int    `json:"line_number"`
			CronExpression  string `json:"cron_expression"`
			CommandToRun    string `json:"command_to_run"`
			Code            string `json:"code"`
			RunAs           string `json:"run_as"`
			EnvVarKey       string `json:"env_var_key,omitempty"`
			EnvVarValue     string `json:"env_var_value,omitempty"`
			Key             string `json:"key,omitempty"`
			CrontabFilename string `json:"crontab_filename,omitempty"`
			DefaultName     string `json:"default_name,omitempty"`
			Job             *Job   `json:"job,omitempty"`
		}{
			IsJob:           base.IsJob,
			IsComment:       base.IsComment,
			IsEnvVar:        base.IsEnvVar,
			Name:            base.Name,
			LineText:        base.LineText,
			LineNumber:      base.LineNumber,
			CronExpression:  base.CronExpression,
			CommandToRun:    base.CommandToRun,
			Code:            base.Code,
			RunAs:           base.RunAs,
			EnvVarKey:       base.EnvVarKey,
			EnvVarValue:     base.EnvVarValue,
			Key:             base.Key,
			CrontabFilename: base.CrontabFilename,
			DefaultName:     base.DefaultName,
			Job:             &job,
		})
	}

	// For non-job lines, return the base structure
	return json.Marshal(base)
}

// IsEnvVar checks if the line is an environment variable declaration
func (l Line) IsEnvVar() bool {
	return !l.IsJob && strings.Contains(l.FullLine, "=")
}

// GetEnvVarKey extracts the key from an environment variable line
func (l Line) GetEnvVarKey() string {
	if !l.IsEnvVar() {
		return ""
	}
	parts := strings.SplitN(l.FullLine, "=", 2)
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

// GetEnvVarValue extracts the value from an environment variable line
func (l Line) GetEnvVarValue() string {
	if !l.IsEnvVar() {
		return ""
	}
	parts := strings.SplitN(l.FullLine, "=", 2)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func createAutoDiscoverLine(crontab *Crontab) *Line {
	cronExpression := fmt.Sprintf("%d * * * *", randomMinute())
	if crontab.UsesSixFieldExpressions {
		cronExpression = fmt.Sprintf("* %s", cronExpression)
	}

	// Normalize the command so it can be run reliably from crontab.
	commandToRun := strings.Join(os.Args, " ")
	commandToRun = strings.Replace(commandToRun, "--save", "", -1)
	commandToRun = strings.Replace(commandToRun, "--verbose", "", -1)
	commandToRun = strings.Replace(commandToRun, "-v", "", -1)
	commandToRun = strings.Replace(commandToRun, "--interactive", "", -1)
	commandToRun = strings.Replace(commandToRun, "-i", "", -1)
	if len(crontab.Filename) > 0 {
		commandToRun = strings.Replace(commandToRun, crontab.Filename, crontab.CanonicalName(), -1)
	}

	// Remove existing --auto flag before adding a new one to prevent doubling up
	commandToRun = strings.Replace(commandToRun, "--auto", "", -1)
	commandToRun = strings.Replace(commandToRun, " discover", " discover --auto ", -1)

	line := Line{}
	line.CronExpression = cronExpression
	line.CommandToRun = commandToRun
	line.FullLine = fmt.Sprintf("%s %s", line.CronExpression, line.CommandToRun)
	return &line
}

func isSixFieldCronExpression(splitLine []string) bool {
	matchDigitOrWildcard, _ := regexp.MatchString("^[-,?*/0-9]+$", splitLine[5])
	matchDayOfWeekStringRange, _ := regexp.MatchString("^(Mon|Tue|Wed|Thr|Fri|Sat|Sun)(-(Mon|Tue|Wed|Thr|Fri|Sat|Sun))?$", splitLine[5])
	matchDayOfWeekStringList, _ := regexp.MatchString("^((Mon|Tue|Wed|Thr|Fri|Sat|Sun),?)+$", splitLine[5])
	return matchDigitOrWildcard || matchDayOfWeekStringRange || matchDayOfWeekStringList
}

func CurrentUserCrontab() string {
	if u, err := user.Current(); err == nil {
		return fmt.Sprintf("user:%s", u.Username)
	}
	return ""
}

func CrontabFactory(username, filename string) *Crontab {
	return &Crontab{
		User:          username,
		IsUserCrontab: strings.HasPrefix(filename, "user:"),
		Filename:      filename,
	}
}

func ReadCrontabsInDirectory(username, directory string, crontabs []*Crontab) []*Crontab {
	files := EnumerateFiles(directory)
	if len(files) > 0 {
		for _, crontabFile := range files {
			crontab := CrontabFactory(username, crontabFile)
			crontab.Parse(true)
			crontabs = append(crontabs, crontab)
		}
	}

	return crontabs
}

func ReadCrontabFromFile(username, filename string, crontabs []*Crontab) []*Crontab {
	if _, err := os.Stat(filename); !strings.HasPrefix(filename, "user:") && os.IsNotExist(err) {
		return crontabs
	}

	crontab := CrontabFactory(username, filename)
	crontab.Parse(true)
	crontabs = append(crontabs, crontab)
	return crontabs
}

// GetAllCrontabs returns a slice of all available Crontab objects.
func GetAllCrontabs() ([]*Crontab, error) {
	var crontabs []*Crontab
	var username string

	// Get current username for user crontabs
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	// Read user crontab
	crontabs = ReadCrontabFromFile(username, CurrentUserCrontab(), crontabs)

	// Read system crontab if it exists
	if systemCrontab := CrontabFactory(username, SYSTEM_CRONTAB); systemCrontab.Exists() {
		crontabs = ReadCrontabFromFile(username, SYSTEM_CRONTAB, crontabs)
	}

	// Read crontabs from drop-in directory
	crontabs = ReadCrontabsInDirectory(username, DROP_IN_DIRECTORY, crontabs)

	return crontabs, nil
}

// GetCrontab returns a single Crontab object for the specified filename.
// The filename can be either a full path to a crontab file or a user crontab in the form "user:<username>".
func GetCrontab(filename string) (*Crontab, error) {
	var username string

	if strings.HasPrefix(filename, "user:") {
		username = strings.TrimPrefix(filename, "user:")
	} else if u, err := user.Current(); err == nil {
		username = u.Username
	}

	crontabs := ReadCrontabFromFile(username, filename, []*Crontab{})
	if len(crontabs) == 0 {
		return nil, fmt.Errorf("no crontab found at %s", filename)
	}
	return crontabs[0], nil
}
