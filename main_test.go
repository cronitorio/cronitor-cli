package main

import (
	"testing"

	"github.com/cronitorio/cronitor-cli/cmd"
)

// cronitor dash starts the separately installed crontab-dashboard. The CLI
// binary does not contain the dashboard.
func TestDashIsTheLauncher(t *testing.T) {
	registerCommands()
	found, _, err := cmd.RootCmd.Find([]string{"dash"})
	if err != nil {
		t.Fatal(err)
	}
	if found.Name() != "dash" {
		t.Fatalf("found %q, want dash", found.Name())
	}
	if !found.DisableFlagParsing {
		t.Error("dash parses flags; the launcher must pass them through")
	}
	if found.Flags().Lookup("port") != nil {
		t.Error("dash has --port; the dashboard is still built into cronitor")
	}
}
