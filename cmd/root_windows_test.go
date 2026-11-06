//go:build windows
// +build windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/capnspacehook/taskmaster"
	"github.com/spf13/viper"
)

// taskmasterPanic is the panic value seen in CI when taskmaster cannot read
// the OLE error of a failed Task Scheduler connection.
const taskmasterPanic = "interface conversion: error is nil, not ole.EXCEPINFO"

func withPanickingTaskScheduler(t *testing.T) {
	t.Helper()
	old := connectTaskScheduler
	connectTaskScheduler = func() (taskmaster.TaskService, error) {
		panic(taskmasterPanic)
	}
	t.Cleanup(func() { connectTaskScheduler = old })
}

func TestGetNextRunFromMonitorKeyRecoversFromTaskSchedulerPanic(t *testing.T) {
	withPanickingTaskScheduler(t)

	logFile := filepath.Join(t.TempDir(), "cronitor.log")
	oldLog := viper.GetString(varLog)
	viper.Set(varLog, logFile)
	t.Cleanup(func() { viper.Set(varLog, oldLog) })

	var got string
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("GetNextRunFromMonitorKey panicked: %v", r)
			}
		}()
		got = GetNextRunFromMonitorKey("any-key")
	}()

	if got != "" {
		t.Errorf("GetNextRunFromMonitorKey = %q, want empty", got)
	}
	logged, _ := os.ReadFile(logFile)
	if !strings.Contains(string(logged), taskmasterPanic) {
		t.Errorf("log = %q, want it to contain the panic value", logged)
	}
}

func TestRunCommandRunsJobWhenTaskSchedulerPanics(t *testing.T) {
	withCompatTest(t, &compatFake{})
	withPanickingTaskScheduler(t)

	marker := filepath.Join(t.TempDir(), "ran.txt")
	monitorCode = compatMonitorKey

	var code int
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("RunCommand panicked: %v", r)
			}
		}()
		code = RunCommand("Set-Content -Path '"+marker+"' -Value ran", true, true)
	}()

	if code != 0 {
		t.Errorf("RunCommand exit code = %d, want 0", code)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("job did not run: %v", err)
	}
}
