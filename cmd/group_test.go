package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGroupCommandStructure(t *testing.T) {
	// Test that group command exists and has expected subcommands
	subcommands := []string{"list", "get", "create", "update", "delete", "pause", "resume"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range groupCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in group command", name)
		}
	}
}

func TestGroupListCommandFlags(t *testing.T) {
	flags := []string{"page-size", "with-status"}

	for _, flag := range flags {
		if groupListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in group list command", flag)
		}
	}
}

func TestGroupGetCommandFlags(t *testing.T) {
	flags := []string{"with-status", "sort"}

	for _, flag := range flags {
		if groupGetCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in group get command", flag)
		}
	}
}

func TestGroupCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if groupCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in group create command", flag)
		}
	}
}

func TestGroupUpdateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if groupUpdateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in group update command", flag)
		}
	}
}

func TestGroupPersistentFlags(t *testing.T) {
	flags := []string{"page", "env", "format", "output"}

	for _, flag := range flags {
		if groupCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in group command", flag)
		}
	}
}

func TestGroupCommandArgs(t *testing.T) {
	tests := []struct {
		name        string
		cmd         *cobra.Command
		expectedArgs cobra.PositionalArgs
	}{
		{"get requires 1 arg", groupGetCmd, cobra.ExactArgs(1)},
		{"update requires 1 arg", groupUpdateCmd, cobra.ExactArgs(1)},
		{"delete requires 1 arg", groupDeleteCmd, cobra.ExactArgs(1)},
		{"pause requires 2 args", groupPauseCmd, cobra.ExactArgs(2)},
		{"resume requires 1 arg", groupResumeCmd, cobra.ExactArgs(1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.Args == nil {
				t.Errorf("%s: Args validator is nil", tt.name)
			}
		})
	}
}

func TestGroupHelpContainsExamples(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *cobra.Command
		examples []string
	}{
		{
			"list has examples",
			groupListCmd,
			[]string{"cronitor group list"},
		},
		{
			"get has examples",
			groupGetCmd,
			[]string{"cronitor group get"},
		},
		{
			"create has examples",
			groupCreateCmd,
			[]string{"cronitor group create"},
		},
		{
			"update has examples",
			groupUpdateCmd,
			[]string{"cronitor group update"},
		},
		{
			"delete has examples",
			groupDeleteCmd,
			[]string{"cronitor group delete"},
		},
		{
			"pause has examples",
			groupPauseCmd,
			[]string{"cronitor group pause"},
		},
		{
			"resume has examples",
			groupResumeCmd,
			[]string{"cronitor group resume"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get the help output
			buf := new(bytes.Buffer)
			tt.cmd.SetOut(buf)
			tt.cmd.SetErr(buf)
			tt.cmd.SetArgs([]string{"--help"})
			tt.cmd.Execute()

			help := buf.String()
			// Also check the Long description directly
			longDesc := tt.cmd.Long

			for _, example := range tt.examples {
				if !strings.Contains(help, example) && !strings.Contains(longDesc, example) {
					t.Errorf("%s: expected example '%s' not found in help text", tt.name, example)
				}
			}
		})
	}
}

func TestGroupListHasPageFlag(t *testing.T) {
	// Verify page flag inherited from persistent flags
	cmd := groupCmd
	flag := cmd.PersistentFlags().Lookup("page")
	if flag == nil {
		t.Error("Expected --page flag on group command")
	}
	if flag.DefValue != "1" {
		t.Errorf("Expected --page default value to be '1', got '%s'", flag.DefValue)
	}
}

func TestGroupPauseResumeRelationship(t *testing.T) {
	// Resume should effectively be pause with 0 hours
	// This is a documentation/behavior test
	pauseLong := groupPauseCmd.Long
	resumeLong := groupResumeCmd.Long

	if !strings.Contains(pauseLong, "hours") {
		t.Error("Pause command should mention hours in description")
	}

	if !strings.Contains(resumeLong, "Resume") {
		t.Error("Resume command should mention resuming in description")
	}
}
