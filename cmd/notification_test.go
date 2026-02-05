package cmd

import (
	"testing"
)

func TestNotificationCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "delete"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range notificationCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in notification command", name)
		}
	}
}

func TestNotificationPersistentFlags(t *testing.T) {
	flags := []string{"page", "page-size", "format", "output"}

	for _, flag := range flags {
		if notificationCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in notification command", flag)
		}
	}
}

func TestNotificationCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if notificationCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in notification create command", flag)
		}
	}
}

func TestNotificationUpdateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if notificationUpdateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in notification update command", flag)
		}
	}
}

func TestNotificationCommandAliases(t *testing.T) {
	aliases := notificationCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "notifications" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'notifications' not found")
	}
}

