//go:build windows
// +build windows

// This file contains Windows-only logic that requires libraries with build constraints for Windows only.
// It must be separated out into its own file or `go build` will complain when building for non-Windows architectures.

package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/capnspacehook/taskmaster"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/rickb777/date/period"
)

func getWindowsKey(taskName string) string {
	const MonitorKeyLength = 12

	h := sha256.New()
	h.Write([]byte(taskName))
	hashed := hex.EncodeToString(h.Sum(nil))
	return hashed[:MonitorKeyLength]
}

type WrappedWindowsTask taskmaster.RegisteredTask

func NewWrappedWindowsTask(t taskmaster.RegisteredTask) WrappedWindowsTask {
	w := WrappedWindowsTask(t)
	return w
}

func (r WrappedWindowsTask) FullName() string {
	hostname, err := os.Hostname()
	if err != nil {
		log(fmt.Sprintf("err: %v", err))
		hostname = "[no-hostname]"
	}
	// Windows Task Scheduler won't allow multiple tasks with the same name, so using
	// the tasks' name should be safe. You also do not seem to be able to edit the name
	// in Windows Task Scheduler, so this seems safe as the Key as well.
	fullName := fmt.Sprintf("%s/%s", hostname, r.Name)
	// Max name length of 75, so we need to truncate
	if len(fullName) >= 74 {
		fullName = fullName[:74]
	}

	return fullName
}

func (r WrappedWindowsTask) WindowsKey() string {
	return getWindowsKey(r.FullName())
}

func (r WrappedWindowsTask) IsMicrosoftTask() bool {
	return strings.HasPrefix(r.Path, "\\Microsoft\\")
}

func (r WrappedWindowsTask) GetCommandToRun() string {
	var commands []string
	for _, action := range r.Definition.Actions {

		if action.GetType() != taskmaster.TASK_ACTION_EXEC {
			// We only support actions of type Exec, not com, email, or message (which are deprecated)
			continue
		}

		execAction := action.(taskmaster.ExecAction)

		commands = append(commands, strings.TrimSpace(fmt.Sprintf("%s %s", execAction.Path, execAction.Args)))
	}

	return strings.Join(commands, " && ")
}

func (r WrappedWindowsTask) GetNextRunTime() int64 {
	return r.NextRunTime.Unix()
}

func (r WrappedWindowsTask) GetNextRunTimeString() string {
	return strconv.Itoa(int(r.GetNextRunTime()))
}

// convertDaysOfWeek converts Windows day flags to RRULE BYDAY format (MO,TU,WE,TH,FR,SA,SU)
func convertDaysOfWeek(days taskmaster.DayOfWeek) string {
	var dayList []string

	if days&taskmaster.Monday != 0 {
		dayList = append(dayList, "MO")
	}
	if days&taskmaster.Tuesday != 0 {
		dayList = append(dayList, "TU")
	}
	if days&taskmaster.Wednesday != 0 {
		dayList = append(dayList, "WE")
	}
	if days&taskmaster.Thursday != 0 {
		dayList = append(dayList, "TH")
	}
	if days&taskmaster.Friday != 0 {
		dayList = append(dayList, "FR")
	}
	if days&taskmaster.Saturday != 0 {
		dayList = append(dayList, "SA")
	}
	if days&taskmaster.Sunday != 0 {
		dayList = append(dayList, "SU")
	}

	return strings.Join(dayList, ",")
}

// convertMonthDays converts Windows month days to RRULE BYMONTHDAY format
func convertMonthDays(days taskmaster.DayOfMonth) string {
	var dayList []string

	// Windows uses bit flags for days 1-31
	for i := 1; i <= 31; i++ {
		if days&(1<<uint(i-1)) != 0 {
			dayList = append(dayList, strconv.Itoa(i))
		}
	}

	return strings.Join(dayList, ",")
}

// convertWeekOfMonth converts Windows week and day to RRULE positional BYDAY format (e.g., 2MO for 2nd Monday)
func convertWeekOfMonth(weeks taskmaster.Week, days taskmaster.DayOfWeek) string {
	var dayList []string

	// Convert days
	dayStrings := make(map[taskmaster.DayOfWeek]string)
	dayStrings[taskmaster.Monday] = "MO"
	dayStrings[taskmaster.Tuesday] = "TU"
	dayStrings[taskmaster.Wednesday] = "WE"
	dayStrings[taskmaster.Thursday] = "TH"
	dayStrings[taskmaster.Friday] = "FR"
	dayStrings[taskmaster.Saturday] = "SA"
	dayStrings[taskmaster.Sunday] = "SU"

	// Convert weeks
	weekNumbers := make(map[taskmaster.Week]string)
	weekNumbers[taskmaster.First] = "1"
	weekNumbers[taskmaster.Second] = "2"
	weekNumbers[taskmaster.Third] = "3"
	weekNumbers[taskmaster.Fourth] = "4"
	weekNumbers[taskmaster.LastWeek] = "-1"

	// Build combinations
	for week, weekNum := range weekNumbers {
		if weeks&week != 0 {
			for day, dayStr := range dayStrings {
				if days&day != 0 {
					dayList = append(dayList, weekNum+dayStr)
				}
			}
		}
	}

	return strings.Join(dayList, ",")
}

