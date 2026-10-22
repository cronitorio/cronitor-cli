package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// exampleLineProblems reports a "cronitor ..." example line that names a
// command that does not exist, a flag the named command does not have, or a
// leftover argument on a command that still has subcommands.
func exampleLineProblems(source, line string) []string {
	if i := strings.Index(line, " #"); i >= 0 {
		line = line[:i]
	}
	fields := splitExampleWords(line)
	if len(fields) < 2 || fields[0] != "cronitor" {
		return nil
	}
	target, rest, err := RootCmd.Find(fields[1:])
	quote := strings.TrimSpace(line)
	if err != nil {
		return []string{fmt.Sprintf("%s: %q: %v", source, quote, err)}
	}
	var problems []string
	positionals := nonFlagArgs(target, rest)
	if len(target.Commands()) > 0 {
		for _, arg := range positionals {
			problems = append(problems, fmt.Sprintf("%s: %q: %s has subcommands; %q is not one of them", source, quote, target.CommandPath(), arg))
		}
	} else if target.Args != nil {
		if err := target.Args(target, positionals); err != nil {
			problems = append(problems, fmt.Sprintf("%s: %q: %s: %v", source, quote, target.CommandPath(), err))
		}
	}
	args := fields[1:]
	for i := 0; i < len(args); i++ {
		f := args[i]
		switch {
		case f == "--":
			return problems
		case strings.HasPrefix(f, "--"):
			name := strings.SplitN(strings.TrimPrefix(f, "--"), "=", 2)[0]
			flag := target.Flag(name)
			if name != "" && flag == nil {
				problems = append(problems, fmt.Sprintf("%s: %q: %s has no --%s flag", source, quote, target.CommandPath(), name))
			}
			if !strings.Contains(f, "=") && flagTakesValue(flag) && i+1 < len(args) {
				i++
			}
		case strings.HasPrefix(f, "-") && len(f) > 1:
			raw := strings.TrimPrefix(f, "-")
			name, _, hasEq := strings.Cut(raw, "=")
			var last *pflag.Flag
			for _, letter := range name {
				s := string(letter)
				last = commandShorthand(target, s)
				if last == nil {
					problems = append(problems, fmt.Sprintf("%s: %q: %s has no -%s flag", source, quote, target.CommandPath(), s))
				}
			}
			if !hasEq && flagTakesValue(last) && i+1 < len(args) {
				i++
			}
		}
	}
	return problems
}

// nonFlagArgs returns arguments left after Find that are not flags or flag
// values. A command with subcommands should have none of these: the leftover
// word is an unknown nested subcommand.
func nonFlagArgs(cmd *cobra.Command, args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "--") {
			name := strings.SplitN(strings.TrimPrefix(a, "--"), "=", 2)[0]
			if !strings.Contains(a, "=") && flagTakesValue(cmd.Flag(name)) && i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			raw := strings.TrimPrefix(a, "-")
			name, _, hasEq := strings.Cut(raw, "=")
			if !hasEq && name != "" && flagTakesValue(commandShorthand(cmd, name[len(name)-1:])) && i+1 < len(args) {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// splitExampleWords splits a help example on whitespace, keeping single- or
// double-quoted text, including spaces and leading dashes, as one word.
// Quotes are removed. A placeholder such as <key> is one word.
func splitExampleWords(line string) []string {
	var words []string
	var b strings.Builder
	var quote byte
	inWord := false
	flush := func() {
		if inWord {
			words = append(words, b.String())
			b.Reset()
			inWord = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			b.WriteByte(c)
			inWord = true
			continue
		}
		switch c {
		case ' ', '\t', '\n', '\r':
			flush()
		case '\'', '"':
			quote = c
			inWord = true
		default:
			b.WriteByte(c)
			inWord = true
		}
	}
	flush()
	return words
}

func flagTakesValue(f *pflag.Flag) bool {
	return f != nil && f.Value.Type() != "bool"
}

func commandShorthand(cmd *cobra.Command, letter string) *pflag.Flag {
	if letter == "" || len(letter) != 1 {
		return nil
	}
	if f := cmd.Flags().ShorthandLookup(letter); f != nil {
		return f
	}
	if f := cmd.InheritedFlags().ShorthandLookup(letter); f != nil {
		return f
	}
	for p := cmd; p != nil; p = p.Parent() {
		if p.HasPersistentFlags() {
			if f := p.PersistentFlags().ShorthandLookup(letter); f != nil {
				return f
			}
		}
	}
	return nil
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

func TestSplitExampleWords(t *testing.T) {
	got := splitExampleWords(`cronitor site update my-site --data '{"name":"My Website","sampling":-1}' "two words"`)
	want := []string{
		"cronitor", "site", "update", "my-site", "--data",
		`{"name":"My Website","sampling":-1}`,
		"two words",
	}
	if len(got) != len(want) {
		t.Fatalf("splitExampleWords = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("word %d = %q, want %q", i, got[i], want[i])
		}
	}
	placeholders := splitExampleWords("cronitor issue update <key> --data '{}'")
	if len(placeholders) != 6 || placeholders[3] != "<key>" {
		t.Fatalf("placeholder split = %#v", placeholders)
	}
}

func TestHelpExamplesUseRealFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{authCmd, environmentCmd, notificationCmd, teamCmd, apiKeyCmd, issueCmd, maintenanceCmd, siteCmd, statuspageCmd} {
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
	prefixes := []string{"cronitor auth ", "cronitor environment ", "cronitor notification ", "cronitor team ", "cronitor apikey ", "cronitor issue ", "cronitor maintenance ", "cronitor site ", "cronitor statuspage "}
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
