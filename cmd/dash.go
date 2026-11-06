package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var launchDashboard = execDashboard

// dashLauncherCmd starts crontab-dashboard, which is released next to
// cronitor. Existing services and MCP configurations keep their command
// lines: all arguments, the environment, and standard streams pass through.
var dashLauncherCmd = &cobra.Command{
	Use:                "dash",
	Short:              "Start the Crontab Guru Dashboard (crontab-dashboard)",
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		path, err := dashboardExecutable()
		if err != nil {
			dir := filepath.Dir(cronitorPath())
			fatal(dashboardMissingMessage(Version, runtime.GOOS, runtime.GOARCH, dir, dirWritable(dir)), 1)
		}
		if err := launchDashboard(path, append([]string{"dash"}, args...), os.Environ()); err != nil {
			fatal(fmt.Sprintf("Could not start %s: %v", path, err), 1)
		}
	},
}

// RegisterDashLauncher adds `cronitor dash`. crontab-dashboard registers the
// dashboard itself instead.
func RegisterDashLauncher() {
	for _, c := range RootCmd.Commands() {
		if c == dashLauncherCmd {
			return
		}
	}
	RootCmd.AddCommand(dashLauncherCmd)
}

func cronitorPath() string {
	if path, err := executablePath(); err == nil {
		return path
	}
	return os.Args[0]
}

// dashboardExecutable finds crontab-dashboard: CRONTAB_DASHBOARD_BIN, then
// the folder that holds cronitor, then PATH.
func dashboardExecutable() (string, error) {
	if path := os.Getenv("CRONTAB_DASHBOARD_BIN"); path != "" {
		return exec.LookPath(path)
	}
	name := DashboardBinaryName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if path, err := executablePath(); err == nil {
		if sibling, err := exec.LookPath(filepath.Join(filepath.Dir(path), name)); err == nil {
			return sibling, nil
		}
	}
	return exec.LookPath(DashboardBinaryName)
}

func dashboardMissingMessage(version, goos, goarch, dir string, writable bool) string {
	url := fmt.Sprintf("https://github.com/cronitorio/cronitor-cli/releases/download/%s/%s_%s_%s", version, DashboardBinaryName, goos, goarch)
	var install string
	if goos == "windows" {
		install = fmt.Sprintf("Download %s.zip\nand extract %s.exe into %s", url, DashboardBinaryName, dir)
	} else {
		sudo := ""
		if !writable {
			sudo = "sudo "
		}
		install = fmt.Sprintf("curl -fsSL %s.tar.gz | %star -xz -C %s", url, sudo, dir)
	}
	return strings.Join([]string{
		"The Crontab Guru Dashboard is now a separate program: " + DashboardBinaryName + ".",
		"Your configuration and cron jobs are unchanged. To install it, run:",
		"",
		"  " + install,
		"",
		"Then run 'cronitor dash' again, or '" + DashboardBinaryName + "'.",
	}, "\n")
}

func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".cronitor-write-check-")
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(f.Name())
	return true
}
