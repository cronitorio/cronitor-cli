package cmd

import (
	"testing"
)

func TestSiteCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get", "create", "update", "delete", "query", "error"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range siteCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in site command", name)
		}
	}
}

func TestSitePersistentFlags(t *testing.T) {
	flags := []string{"page", "page-size", "format", "output"}

	for _, flag := range flags {
		if siteCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Expected persistent flag '--%s' not found in site command", flag)
		}
	}
}

func TestSiteGetCommandFlags(t *testing.T) {
	flags := []string{"with-snippet"}

	for _, flag := range flags {
		if siteGetCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in site get command", flag)
		}
	}
}

func TestSiteCreateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if siteCreateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in site create command", flag)
		}
	}
}

func TestSiteUpdateCommandFlags(t *testing.T) {
	flags := []string{"data"}

	for _, flag := range flags {
		if siteUpdateCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in site update command", flag)
		}
	}
}

func TestSiteQueryCommandFlags(t *testing.T) {
	flags := []string{"site", "type", "time", "start", "end", "metric", "group-by", "filter", "order-by", "timezone", "bucket", "compare"}

	for _, flag := range flags {
		if siteQueryCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in site query command", flag)
		}
	}
}

func TestSiteErrorCommandStructure(t *testing.T) {
	subcommands := []string{"list", "get"}

	for _, name := range subcommands {
		found := false
		for _, cmd := range siteErrorsCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand '%s' not found in site error command", name)
		}
	}
}

func TestSiteErrorCommandAliases(t *testing.T) {
	aliases := siteErrorsCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "errors" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected alias 'errors' not found for error command")
	}
}

func TestSiteErrorListCommandFlags(t *testing.T) {
	flags := []string{"site"}

	for _, flag := range flags {
		if siteErrorListCmd.Flags().Lookup(flag) == nil {
			t.Errorf("Expected flag '--%s' not found in site error list command", flag)
		}
	}
}
