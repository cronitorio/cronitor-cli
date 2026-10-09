package lib

import (
	"bytes"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/kballard/go-shellquote"
)

// commandIsComplex reports whether the shell, not cronitor, would interpret
// control operators if the command were left unquoted.
func commandIsComplex(command string) bool {
	return strings.Contains(command, ";") || strings.Contains(command, "|") || strings.Contains(command, "&&") || strings.Contains(command, "||")
}

// formatWrappedCommand renders CommandToRun as the trailing token of a
// `cronitor exec` line. Complex commands are one shell word so the outer shell
// does not split on ; | && ||. Every % is written as \% because cron treats an
// unescaped % as newline-plus-stdin.
func formatWrappedCommand(command string) string {
	var formatted string
	if commandIsComplex(command) {
		formatted = shellquote.Join(command)
	} else {
		formatted = strings.Join(strings.Fields(command), " ")
	}
	return strings.ReplaceAll(formatted, "%", "\\%")
}

// skipWSFields returns the remainder of s after n whitespace-separated fields.
// Cron schedule fields are unquoted, so this does not have to understand shell quotes.
func skipWSFields(s string, n int) string {
	i := 0
	for field := 0; field < n && i < len(s); field++ {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}

// Persistent flags that take a value. Bool flags and anything unknown use a
// heuristic so a future flag still doesn't swallow `exec` or the monitor key.
var cronitorValueFlags = map[string]bool{
	"config": true, "c": true,
	"env":     true,
	"api-key": true, "k": true,
	"ping-api-key": true, "p": true,
	"ping-api-host": true,
	"hostname":      true, "n": true,
	"log": true, "l": true,
	"users": true, "u": true,
	"api-version": true,
}

var cronitorBoolFlags = map[string]bool{
	"verbose": true, "v": true,
	"use-dev":   true,
	"no-stdout": true,
	"help":      true, "h": true,
}

// unwrapCronitorExec reports whether raw is `cronitor [flags...] exec <key> [command]`.
// The binary may be a path (…/cronitor, ./cronitor, cronitor.exe). noStdout is set
// when --no-stdout was present so a later Write emits the same flag.
func unwrapCronitorExec(raw string) (code, command string, noStdout, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false, false
	}

	// Cron stores a literal % as \%. Undo that before shell splitting so the
	// command we keep is the one the shell runs, and Write can escape it again.
	logical := strings.ReplaceAll(raw, "\\%", "%")
	words, ends, err := splitShellWords(logical)
	if err != nil {
		code, cmdWords, _, noStdout, found := detectCronitorWrap(strings.Fields(raw))
		if !found {
			return "", "", false, false
		}
		return code, strings.Join(cmdWords, " "), noStdout, true
	}

	code, cmdWords, keyIndex, noStdout, found := detectCronitorWrap(words)
	if !found {
		return "", "", false, false
	}
	if len(cmdWords) == 0 {
		return code, "", noStdout, true
	}
	// A single shell word is how complex commands are quoted. Multiple words
	// keep the raw tail so quote characters that are part of a simple command
	// (echo "hi there") survive.
	if len(cmdWords) == 1 {
		return code, cmdWords[0], noStdout, true
	}
	if keyIndex < len(ends) {
		tail := strings.TrimSpace(logical[ends[keyIndex]:])
		return code, strings.Join(strings.Fields(tail), " "), noStdout, true
	}
	return code, strings.Join(cmdWords, " "), noStdout, true
}

func detectCronitorWrap(words []string) (code string, commandWords []string, keyIndex int, noStdout, ok bool) {
	if len(words) < 3 || !isCronitorBinary(words[0]) {
		return "", nil, 0, false, false
	}

	i := 1
	for i < len(words) {
		tok := words[i]
		if tok == "exec" {
			if i+1 >= len(words) {
				return "", nil, 0, false, false
			}
			return words[i+1], words[i+2:], i + 1, noStdout, true
		}
		if !isCLIFlag(tok) {
			return "", nil, 0, false, false
		}
		if tok == "--no-stdout" {
			noStdout = true
		}
		if flagConsumesNext(tok, words, i) {
			i += 2
			continue
		}
		i++
	}
	return "", nil, 0, false, false
}

