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
// A single quoted or escaped word is kept as written.
func formatWrappedCommand(command string) string {
	body, stdin := splitCronStdin(command)
	body = strings.TrimSpace(body)
	formatted := formatCommandBody(body)
	if formatted == "" {
		return stdin
	}
	return formatted + stdin
}

func formatCommandBody(body string) string {
	if body == "" {
		return ""
	}
	if words, err := shellquote.Split(body); err == nil && len(words) == 1 && body != words[0] && !commandIsComplex(words[0]) {
		return body
	}
	if commandIsComplex(body) {
		return shellquote.Join(body)
	}
	return strings.Join(strings.Fields(body), " ")
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
// flags are the flag tokens the MONITORIO marker cannot carry.
func unwrapCronitorExec(raw string) (code, command, prefix string, noStdout bool, flags []string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false, nil, false
	}

	var keyIndex int
	code, keyIndex, noStdout, flags, ok = detectCronitorWrap(strings.Fields(raw))
	if !ok {
		return "", "", "", false, nil, false
	}
	code = unquoteWord(code)
	prefix, tail := cutAfterFields(raw, keyIndex+1)
	return code, commandFromTail(tail), prefix, noStdout, flags, true
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
	// Cron cuts stdin at the first unescaped % before the shell parses the command.
	body, stdin := splitCronStdin(tail)
	body = strings.TrimSpace(body)
	if body == "" {
		return stdin
	}
	words, err := shellquote.Split(body)
	if err != nil || len(words) != 1 {
		return strings.Join(strings.Fields(body), " ") + stdin
	}
	// A complex word was quoted by us; store the inner command and re-quote on write.
	// Any other single quoted or escaped word stays as written.
	if !commandIsComplex(words[0]) && body != words[0] {
		return body + stdin
	}
	return words[0] + stdin
}

func detectCronitorWrap(words []string) (code string, keyIndex int, noStdout bool, flags []string, ok bool) {
	if len(words) < 3 || !isCronitorBinary(words[0]) {
		return "", 0, false, nil, false
	}

	i := 1
	for i < len(words) && words[i] != "exec" {
		tok := words[i]
		if !isCLIFlag(tok) {
			return "", 0, false, nil, false
		}
		flags = append(flags, tok)
		if tok == "--no-stdout" {
			noStdout = true
		}
		if flagConsumesNext(tok, words, i) {
			flags = append(flags, words[i+1])
			i += 2
			continue
		}
		i++
	}
	if i >= len(words) || words[i] != "exec" {
		return "", 0, false, nil, false
	}
	i++
	for i < len(words) && isExecArgFlag(words[i]) {
		tok := words[i]
		flags = append(flags, tok)
		if tok == "--no-stdout" {
			noStdout = true
		}
		i++
	}
	if i >= len(words) {
		return "", 0, false, nil, false
	}
	return words[i], i, noStdout, flags, true
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
		// The exec arg scanner treats the first "exec" as the subcommand, even
		// when it sits where a flag value would be (`--env exec exec realkey`).
		return i+1 < len(words) && words[i+1] != "exec"
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

// insertSyncFlags adds a missing --env before exec and puts --no-stdout after
// exec. An --env already on the line wins over CRONITOR_ENV. A --no-stdout
// that was before exec is moved after it, where the exec command accepts it.
func insertSyncFlags(prefix string, noStdout bool) string {
	words := strings.Fields(prefix)
	hasEnv := false
	var kept []string
	for _, word := range words {
		if word == "--env" || strings.HasPrefix(word, "--env=") {
			hasEnv = true
		}
		if word == "--no-stdout" {
			noStdout = true
			continue
		}
		kept = append(kept, word)
	}
	execAt := -1
	for i, word := range kept {
		if word == "exec" {
			execAt = i
			break
		}
	}
	if execAt < 0 {
		return prefix
	}
	out := append([]string{}, kept[:execAt]...)
	if env := viper.GetString("CRONITOR_ENV"); env != "" && !hasEnv {
		out = append(out, "--env", env)
	}
	out = append(out, "exec")
	if noStdout {
		out = append(out, "--no-stdout")
	}
	out = append(out, kept[execAt+1:]...)
	return strings.Join(out, " ")
}
