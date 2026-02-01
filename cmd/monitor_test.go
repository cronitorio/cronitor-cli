package cmd

import (
	"testing"
)

func TestMonitorCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "delete", "search", "clone"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range monitorCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in monitor command", name)
		}
	}
}

func TestMonitorListCommandFlags(t *testing.T) {
	flags := []string{"type", "group", "tag", "state", "search", "page-size", "sort"}

	for _, flag := range flags {
		if monitorListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in monitor list command", flag)
		}
	}
}

func TestMonitorGetCommandFlags(t *testing.T) {
	flags := []string{"with-events", "with-invocations"}

	for _, flag := range flags {
		if monitorGetCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in monitor get command", flag)
		}
	}
}

func TestMonitorPersistentFlags(t *testing.T) {
	flags := []string{"page", "env", "format", "output"}

	for _, flag := range flags {
		if monitorCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in monitor command", flag)
		}
	}
}

func TestMonitorDeleteSupportsMultipleArgs(t *testing.T) {
	// Delete should accept 1 or more arguments for bulk delete
	if monitorDeleteCmd.Args == nil {
		t.Error("monitor delete command should have Args validator")
	}
}

func TestMonitorCloneCommandFlags(t *testing.T) {
	flags := []string{"name"}

	for _, flag := range flags {
		if monitorCloneCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in monitor clone command", flag)
		}
	}
}
