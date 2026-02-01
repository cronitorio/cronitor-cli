package cmd

import (
	"testing"
)

func TestEnvironmentCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "delete"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range environmentCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in environment command", name)
		}
	}
}

func TestEnvironmentPersistentFlags(t *testing.T) {
	flags := []string{"page", "format", "output"}

	for _, flag := range flags {
		if environmentCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in environment command", flag)
		}
	}
}

func TestEnvironmentCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if environmentCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in environment create command", flag)
		}
	}
}

func TestEnvironmentUpdateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if environmentUpdateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in environment update command", flag)
		}
	}
}

func TestEnvironmentCommandAliases(t *testing.T) {
	aliases := environmentCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "env" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'env' not found")
	}
}
