package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDashLauncherForwardsArgumentsAndEnvironment(t *testing.T) {
	RegisterDashLauncher()
	bin := filepath.Join(t.TempDir(), DashboardBinaryName)
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRONTAB_DASHBOARD_BIN", bin)
	t.Setenv(varDashPassword, "fake-dash-password-not-real")

	oldLaunch := launchDashboard
	t.Cleanup(func() { launchDashboard = oldLaunch; RootCmd.SetArgs([]string{}) })
	var gotPath string
	var gotArgs, gotEnv []string
	launchDashboard = func(path string, args, env []string) error {
		gotPath, gotArgs, gotEnv = path, args, env
		return nil
	}

	RootCmd.SetArgs([]string{"--config", "/tmp/dash config.json", "dash", "--mcp-instance", "production", "--safe-mode"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotPath != bin {
		t.Errorf("path = %q, want %q", gotPath, bin)
	}
	want := []string{"dash", "--config", "/tmp/dash config.json", "--mcp-instance", "production", "--safe-mode"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("args = %q, want %q", gotArgs, want)
	}
	if !strings.Contains(strings.Join(gotEnv, "\n"), varDashPassword+"=fake-dash-password-not-real") {
		t.Error("environment not forwarded")
	}
}

func TestDashboardExecutableFindsSiblingOfCronitor(t *testing.T) {
	dir := t.TempDir()
	name := DashboardBinaryName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	sibling := filepath.Join(dir, name)
	if err := os.WriteFile(sibling, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CRONTAB_DASHBOARD_BIN", "")
	t.Setenv("PATH", t.TempDir())
	oldPath := executablePath
	executablePath = func() (string, error) { return filepath.Join(dir, "cronitor"), nil }
	t.Cleanup(func() { executablePath = oldPath })

	got, err := dashboardExecutable()
	if err != nil || got != sibling {
		t.Fatalf("dashboardExecutable() = %q, %v; want %q", got, err, sibling)
	}
}

func TestDashboardExecutableMissing(t *testing.T) {
	t.Setenv("CRONTAB_DASHBOARD_BIN", "")
	t.Setenv("PATH", t.TempDir())
	oldPath := executablePath
	executablePath = func() (string, error) { return filepath.Join(t.TempDir(), "cronitor"), nil }
	t.Cleanup(func() { executablePath = oldPath })
	if _, err := dashboardExecutable(); err == nil {
		t.Fatal("found a dashboard that is not installed")
	}
}

func TestDashboardMissingMessage(t *testing.T) {
	base := "https://github.com/cronitorio/cronitor-cli/releases/download/34.0/"
	for _, tc := range []struct {
		name, goos, goarch string
		writable           bool
		want, notWant      []string
	}{
		{"linux writable", "linux", "amd64", true,
			[]string{"now a separate program", "unchanged", "curl -fsSL " + base + "crontab-dashboard_linux_amd64.tar.gz | tar -xz -C /usr/local/bin", "cronitor dash"},
			[]string{"sudo"}},
		{"linux root dir", "linux", "arm64", false,
			[]string{"curl -fsSL " + base + "crontab-dashboard_linux_arm64.tar.gz | sudo tar -xz -C /usr/local/bin"}, nil},
		{"windows", "windows", "amd64", true,
			[]string{base + "crontab-dashboard_windows_amd64.zip", "crontab-dashboard.exe", "/usr/local/bin"},
			[]string{"curl", "sudo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := dashboardMissingMessage("34.0", tc.goos, tc.goarch, "/usr/local/bin", tc.writable)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("message lacks %q:\n%s", w, got)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(got, w) {
					t.Errorf("message has %q:\n%s", w, got)
				}
			}
		})
	}
}
