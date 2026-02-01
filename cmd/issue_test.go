package cmd

import (
	"testing"
)

func TestIssueCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "resolve", "delete", "bulk"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range issueCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in issue command", name)
		}
	}
}

func TestIssuePersistentFlags(t *testing.T) {
	flags := []string{"page", "page-size", "format", "output"}

	for _, flag := range flags {
		if issueCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in issue command", flag)
		}
	}
}

func TestIssueListCommandFlags(t *testing.T) {
	flags := []string{"state", "severity", "monitor", "group", "tag", "env", "search", "time", "order-by"}

	for _, flag := range flags {
		if issueListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in issue list command", flag)
		}
	}
}

func TestIssueCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if issueCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in issue create command", flag)
		}
	}
}

func TestIssueUpdateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if issueUpdateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in issue update command", flag)
		}
	}
}

func TestIssueResolveExists(t *testing.T) {
	// Resolve is a convenience command that sets state to resolved
	if issueResolveCmd == nil {
		t.Error("issueResolveCmd should exist")
	}
	if issueResolveCmd.Args == nil {
		t.Error("issueResolveCmd should require args")
	}
}

func TestIssueListExpansionFlags(t *testing.T) {
	flags := []string{"with-statuspage-details", "with-monitor-details", "with-alert-details", "with-component-details"}

	for _, flag := range flags {
		if issueListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in issue list command", flag)
		}
	}
}

func TestIssueGetExpansionFlags(t *testing.T) {
	flags := []string{"with-statuspage-details", "with-monitor-details", "with-alert-details", "with-component-details"}

	for _, flag := range flags {
		if issueGetCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in issue get command", flag)
		}
	}
}

func TestIssueBulkCommandFlags(t *testing.T) {
	flags := []string{"action", "issues", "state", "assign-to"}

	for _, flag := range flags {
		if issueBulkCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in issue bulk command", flag)
		}
	}
}
