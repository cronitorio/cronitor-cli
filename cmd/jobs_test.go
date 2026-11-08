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

	RootCmd.SetArgs([]string{"jobs", "not-a-command"})
	t.Cleanup(func() { RootCmd.SetArgs(nil) })
	execErr := RootCmd.Execute()
	if execErr == nil || !strings.Contains(execErr.Error(), "unknown command") || !strings.Contains(execErr.Error(), `"not-a-command"`) {
		t.Fatalf("jobs typo: %v", execErr)
	}
}
