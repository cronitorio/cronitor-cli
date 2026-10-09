package lib

import (
	"path"
	"strings"

	"github.com/kballard/go-shellquote"
)

func commandIsComplex(command string) bool {
	return strings.Contains(command, ";") || strings.Contains(command, "|") || strings.Contains(command, "&&") || strings.Contains(command, "||")
}

// formatWrappedCommand quotes complex commands as one shell word and writes % as \%.
func formatWrappedCommand(command string) string {
	var formatted string
	if commandIsComplex(command) {
		formatted = shellquote.Join(command)
	} else {
		formatted = strings.Join(strings.Fields(command), " ")
	}
	return escapeCronPercents(formatted)
}

func escapeCronPercents(s string) string {
	return strings.ReplaceAll(s, "%", "\\%")
}

func unescapeCronPercents(s string) string {
	return strings.ReplaceAll(s, "\\%", "%")
}

func skipWSFields(s string, n int) string {
	_, tail := cutAfterFields(s, n)
	return tail
}

func cutAfterFields(s string, n int) (prefix, tail string) {
	i := 0
	for field := 0; field < n && i < len(s); field++ {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		for i < len(s) && s[i] != ' ' && s[i] != '\t' {
			i++
		}
	}
	prefix = strings.TrimRight(s[:i], " \t")
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return prefix, s[i:]
}

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

// unwrapCronitorExec parses `cronitor [flags] exec [flags] <key> [command]`.
// prefix is the original text through the key so Write can emit it unchanged.
func unwrapCronitorExec(raw string) (code, command, prefix string, noStdout, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false, false
	}

	logical := unescapeCronPercents(raw)
	var keyIndex int
	code, keyIndex, noStdout, ok = detectCronitorWrap(strings.Fields(logical))
	if !ok {
		return "", "", "", false, false
	}
	prefix, tail := cutAfterFields(raw, keyIndex+1)
	return code, commandFromTail(unescapeCronPercents(tail)), prefix, noStdout, true
}

func commandFromTail(tail string) string {
	tail = strings.TrimSpace(tail)
	if tail == "" {
		return ""
	}
	words, err := shellquote.Split(tail)
	if err != nil || len(words) != 1 {
		return strings.Join(strings.Fields(tail), " ")
	}
	return words[0]
}

func detectCronitorWrap(words []string) (code string, keyIndex int, noStdout, ok bool) {
	if len(words) < 3 || !isCronitorBinary(words[0]) {
		return "", 0, false, false
	}

	i := 1
	for i < len(words) && words[i] != "exec" {
		tok := words[i]
		if !isCLIFlag(tok) {
			return "", 0, false, false
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
	if i >= len(words) || words[i] != "exec" {
		return "", 0, false, false
	}
	i++
	for i < len(words) && isExecArgFlag(words[i]) {
		if words[i] == "--no-stdout" {
			noStdout = true
		}
		i++
	}
	if i >= len(words) {
		return "", 0, false, false
	}
	return words[i], i, noStdout, true
}

func isExecArgFlag(tok string) bool {
	if tok == "--" || tok == "-" {
		return true
	}
	if strings.Contains(tok, "=") || !isCLIFlag(tok) {
		return false
	}
	name := strings.TrimPrefix(tok, "--")
	if name == tok {
		name = strings.TrimPrefix(tok, "-")
	}
	return cronitorBoolFlags[name] || cronitorValueFlags[name]
}

func isCLIFlag(tok string) bool {
	return strings.HasPrefix(tok, "-") && tok != "-" && tok != "--"
}

func flagConsumesNext(tok string, words []string, i int) bool {
	if strings.Contains(tok, "=") {
		return false
	}
	// -c/path is one token and must not consume the next word.
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
	base := strings.ToLower(path.Base(strings.ReplaceAll(token, `\`, "/")))
	return base == "cronitor" || base == "cronitor.exe"
}
