// crontab-dashboard is the Crontab Guru Dashboard. It is built from the same
// commit as cronitor and shares its configuration, flags, and API client.
package main

import (
	"os"

	"github.com/cronitorio/cronitor-cli/cmd"
	"github.com/cronitorio/cronitor-cli/internal/dashboard"
)

func main() {
	setup()
	os.Args = dashboardArgs(os.Args)
	cmd.Execute()
}

func setup() {
	cmd.RootCmd.Use = cmd.DashboardBinaryName
	cmd.SetUpdateBinary(cmd.DashboardBinaryName)
	dashboard.Register(cmd.RootCmd)
}

// passThrough are the CLI commands crontab-dashboard runs as named, so a
// standalone install needs no `cronitor` command for settings and updates.
var passThrough = map[string]bool{"dash": true, "configure": true, "update": true}

// dashboardArgs runs the dash command unless the first argument names a
// pass-through command, so `crontab-dashboard --port 9100` and
// `cronitor dash --port 9100` match.
func dashboardArgs(args []string) []string {
	if len(args) > 1 && passThrough[args[1]] {
		return args
	}
	return append([]string{args[0], "dash"}, args[1:]...)
}
