package cmd

import (
	"testing"
)

func TestMaintenanceCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "delete"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range maintenanceCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in maintenance command", name)
		}
	}
}

func TestMaintenancePersistentFlags(t *testing.T) {
	flags := []string{"page", "format", "output"}

	for _, flag := range flags {
		if maintenanceCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in maintenance command", flag)
		}
	}
}

func TestMaintenanceListCommandFlags(t *testing.T) {
	flags := []string{"past", "ongoing", "upcoming", "statuspage", "env", "with-monitors"}

	for _, flag := range flags {
		if maintenanceListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in maintenance list command", flag)
		}
	}
}

func TestMaintenanceCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if maintenanceCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in maintenance create command", flag)
		}
	}
}

func TestMaintenanceCommandAliases(t *testing.T) {
	aliases := maintenanceCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "maint" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'maint' not found")
	}
}