// extractTimeComponents extracts hour and minute from a time.Time
func extractTimeComponents(t time.Time) (hour int, minute int) {
	// time.Time provides Hour() and Minute() methods
	return t.Hour(), t.Minute()
}

// formatPeriod converts a period.Period to a readable string
func formatPeriod(p period.Period) string {
	// period.Period has a String() method
	return p.String()
}

// TriggerInfo contains RRULE and boundary information from a trigger
type TriggerInfo struct {
	RRULE              string
	StartBoundary      time.Time
	EndBoundary        time.Time
	Description        string // For event-driven triggers
	RandomDelaySeconds int    // For recurring triggers (grace period)
}

// convertTriggerToRRULE converts a Windows trigger to RRULE format
// Returns TriggerInfo with RRULE string and boundary times
func convertTriggerToRRULE(trigger taskmaster.Trigger) TriggerInfo {
	var info TriggerInfo

	startBoundary := trigger.GetStartBoundary()
	endBoundary := trigger.GetEndBoundary()

	info.StartBoundary = startBoundary
	info.EndBoundary = endBoundary

	// Extract time from start boundary if available
	var timeComponents string
	if !startBoundary.IsZero() {
		hour, minute := extractTimeComponents(startBoundary)
		if minute > 0 {
			timeComponents = fmt.Sprintf(";BYHOUR=%d;BYMINUTE=%d", hour, minute)
		} else {
			timeComponents = fmt.Sprintf(";BYHOUR=%d", hour)
		}
	}

	switch t := trigger.(type) {
	case taskmaster.DailyTrigger:
		info.RRULE = fmt.Sprintf("FREQ=DAILY;INTERVAL=%d%s", t.DayInterval, timeComponents)
		// Capture random delay for grace period
		if !t.RandomDelay.IsZero() {
			info.RandomDelaySeconds = int(t.RandomDelay.Seconds())
		}

	case taskmaster.WeeklyTrigger:
		days := convertDaysOfWeek(t.DaysOfWeek)
		if days != "" {
			info.RRULE = fmt.Sprintf("FREQ=WEEKLY;INTERVAL=%d;BYDAY=%s%s", t.WeekInterval, days, timeComponents)
		} else {
			info.RRULE = fmt.Sprintf("FREQ=WEEKLY;INTERVAL=%d%s", t.WeekInterval, timeComponents)
		}
		// Capture random delay for grace period
		if !t.RandomDelay.IsZero() {
			info.RandomDelaySeconds = int(t.RandomDelay.Seconds())
		}

	case taskmaster.MonthlyTrigger:
		monthDays := convertMonthDays(t.DaysOfMonth)
		if monthDays != "" {
			info.RRULE = fmt.Sprintf("FREQ=MONTHLY;BYMONTHDAY=%s%s", monthDays, timeComponents)
		} else {
			info.RRULE = fmt.Sprintf("FREQ=MONTHLY%s", timeComponents)
		}

		if t.RunOnLastWeekOfMonth {
			if monthDays != "" {
				info.RRULE = fmt.Sprintf("FREQ=MONTHLY;BYMONTHDAY=%s,-1%s", monthDays, timeComponents)
			} else {
				info.RRULE = fmt.Sprintf("FREQ=MONTHLY;BYMONTHDAY=-1%s", timeComponents)
			}
		}
		// Capture random delay for grace period
		if !t.RandomDelay.IsZero() {
			info.RandomDelaySeconds = int(t.RandomDelay.Seconds())
		}

	case taskmaster.MonthlyDOWTrigger:
		daySpec := convertWeekOfMonth(t.WeeksOfMonth, t.DaysOfWeek)
		if daySpec != "" {
			info.RRULE = fmt.Sprintf("FREQ=MONTHLY;BYDAY=%s%s", daySpec, timeComponents)
		} else {
			info.RRULE = fmt.Sprintf("FREQ=MONTHLY%s", timeComponents)
		}
		// Capture random delay for grace period
		if !t.RandomDelay.IsZero() {
			info.RandomDelaySeconds = int(t.RandomDelay.Seconds())
		}

	case taskmaster.TimeTrigger:
		// One-time trigger - skip
		return info

	case taskmaster.BootTrigger:
		delay := ""
		if !t.Delay.IsZero() {
			delay = fmt.Sprintf(" (waits %s)", formatPeriod(t.Delay))
		}
		info.Description = fmt.Sprintf("Runs on system boot%s", delay)
		return info

	case taskmaster.LogonTrigger:
		delay := ""
		if !t.Delay.IsZero() {
			delay = fmt.Sprintf(" after %s", formatPeriod(t.Delay))
		}
		user := ""
		if t.UserID != "" {
			user = fmt.Sprintf(" for user %s", t.UserID)
		}
		info.Description = fmt.Sprintf("Runs on user logon%s%s", user, delay)
		return info

	case taskmaster.IdleTrigger:
		info.Description = "Runs when system is idle"
		return info

	case taskmaster.RegistrationTrigger:
		delay := ""
		if !t.Delay.IsZero() {
			delay = fmt.Sprintf(" after %s", formatPeriod(t.Delay))
		}
		info.Description = fmt.Sprintf("Runs when task is registered%s", delay)
		return info

	case taskmaster.SessionStateChangeTrigger:
		user := ""
		if t.UserId != "" {
			user = fmt.Sprintf(" for %s", t.UserId)
		}
		info.Description = fmt.Sprintf("Runs on %s%s", t.StateChange.String(), user)
		return info

	default:
		// Unknown trigger type
		return info
	}

	return info
}