func isCLIFlag(tok string) bool {
	return strings.HasPrefix(tok, "-") && tok != "-" && tok != "--"
}

func flagConsumesNext(tok string, words []string, i int) bool {
	if strings.Contains(tok, "=") {
		return false
	}
	// Attached short option, e.g. -c/path/to/cfg.
	if strings.HasPrefix(tok, "-") && !strings.HasPrefix(tok, "--") && len(tok) > 2 {
		return false
	}
	name := strings.TrimLeft(tok, "-")
	if cronitorBoolFlags[name] {
		return false
	}
	if cronitorValueFlags[name] {
		return i+1 < len(words)
	}
	if i+1 >= len(words) {
		return false
	}
	next := words[i+1]
	if next == "exec" || isCLIFlag(next) || next == "--" {
		return false
	}
	return true
}

func isCronitorBinary(token string) bool {
	normalized := strings.ReplaceAll(token, `\`, "/")
	base := strings.ToLower(path.Base(normalized))
	return base == "cronitor" || base == "cronitor.exe"
}

// splitShellWords splits s with /bin/sh word rules and returns the byte offset
// just after each word (a following delimiter, if any, is included). Word text
// matches shellquote.Split; the offsets are how we recover a multi-word tail.
func splitShellWords(input string) (words []string, endOffsets []int, err error) {
	original := input
	var buf bytes.Buffer
	for len(input) > 0 {
		c, l := utf8.DecodeRuneInString(input)
		if strings.ContainsRune(" \n\t", c) {
			input = input[l:]
			continue
		} else if c == '\\' {
			next := input[l:]
			if len(next) == 0 {
				return nil, nil, shellquote.UnterminatedEscapeError
			}
			c2, l2 := utf8.DecodeRuneInString(next)
			if c2 == '\n' {
				input = next[l2:]
				continue
			}
		}

		var word string
		word, input, err = splitShellWord(input, &buf)
		if err != nil {
			return nil, nil, err
		}
		words = append(words, word)
		endOffsets = append(endOffsets, len(original)-len(input))
	}
	return words, endOffsets, nil
}

// splitShellWord is the shellquote word scanner, plus the unconsumed remainder,
// so callers can slice the original string. Behavior matches shellquote.Split.
func splitShellWord(input string, buf *bytes.Buffer) (word string, remainder string, err error) {
	buf.Reset()

raw:
	{
		cur := input
		for len(cur) > 0 {
			c, l := utf8.DecodeRuneInString(cur)
			cur = cur[l:]
			if c == '\'' {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto single
			} else if c == '"' {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto double
			} else if c == '\\' {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto escape
			} else if strings.ContainsRune(" \n\t", c) {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				return buf.String(), cur, nil
			}
		}
		if len(input) > 0 {
			buf.WriteString(input)
			input = ""
		}
		goto done
	}

escape:
	{
		if len(input) == 0 {
			return "", "", shellquote.UnterminatedEscapeError
		}
		c, l := utf8.DecodeRuneInString(input)
		if c == '\n' {
			// a backslash-escaped newline is elided from the output entirely
		} else {
			buf.WriteString(input[:l])
		}
		input = input[l:]
	}
	goto raw

single:
	{
		i := strings.IndexRune(input, '\'')
		if i == -1 {
			return "", "", shellquote.UnterminatedSingleQuoteError
		}
		buf.WriteString(input[0:i])
		input = input[i+1:]
		goto raw
	}

double:
	{
		cur := input
		for len(cur) > 0 {
			c, l := utf8.DecodeRuneInString(cur)
			cur = cur[l:]
			if c == '"' {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto raw
			} else if c == '\\' {
				c2, l2 := utf8.DecodeRuneInString(cur)
				cur = cur[l2:]
				if strings.ContainsRune("$`\"\n\\", c2) {
					buf.WriteString(input[0 : len(input)-len(cur)-l-l2])
					if c2 == '\n' {
						// newline is special, skip the backslash entirely
					} else {
						buf.WriteRune(c2)
					}
					input = cur
				}
			}
		}
		return "", "", shellquote.UnterminatedDoubleQuoteError
	}

done:
	return buf.String(), input, nil
}
