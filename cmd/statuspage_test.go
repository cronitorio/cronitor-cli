package cmd

import (
	"testing"
)

func TestStatuspageCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "delete", "component"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range statuspageCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in statuspage command", name)
		}
	}
}

func TestStatuspagePersistentFlags(t *testing.T) {
	flags := []string{"page", "format", "output"}

	for _, flag := range flags {
		if statuspageCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in statuspage command", flag)
		}
	}
}

func TestStatuspageListCommandFlags(t *testing.T) {
	flags := []string{"with-status", "with-components"}

	for _, flag := range flags {
		if statuspageListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in statuspage list command", flag)
		}
	}
}

func TestStatuspageCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if statuspageCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in statuspage create command", flag)
		}
	}
}

func TestComponentCommandStructure(t *testing.T) {
	subcommands := []string{"list", "create", "update", "delete"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range componentCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in component command", name)
		}
	}
}

func TestComponentListCommandFlags(t *testing.T) {
	flags := []string{"statuspage", "with-status"}

	for _, flag := range flags {
		if componentListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in component list command", flag)
		}
	}
}

func TestComponentCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if componentCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in component create command", flag)
		}
	}
}

func TestComponentUpdateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if componentUpdateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in component update command", flag)
		}
	}
}

func TestComponentUpdateRequiresArgs(t *testing.T) {
	if componentUpdateCmd.Args == nil {
		t.Error("componentUpdateCmd should require args")
	}
}
