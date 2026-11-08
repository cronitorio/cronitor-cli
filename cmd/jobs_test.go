package cmd

import (
	"strings"
	"testing"
)

// TestJobsCommandLayout locks the clean break: sync, list, shell, and dash
// live only under jobs, and discover is only an alias of jobs sync.
func TestJobsCommandLayout(t *testing.T) {
	got := map[string]bool{}
	for _, c := range jobsCmd.Commands() {
		if c.Name() == "help" {
			continue
		}
		got[c.Name()] = true
		if c.Parent() != jobsCmd {
			t.Errorf("%s parent = %v", c.Name(), c.Parent())
		}
	}
	for _, name := range []string{"sync", "list", "shell", "dash"} {
		if !got[name] {
			t.Errorf("jobs is missing %s", name)
		}
	}
	if len(got) != 4 {
		t.Errorf("jobs subcommands = %v", got)
	}

	aliasOK := false
	for _, alias := range discoverCmd.Aliases {
		if alias == "discover" {
			aliasOK = true
		}
	}
	if !aliasOK {
		t.Fatalf("sync aliases = %v", discoverCmd.Aliases)
	}
	viaAlias, _, err := RootCmd.Find([]string{"jobs", "discover"})
	if err != nil || viaAlias != discoverCmd {
		t.Fatalf("jobs discover resolved to %v (%v)", viaAlias, err)
	}
	for _, flag := range []string{"auto", "dry-run", "format", "with-monitored", "silent"} {
		if discoverCmd.Flags().Lookup(flag) == nil {
			t.Errorf("sync is missing --%s", flag)
		}
	}
	if dashCmd.Flags().Lookup("mcp-instance") == nil || dashCmd.Flags().Lookup("mcp") == nil {
		t.Fatal("dash is missing --mcp-instance or --mcp")
	}
	if listCmd.Flags().Lookup("json") == nil {
		t.Fatal("list is missing --json")
	}

	for _, name := range []string{"sync", "discover", "list", "shell", "dash"} {
		found, _, findErr := RootCmd.Find([]string{name})
		if findErr == nil || !strings.Contains(findErr.Error(), "unknown command") || !strings.Contains(findErr.Error(), `"`+name+`"`) {
			path := ""
			if found != nil {
				path = found.CommandPath()
			}
			t.Errorf("root %s: %v (%s)", name, findErr, path)
		}
	}
}
