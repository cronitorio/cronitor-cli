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
	cmd.RootCmd.Use = "crontab-dashboard"
	dashboard.Register(cmd.RootCmd)
}

// dashboardArgs runs the dash command unless it is already named, so
// `crontab-dashboard --port 9100` and `cronitor dash --port 9100` match.
func dashboardArgs(args []string) []string {
	if len(args) > 1 && args[1] == "dash" {
		return args
	}
	return append([]string{args[0], "dash"}, args[1:]...)
}
