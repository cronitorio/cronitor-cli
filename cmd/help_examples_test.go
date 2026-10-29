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
	fields, err := splitExampleWords(line)
	if err != nil {
		return []string{fmt.Sprintf("%s: %q: %v", source, strings.TrimSpace(line), err)}
	}
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
			if !strings.Contains(f, "=") && flagTakesValue(flag) {
				if i+1 >= len(args) {
					problems = append(problems, fmt.Sprintf("%s: %q: %s: --%s needs a value", source, quote, target.CommandPath(), name))
				} else {
					i++
				}
			}
		case strings.HasPrefix(f, "-") && len(f) > 1:
			missing, attached, last := shorthandCluster(target, f)
			for _, s := range missing {
				problems = append(problems, fmt.Sprintf("%s: %q: %s has no -%s flag", source, quote, target.CommandPath(), s))
			}
			if flagTakesValue(last) && !attached {
				if i+1 >= len(args) {
					problems = append(problems, fmt.Sprintf("%s: %q: %s: -%s needs a value", source, quote, target.CommandPath(), string(last.Shorthand)))
				} else {
					i++
				}
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
			_, attached, last := shorthandCluster(cmd, a)
			if flagTakesValue(last) && !attached && i+1 < len(args) {
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
// Quotes are removed. A placeholder such as <key> is one word. A " #"
// comment is dropped only outside quotes. An unclosed quote is an error.
// A backslash is an error: these examples do not use shell escapes.
func splitExampleWords(line string) ([]string, error) {
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
		if c == '\\' {
			return nil, fmt.Errorf("backslash in example")
		}
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			b.WriteByte(c)
			inWord = true
			continue
		}
		if c == ' ' && i+1 < len(line) && line[i+1] == '#' {
			flush()
			return words, nil
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
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	return words, nil
}

// shorthandCluster checks each letter of a short-flag cluster. Once a letter
// takes a value, the rest of the token is that value (as in -d'{}' or -d={})
// and is not checked as more flags. attached is true when the value is in
// this token rather than the next word.
func shorthandCluster(cmd *cobra.Command, token string) (missing []string, attached bool, last *pflag.Flag) {
	raw := strings.TrimPrefix(token, "-")
	name, _, hasEq := strings.Cut(raw, "=")
	attached = hasEq
	for idx, letter := range name {
		s := string(letter)
		last = commandShorthand(cmd, s)
		if last == nil {
			missing = append(missing, s)
		}
		if flagTakesValue(last) {
			if idx < len(name)-1 {
				attached = true
			}
			break
		}
	}
	return missing, attached, last
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
	got, err := splitExampleWords(`cronitor site update my-site --data '{"name":"My Website","sampling":-1}' "two words"`)
	if err != nil {
		t.Fatal(err)
	}
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
	placeholders, err := splitExampleWords("cronitor issue update <key> --data '{}'")
	if err != nil {
		t.Fatal(err)
	}
	if len(placeholders) != 6 || placeholders[3] != "<key>" {
		t.Fatalf("placeholder split = %#v", placeholders)
	}

	commented, err := splitExampleWords("cronitor issue list --state unresolved # not a flag")
	if err != nil {
		t.Fatal(err)
	}
	if len(commented) != 5 || commented[4] != "unresolved" {
		t.Fatalf("comment outside quotes = %#v", commented)
	}
	quotedHash, err := splitExampleWords(`cronitor issue create --data '{"name":"a # b"}'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(quotedHash) != 5 || !strings.Contains(quotedHash[4], "a # b") {
		t.Fatalf("hash inside quotes = %#v", quotedHash)
	}
	if _, err := splitExampleWords(`cronitor issue create --data '{"name":`); err == nil || !strings.Contains(err.Error(), "unterminated quote") {
		t.Fatalf("unterminated quote err = %v", err)
	}
	if _, err := splitExampleWords(`cronitor issue list \`); err == nil || !strings.Contains(err.Error(), "backslash") {
		t.Fatalf("bare backslash err = %v", err)
	}
	if _, err := splitExampleWords(`cronitor issue create --data "a\nb"`); err == nil || !strings.Contains(err.Error(), "backslash") {
		t.Fatalf("backslash inside double quotes err = %v", err)
	}
}

func TestValueFlagRequiresValue(t *testing.T) {
	problems := exampleLineProblems("test", "cronitor site update x -d")
	found := false
	for _, p := range problems {
		if strings.Contains(p, "-d needs a value") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected missing -d value, got %#v", problems)
	}
}

func TestAttachedShorthandStopsAtValue(t *testing.T) {
	problems := exampleLineProblems("test", `cronitor issue create -d'{"name":"n"}'`)
	for _, p := range problems {
		if strings.Contains(p, "has no -") {
			t.Errorf("attached shorthand value was scanned as flags: %s", p)
		}
	}
	token, err := splitExampleWords(`-d'{}'`)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 1 || token[0] != "-d{}" {
		t.Fatalf("attached shorthand token = %#v", token)
	}
}

func TestHelpExamplesUseRealFlags(t *testing.T) {
	for _, cmd := range []*cobra.Command{authCmd, environmentCmd, notificationCmd, teamCmd, apiKeyCmd, issueCmd, maintenanceCmd, siteCmd, statuspageCmd, errorsCmd, logsCmd} {
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
	prefixes := []string{"cronitor auth ", "cronitor environment ", "cronitor notification ", "cronitor team ", "cronitor apikey ", "cronitor issue ", "cronitor maintenance ", "cronitor site ", "cronitor statuspage ", "cronitor errors ", "cronitor logs "}
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
