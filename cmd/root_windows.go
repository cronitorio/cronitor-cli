//go:build windows
// +build windows

package cmd

import (
	"fmt"

	"github.com/capnspacehook/taskmaster"
)

// connectTaskScheduler opens the Task Scheduler connection. Tests replace it.
var connectTaskScheduler = taskmaster.Connect

// GetNextRunFromMonitorKey returns the NextRunTime timestamp from Windows
// Task Scheduler. Since each `cronitor ping` call is run independently,
// this call can't be memoized, regardless of how expensive it is.
//
// The next run time is optional, so this never stops the job from running.
// taskmaster panics instead of returning an error when a Task Scheduler call
// fails with an OLE error that has no EXCEPINFO (an unchecked type assertion
// in getOLEErrorCode). Connect and GetRegisteredTasks both use that path, and
// task parsing uses oleutil.MustGetProperty, which panics on any COM error.
// So recover from any panic here and return "".
func GetNextRunFromMonitorKey(key string) (nextRun string) {
	defer func() {
		if r := recover(); r != nil {
			log(fmt.Sprintf("err: could not read next run time from Task Scheduler: %v", r))
			nextRun = ""
		}
	}()

	taskService, err := connectTaskScheduler()
	if err != nil {
		log(fmt.Sprintf("err: %v", err))
		return ""
	}
	defer taskService.Disconnect()
	collection, err := taskService.GetRegisteredTasks()
	if err != nil {
		log(fmt.Sprintf("err: %v", err))
		return ""
	}
	defer collection.Release()

	for _, task := range collection {
		t := NewWrappedWindowsTask(task)

		if t.WindowsKey() == key {
			return t.GetNextRunTimeString()
		}
	}

	return ""
}
