package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// exampleLineProblems reports a "cronitor ..." example line that names a
// command that does not exist or a --flag the named command does not have.
func exampleLineProblems(source, line string) []string {
	if i := strings.Index(line, " #"); i >= 0 {
		line = line[:i]
	}
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "cronitor" {
		return nil
	}
	target, _, err := RootCmd.Find(fields[1:])
	if err != nil {
		return []string{fmt.Sprintf("%s: %q: %v", source, strings.TrimSpace(line), err)}
	}
	var problems []string
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "--") {
			continue
		}
		name := strings.SplitN(strings.TrimPrefix(f, "--"), "=", 2)[0]
		if target.Flag(name) == nil {
			problems = append(problems, fmt.Sprintf("%s: %q: %s has no --%s flag", source, strings.TrimSpace(line), target.CommandPath(), name))
		}
	}
	return problems
}

// exampleFlagProblems walks cmd and its subcommands and reports every
// indented "cronitor ..." example line in Long or Example help that names a
// command that does not exist or a --flag the named command does not have.
func exampleFlagProblems(cmd *cobra.Command) []string {
	var problems []string
	for _, line := range strings.Split(cmd.Long+"\n"+cmd.Example, "\n") {
		if strings.HasPrefix(line, "  cronitor ") {
			problems = append(problems, exampleLineProblems(cmd.CommandPath(), line)...)
		}
	}
	for _, child := range cmd.Commands() {
		problems = append(problems, exampleFlagProblems(child)...)
	}
	return problems
}

func TestHelpExamplesUseRealFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{authCmd, environmentCmd, notificationCmd, teamCmd, apiKeyCmd} {
		for _, problem := range exampleFlagProblems(cmd) {
			t.Error(problem)
		}
	}
}

func TestREADMEExamplesUseRealFlags(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	prefixes := []string{"cronitor auth ", "cronitor environment ", "cronitor notification ", "cronitor team ", "cronitor apikey "}
	for _, line := range strings.Split(string(data), "\n") {
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				for _, problem := range exampleLineProblems("README.md", line) {
					t.Error(problem)
				}
			}
		}
	}
}