// shouldIncludeTask determines if a Windows task should be included in the sync
// Returns true if the task has at least one enabled, schedulable trigger
// Excludes tasks that only have:
// - Disabled triggers (enabled=false)
// - One-time triggers (TimeTrigger)
// - Session state change triggers (SessionStateChangeTrigger)
func shouldIncludeTask(task taskmaster.RegisteredTask) bool {
	if task.Definition.Triggers == nil || len(task.Definition.Triggers) == 0 {
		// No triggers = skip
		return false
	}

	hasValidTrigger := false

	for _, trigger := range task.Definition.Triggers {
		// Skip disabled triggers
		if !trigger.GetEnabled() {
			continue
		}

		// Check trigger type
		switch trigger.(type) {
		case taskmaster.TimeTrigger:
			// Skip one-time triggers
			continue
		case taskmaster.SessionStateChangeTrigger:
			// Skip session state change triggers
			continue
		case taskmaster.DailyTrigger, taskmaster.WeeklyTrigger,
			taskmaster.MonthlyTrigger, taskmaster.MonthlyDOWTrigger,
			taskmaster.BootTrigger, taskmaster.LogonTrigger,
			taskmaster.IdleTrigger, taskmaster.RegistrationTrigger:
			// Valid trigger type found
			hasValidTrigger = true
			break
		}

		if hasValidTrigger {
			break
		}
	}

	return hasValidTrigger
}

// ScheduleInfo contains schedules and optional note for event-driven tasks
type ScheduleInfo struct {
	Schedules       []string
	Note            string
	MaxGraceSeconds int // Maximum random delay across all triggers (for grace_seconds field)
}

// extractScheduleData extracts schedule information from a Windows task and converts to RRULE format
// Returns list of RRULE strings and note for event-driven tasks
func extractScheduleData(task taskmaster.RegisteredTask) ScheduleInfo {
	var info ScheduleInfo

	if task.Definition.Triggers == nil || len(task.Definition.Triggers) == 0 {
		return info
	}

	var rrules []string
	var descriptions []string
	var maxGraceSeconds int

	for _, trigger := range task.Definition.Triggers {
		triggerInfo := convertTriggerToRRULE(trigger)

		if triggerInfo.RRULE != "" {
			// Recurring schedule trigger
			rrules = append(rrules, triggerInfo.RRULE)

			// Track maximum random delay (for grace_seconds)
			if triggerInfo.RandomDelaySeconds > maxGraceSeconds {
				maxGraceSeconds = triggerInfo.RandomDelaySeconds
			}
		} else if triggerInfo.Description != "" {
			// Event-driven trigger
			descriptions = append(descriptions, triggerInfo.Description)
		}
	}

	// Set maximum grace seconds
	info.MaxGraceSeconds = maxGraceSeconds

	if len(rrules) > 0 {
		info.Schedules = rrules
	}

	// If there are event-driven triggers, add to note
	if len(descriptions) > 0 {
		info.Note = strings.Join(descriptions, "; ")
	}

	return info
}

