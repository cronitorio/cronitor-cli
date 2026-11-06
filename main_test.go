package main

import (
	"testing"

	"github.com/cronitorio/cronitor-cli/cmd"
)

// The dashboard must stay registered on the CLI while its code lives in its
// own package.
func TestDashCommandIsRegistered(t *testing.T) {
	registerCommands()
	found, _, err := cmd.RootCmd.Find([]string{"dash"})
	if err != nil {
		t.Fatal(err)
	}
	if found.Name() != "dash" {
		t.Fatalf("found %q, want dash", found.Name())
	}
	for _, flag := range []string{"port", "mcp-instance"} {
		if found.Flags().Lookup(flag) == nil {
			t.Errorf("dash is missing --%s", flag)
		}
	}
}
