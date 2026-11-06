package lib

import "strings"

// Settings a marker line can carry after CRONITOR=<key>, for example
// `CRONITOR=abc CRONITOR_ENV=staging ./job.sh`. Each one stands for an exec
// flag. The shell wrapper passes them to cronitor exec as those flags.
var shimSettingNames = map[string]bool{
	"CRONITOR_NO_STDOUT":    true,
	"CRONITOR_ENV":          true,
	"CRONITOR_API_KEY":      true,
	"CRONITOR_PING_API_KEY": true,
	"CRONITOR_HOSTNAME":     true,
}

// execFlagSettings maps exec value flags to their setting. --no-stdout is
// handled on its own. Other flags have no setting, so the line stays exec.
var execFlagSettings = map[string]string{
	"env":          "CRONITOR_ENV",
	"api-key":      "CRONITOR_API_KEY",
	"k":            "CRONITOR_API_KEY",
	"ping-api-key": "CRONITOR_PING_API_KEY",
	"p":            "CRONITOR_PING_API_KEY",
	"hostname":     "CRONITOR_HOSTNAME",
	"n":            "CRONITOR_HOSTNAME",
}

// simpleSettingValue accepts values that need no shell quoting.
func simpleSettingValue(v string) bool {
	if v == "" {
		return false
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("._:@/+-", c):
		default:
			return false
		}
	}
	return true
}

// SplitShimSettings removes leading settings from the command after the
// marker. A setting must be followed by a command.
func SplitShimSettings(rest string) (settings []string, job string) {
	s := rest
	for {
		i := 0
		for i < len(s) && !isPOSIXSpace(s[i]) {
			i++
		}
		word := s[:i]
		after := strings.TrimLeft(s[i:], " \t\n\v\f\r")
		name, value, ok := strings.Cut(word, "=")
		if !ok || !shimSettingNames[name] || !simpleSettingValue(value) || after == "" {
			break
		}
		settings = append(settings, word)
		s = after
	}
	if len(settings) == 0 {
		return nil, rest
	}
	return settings, s
}

// execSettings returns the settings for the flags on an exec line. ok is
// false when a flag has no setting or its value needs quoting.
func (l Line) execSettings() (settings []string, ok bool) {
	words := strings.Fields(l.wrapPrefix) // <binary> [flags] exec [flags] <key>
	if len(words) < 3 {
		return nil, false
	}
	last := len(words) - 1
	for i := 1; i < last; i++ {
		tok := words[i]
		if tok == "exec" {
			continue
		}
		if tok == "--no-stdout" {
			settings = append(settings, "CRONITOR_NO_STDOUT=1")
			continue
		}
		var name, value string
		switch {
		case strings.HasPrefix(tok, "--"):
			n, v, hasValue := strings.Cut(tok[2:], "=")
			name, value = n, v
			if !hasValue && i+1 < last {
				i++
				value = words[i]
			}
		case len(tok) >= 2 && tok[0] == '-':
			name, value = tok[1:2], tok[2:]
			if value == "" && i+1 < last {
				i++
				value = words[i]
			}
		default:
			return nil, false
		}
		setting, known := execFlagSettings[name]
		if !known || !simpleSettingValue(value) {
			return nil, false
		}
		settings = append(settings, setting+"="+value)
	}
	return settings, true
}

var settingFlagNames = map[string]string{
	"CRONITOR_ENV":          "--env",
	"CRONITOR_API_KEY":      "--api-key",
	"CRONITOR_PING_API_KEY": "--ping-api-key",
	"CRONITOR_HOSTNAME":     "--hostname",
}

// settingFlags turns settings back into root flags for an exec line.
// CRONITOR_NO_STDOUT is written as exec --no-stdout from Mon.NoStdoutPassthru.
func settingFlags(settings []string) []string {
	var flags []string
	for _, s := range settings {
		name, value, _ := strings.Cut(s, "=")
		if flag, ok := settingFlagNames[name]; ok {
			flags = append(flags, flag, value)
		}
	}
	return flags
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