func processWindowsTaskScheduler() bool {
	const CronitorWindowsPath = "C:\\Program Files\\cronitor.exe"

	taskService, err := taskmaster.Connect()
	if err != nil {
		log(fmt.Sprintf("err: %v", err))
		return false
	}
	defer taskService.Disconnect()
	collection, err := taskService.GetRegisteredTasks()
	if err != nil {
		log(fmt.Sprintf("err: %v", err))
		return false
	}
	defer collection.Release()

	// Read crontab into map of Monitor structs
	monitors := map[string]*lib.Monitor{}
	monitorToRegisteredTask := map[string]taskmaster.RegisteredTask{}
	for _, task := range collection {
		t := NewWrappedWindowsTask(task)
		// Skip all built-in tasks; users don't want to monitor those
		if t.IsMicrosoftTask() {
			continue
		}

		// Skip tasks that only have disabled, one-time, or session state change triggers
		if !shouldIncludeTask(task) {
			continue
		}

		defaultName := t.FullName()
		tags := createTags()
		key := t.WindowsKey()
		name := defaultName
		skip := false

		// The monitor name will always be the same, so we don't have to fetch it
		// from the Cronitor existing monitors

		if !isAutoDiscover {
			fmt.Println(fmt.Sprintf("\n    %s  %s", defaultName, t.GetCommandToRun()))
			for {
				model := initialNameInputModel(name)
				p := tea.NewProgram(model)

				if result, err := p.Run(); err == nil {
					finalModel := result.(nameInputModel)
					if !finalModel.done {
						printWarningText("Skipped", true)
						skip = true
					} else {
						name = finalModel.textInput.Value()
					}
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

		var notifications []string
		if notificationList != "" {
			notifications = []string{notificationList}
		} else {
			notifications = []string{"default"}
		}

		monitor := lib.Monitor{
			DefaultName:      defaultName,
			Name:             name,
			Key:              key,
			Platform:         lib.WINDOWS,
			Tags:             tags,
			Type:             "job",
			Notify:           notifications,
			NoStdoutPassthru: noStdoutPassthru,
		}
		tz := effectiveTimezoneLocationName()
		if tz.Name != "" {
			monitor.Timezone = tz.Name
		}

		// Extract schedule data from Windows triggers and convert to RRULE
		scheduleInfo := extractScheduleData(task)
		if len(scheduleInfo.Schedules) > 0 {
			monitor.Schedules = &scheduleInfo.Schedules
		}
		if scheduleInfo.Note != "" {
			monitor.Note = scheduleInfo.Note
		}
		if scheduleInfo.MaxGraceSeconds > 0 {
			monitor.GraceSeconds = scheduleInfo.MaxGraceSeconds
		}

		monitors[key] = &monitor
		monitorToRegisteredTask[key] = task
	}

	printLn()

	if len(monitors) > 0 {
		printDoneText("Sending to Cronitor", true)
	}

	monitors, err = getCronitorApi().PutMonitors(monitors)
	if err != nil {
		fatal(err.Error(), 1)
	}

	if !dryRun && len(monitors) > 0 {
		for key, task := range monitorToRegisteredTask {
			newDefinition := task.Definition
			// Clear out all existing actions on the new definition
			newDefinition.Actions = []taskmaster.Action{}
			var actionList []taskmaster.Action
			for _, action := range task.Definition.Actions {
				if action.GetType() != taskmaster.TASK_ACTION_EXEC {
					// We only support actions of type Exec, not com, email, or message (which are deprecated)

					fmt.Printf("not exec: %v", action)

					// We don't want to delete the old actions
					actionList = append(actionList, action)
					continue
				}

				execAction := action.(taskmaster.ExecAction)

				// If the action has already been converted to use cronitor.exe, then we
				// don't need to modify it
				// TODO: What if cronitor.exe has been renamed?
				if strings.HasSuffix(strings.ToLower(execAction.Path), "cronitor.exe") {
					actionList = append(actionList, action)
					continue
				}

				actionList = append(actionList, taskmaster.ExecAction{
					ID:         execAction.ID,
					Path:       CronitorWindowsPath,
					Args:       strings.TrimSpace(fmt.Sprintf("exec %s %s %s", key, execAction.Path, execAction.Args)),
					WorkingDir: execAction.WorkingDir,
				})
			}
			for _, action := range actionList {
				newDefinition.AddAction(action)
			}

			output, _ := json.Marshal(newDefinition)
			log(fmt.Sprintf("%s: %s", task.Path, output))

			newTask, err := taskService.UpdateTask(task.Path, newDefinition)
			defer newTask.Release()
			if err != nil {
				serialized, _ := json.Marshal(newTask)
				log(fmt.Sprintf("err updating task %s: %v. JSON: %s", task.Path, err, serialized))
				printWarningText(fmt.Sprintf("Could not update task %s to automatically ping Cronitor. Error: `%s`", task.Name, err), true)
			}
		}
	}

	return len(monitors) > 0
}
