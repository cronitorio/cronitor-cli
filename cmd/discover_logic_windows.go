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

	"github.com/capnspacehook/taskmaster"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/cronitorio/cronitor-cli/lib"
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
