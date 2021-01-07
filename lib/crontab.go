package lib

import (
	"crypto/sha1"
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
	User                    string
	IsUserCrontab           bool
	IsSaved                 bool
	Filename                string
	Lines                   []*Line
	TimezoneLocationName    *TimezoneLocationName
	UsesSixFieldExpressions bool
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
			CronExpression: cronExpression,
			FullLine:       fullLine,
			LineNumber:     lineNumber,
			RunAs:          runAs,
		}

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
				continue // remove the auto-discover line from the crontab if --no-auto-discover flag is passed
			}
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
	if crontabLines == "" {
		return errors.New("cannot save crontab, file is empty")
	}

	if c.IsUserCrontab {
		cmd := exec.Command("crontab", "-")

		// crontab will use whatever $EDITOR is set. Temporarily just cat it out.
		cmd.Env = []string{"EDITOR=/bin/cat"}
		cmdStdin, _ := cmd.StdinPipe()
		cmdStdin.Write([]byte(crontabLines + "\n"))
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
		if u, err := user.Current(); err == nil {
			return fmt.Sprintf("user \"%s\" crontab", u.Username)
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
	// Users don't want to see "plumbing" cron jobs on their dashboard...
	return len(l.CronExpression) > 0 && len(l.CommandToRun) > 0 && !l.IsMetaCronJob() && !l.HasLegacyIntegration()
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
	if !l.IsMonitorable() || len(l.Code) > 0 {
		// If a cronitor integration already existed on the line we have nothing else here to change
		return l.FullLine
	}

	var lineParts []string
	lineParts = append(lineParts, l.CronExpression)
	lineParts = append(lineParts, l.RunAs)

	if len(l.Mon.Code) > 0 {
		lineParts = append(lineParts, "cronitor")
		if l.Mon.NoStdoutPassthru {
			lineParts = append(lineParts, "--no-stdout")
		}
		lineParts = append(lineParts, "exec")
		lineParts = append(lineParts, l.Mon.Code)

		if len(l.CommandToRun) > 0 {
			if l.CommandIsComplex() {
				lineParts = append(lineParts, "\""+strings.Replace(l.CommandToRun, "\"", "\\\"", -1)+"\"")
			} else {
				lineParts = append(lineParts, l.CommandToRun)
			}
		}
	} else {
		return l.FullLine
	}

	return strings.Replace(strings.Join(lineParts, " "), "  ", " ", -1)
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

func EnumerateCrontabFiles(dirToEnumerate string) []string {
	var fileList []string
	files, err := ioutil.ReadDir(dirToEnumerate)
	if err != nil {
		return fileList
	}

	for _, f := range files {
		firstChar := string([]rune(f.Name())[0])
		if firstChar == "." {
			continue
		}

		fileList = append(fileList, filepath.Join(dirToEnumerate, f.Name()))
	}

	return fileList
}

func CrontabFactory(username, filename string) *Crontab {
	return &Crontab{
		User:          username,
		IsUserCrontab: filename == "",
		Filename:      filename,
	}
}

func ReadCrontabsInDirectory(username, directory string, crontabs []*Crontab) []*Crontab {
	files := EnumerateCrontabFiles(directory)
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
	if _, err := os.Stat(filename); filename != "" && os.IsNotExist(err) {
		return crontabs
	}

	crontab := CrontabFactory(username, filename)
	crontab.Parse(true)
	crontabs = append(crontabs, crontab)
	return crontabs
}
