package lib

import (
	"path"
	"strings"

	"github.com/kballard/go-shellquote"
	"github.com/spf13/viper"
)

func commandIsComplex(command string) bool {
	return strings.Contains(command, ";") || strings.Contains(command, "|") || strings.Contains(command, "&&") || strings.Contains(command, "||")
}

// formatWrappedCommand quotes a complex command as one shell word. A bare %
// starts cron stdin and is left outside the quotes; \% is left as written.
func formatWrappedCommand(command string) string {
	body, stdin := splitCronStdin(command)
	var formatted string
	if body != "" && commandIsComplex(body) {
		formatted = shellquote.Join(body)
	} else {
		formatted = strings.Join(strings.Fields(body), " ")
	}
	if formatted == "" {
		return stdin
	}
	return formatted + stdin
}

// splitCronStdin cuts at the first % cronie would treat as stdin. A backslash
// escapes the next byte, so \% stays in the command and \\% does not.
func splitCronStdin(command string) (body, stdin string) {
	escaped := false
	for i := 0; i < len(command); i++ {
		if escaped {
			escaped = false
			continue
		}
		if command[i] == '\\' {
			escaped = true
			continue
		}
		if command[i] == '%' {
			return command[:i], command[i:]
		}
	}
	return command, ""
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
	prefix = s[:i]
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

	var keyIndex int
	code, keyIndex, noStdout, ok = detectCronitorWrap(strings.Fields(raw))
	if !ok {
		return "", "", "", false, false
	}
	code = unquoteWord(code)
	prefix, tail := cutAfterFields(raw, keyIndex+1)
	return code, commandFromTail(tail), prefix, noStdout, true
}

func unquoteWord(tok string) string {
	if len(tok) >= 2 && ((tok[0] == '\'' && tok[len(tok)-1] == '\'') || (tok[0] == '"' && tok[len(tok)-1] == '"')) {
		return tok[1 : len(tok)-1]
	}
	return tok
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

// replacePrefixKey swaps the monitor key, the last field of a saved prefix.
func replacePrefixKey(prefix, code string) string {
	n := len(strings.Fields(prefix))
	if n == 0 {
		return code
	}
	head, _ := cutAfterFields(prefix, n-1)
	if head == "" {
		return code
	}
	return head + " " + code
}

// mergeSyncFlags adds the current sync --env and --no-stdout when the prefix
// does not already have them.
func mergeSyncFlags(prefix string, noStdout bool) string {
	env := viper.GetString("CRONITOR_ENV")
	var insert []string
	if env != "" && !prefixHasFlag(prefix, "--env") {
		insert = append(insert, "--env", env)
	}
	if noStdout && !prefixHasFlag(prefix, "--no-stdout") {
		insert = append(insert, "--no-stdout")
	}
	if len(insert) == 0 {
		return prefix
	}
	at := execTokenStart(prefix)
	if at < 0 {
		return prefix
	}
	return strings.TrimRight(prefix[:at], " \t") + " " + strings.Join(insert, " ") + " " + prefix[at:]
}

func prefixHasFlag(prefix, flag string) bool {
	for _, word := range strings.Fields(prefix) {
		if word == flag || strings.HasPrefix(word, flag+"=") {
			return true
		}
	}
	return false
}

func execTokenStart(prefix string) int {
	i := 0
	for i < len(prefix) {
		for i < len(prefix) && (prefix[i] == ' ' || prefix[i] == '\t') {
			i++
		}
		start := i
		for i < len(prefix) && prefix[i] != ' ' && prefix[i] != '\t' {
			i++
		}
		if start == i {
			return -1
		}
		if prefix[start:i] == "exec" {
			return start
		}
	}
	return -1
}
