package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestDashLaunchForwardsArgumentsAndConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	path := filepath.Join(t.TempDir(), "dashboard")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRONTAB_DASHBOARD_BIN", path)
	t.Setenv(varDashPassword, "fake-dash-password-not-real")
	changed := map[*pflag.Flag]bool{}
	RootCmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) { changed[flag] = flag.Changed; flag.Changed = false })
	t.Cleanup(func() {
		for flag, old := range changed {
			flag.Changed = old
		}
	})
	oldLaunch := launchDashboard
	t.Cleanup(func() { launchDashboard = oldLaunch; RootCmd.SetArgs([]string{}) })
	called := false
	launchDashboard = func(gotPath string, args, env []string) error {
		called = true
		if gotPath != path {
			t.Errorf("path = %q", gotPath)
		}
		want := []string{"--config", "/tmp/dashboard config.json", "--mcp-instance", "production", "--safe-mode"}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("args = %q, want %q", args, want)
		}
		if !strings.Contains(strings.Join(env, "\n"), varDashPassword+"=fake-dash-password-not-real") {
			t.Error("environment lost")
		}
		return nil
	}
	RootCmd.SetArgs([]string{"dash", "--config", "/tmp/dashboard config.json", "--mcp-instance", "production", "--safe-mode"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("launcher not invoked")
	}
}

func TestDashboardExecutableMissingHasInstallGuidance(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CRONTAB_DASHBOARD_BIN", "")
	_, err := dashboardExecutable()
	if err == nil || !strings.Contains(err.Error(), "installed separately") {
		t.Fatalf("missing install guidance: %v", err)
	}
}
