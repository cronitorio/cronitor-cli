//go:build windows
// +build windows

package cmd

import (
	"fmt"
	"github.com/capnspacehook/taskmaster"
)

// GetNextRunFromMonitorKey returns the NextRunTime timestamp from Windows
// Task Scheduler. Since each `cronitor ping` call is run independently,
// this call can't be memoized, regardless of how expensive it is.
func GetNextRunFromMonitorKey(key string) string {
	taskService, err := taskmaster.Connect()
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
