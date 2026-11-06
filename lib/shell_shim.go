package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kballard/go-shellquote"
	"github.com/spf13/viper"
)

const (
	MonitorPrefix        = "CRONITOR="      // written on new lines
	RealShellEnv         = "CRONITOR_SHELL" // the shell jobs run in; the wrapper hands every line to it
	ShimWrapperBase      = "shell"
	DefaultShimShellPath = "/etc/cronitor/shell"
)

type WriteMode int

const (
	WriteModeLegacy WriteMode = iota
	WriteModeShim
	WriteModeExec
)

const (
	IntegrationExec = "exec"
	IntegrationShim = "shim"
)

var ShimShellPathOverride string

// MonitorPrefixes are the markers sync and the wrapper accept.
var MonitorPrefixes = []string{MonitorPrefix, "MONITORIO="}

func markerPrefix(command string) string {
	for _, p := range MonitorPrefixes {
		if strings.HasPrefix(command, p) {
			return p
		}
	}
	return ""
}

func ParseMonitorMarker(command string) (key, rest string, ok bool) {
	prefix := markerPrefix(command)
	if prefix == "" {
		return "", "", false
	}
	body := command[len(prefix):]
	if body == "" || isPOSIXSpace(body[0]) {
		return "", "", false
	}
	i := 0
	for i < len(body) && !isPOSIXSpace(body[i]) {
		i++
	}
	if i == len(body) || !isPOSIXSpace(body[i]) {
		return "", "", false
	}
	key = body[:i]
	j := i
	for j < len(body) && isPOSIXSpace(body[j]) {
		j++
	}
	return key, body[j:], true
}

func isPOSIXSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	default:
		return false
	}
}

func formatShimCommand(line Line, code string) string {
	prefix := line.marker
	if prefix == "" {
		prefix = MonitorPrefix
	}
	return prefix + code + shimTail(line)
}

func shimTail(line Line) string {
	if line.Integration == IntegrationShim && line.rawTail != "" && line.CommandToRun == line.parsedCommand {
		return line.rawTail
	}
	parts := line.shimSettings
	if line.Integration == IntegrationExec {
		parts, _ = line.execSettings()
	}
	if cmd := shimJob(line); cmd != "" {
		parts = append(append([]string{}, parts...), cmd)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// shimJob is the job text without a marker: the bytes from the file when
// the command is unedited, otherwise CommandToRun (for example a dash edit).
// An exec line whose command is one quoted word gives the unquoted command.
func shimJob(line Line) string {
	if line.CommandToRun == line.parsedCommand {
		raw := strings.TrimLeft(line.rawTail, " \t\v\f\r\n")
		if line.Integration == IntegrationShim {
			_, raw = SplitShimSettings(raw)
		}
		if line.Integration == IntegrationExec {
			// cronitor exec runs a single argument as shell code. Cron turns \% into %
			// before the outer shell removes the quotes, so do the same, then escape
			// % again for the crontab.
			body, stdin := splitCronStdin(raw)
			if words, err := shellquote.Split(strings.ReplaceAll(body, `\%`, "%")); err == nil && len(words) == 1 {
				return strings.ReplaceAll(words[0], "%", `\%`) + stdin
			}
		}
		if raw != "" {
			return raw
		}
	}
	return line.CommandToRun
}

func shimExecCommand(line Line) string {
	cmd := shimJob(line)
	if cmd == "" {
		return ""
	}
	if commandIsComplex(cmd) {
		return formatWrappedCommand(cmd)
	}
	return cmd
}

// IsShimShellPath reports whether p is the wrapper sync installs: shell
// in a cronitor or .cronitor directory. The file name alone is too generic.
func IsShimShellPath(p string) bool {
	p = filepath.Clean(strings.TrimSpace(p))
	if filepath.Base(p) != ShimWrapperBase {
		return false
	}
	dir := filepath.Base(filepath.Dir(p))
	return dir == "cronitor" || dir == ".cronitor"
}

func (l Line) renderIntegration(c Crontab) string {
	if l.GetCode() == "" || !l.IsMonitorable() {
		return ""
	}
	if runtime.GOOS == "windows" {
		return IntegrationExec
	}
	if c.RewriteShimToExec && l.Integration == IntegrationShim {
		return IntegrationExec
	}
	switch c.WriteMode {
	case WriteModeExec:
		return IntegrationExec
	case WriteModeShim:
		if l.Integration == IntegrationExec {
			if _, ok := l.execSettings(); ok && c.ConvertExec {
				return IntegrationShim
			}
			return IntegrationExec
		}
		if l.Integration == IntegrationShim {
			return IntegrationShim
		}
		if c.BlockNewShim || l.Mon.NoStdoutPassthru || viper.GetString("CRONITOR_ENV") != "" {
			return IntegrationExec
		}
		return IntegrationShim
	default:
		if l.Integration == IntegrationShim {
			return IntegrationShim
		}
		return IntegrationExec
	}
}

func (c Crontab) EmitsShim() bool {
	for _, line := range c.Lines {
		if line.renderIntegration(c) == IntegrationShim {
			return true
		}
	}
	return false
}

func (c Crontab) ShimNotices() (stop bool, notices []string) {
	if runtime.GOOS == "windows" {
		return false, nil
	}
	if c.WriteMode != WriteModeExec {
		seenShim, users, after := false, 0, false
		for _, l := range c.Lines {
			if l == nil || !l.isManagedEnv() || l.GetEnvVarKey() != "SHELL" {
				continue
			}
			if IsShimShellPath(l.GetEnvVarValue()) {
				seenShim = true
				continue
			}
			users++
			if seenShim {
				after = true
			}
		}
		if after {
			notices = append(notices, "notice: a SHELL= line follows the cronitor shell shim; leaving SHELL lines unchanged and keeping exec style")
			if !c.RewriteShimToExec {
				return true, notices
			}
			stop = true
		} else if users > 1 {
			notices = append(notices, "notice: crontab has more than one SHELL= line; keeping exec style and not installing the shell shim")
			if !c.RewriteShimToExec {
				return true, notices
			}
			stop = true
		}
	}
	if c.ConvertExec && c.WriteMode == WriteModeShim {
		for _, line := range c.Lines {
			if line.Integration != IntegrationExec || line.GetCode() == "" {
				continue
			}
			if _, ok := line.execSettings(); !ok {
				notices = append(notices, fmt.Sprintf(
					"notice: monitor %s: kept as cronitor exec because the line has flags a CRONITOR= line cannot carry", line.GetCode()))
			}
		}
	}
	if c.RewriteShimToExec {
		for _, line := range c.Lines {
			if line.Integration != IntegrationShim || line.GetCode() == "" {
				continue
			}
			notices = append(notices, fmt.Sprintf(
				"notice: monitor %s: --no-stdout cannot be expressed on a shim marker line; rewritten as exec",
				line.GetCode()))
		}
	}
	return stop, notices
}

func (c Crontab) linesForWrite() []*Line {
	if runtime.GOOS == "windows" {
		return c.Lines
	}
	switch c.WriteMode {
	case WriteModeShim:
		if c.EmitsShim() {
			return c.withShimShell(c.shimPath())
		}
		if c.RewriteShimToExec {
			return c.withoutShimShell()
		}
		return c.Lines
	case WriteModeExec:
		return c.withoutShimShell()
	default:
		return c.Lines
	}
}

func (c Crontab) shimPath() string {
	if c.ShimShellPath != "" {
		return c.ShimShellPath
	}
	return DefaultShimShellPath
}

func (l *Line) isManagedEnv() bool {
	return l != nil && !l.IsJob && !l.IsComment && l.IsEnvVar()
}

func (c Crontab) withShimShell(wrapper string) []*Line {
	real := ""
	for _, l := range c.Lines {
		if !l.isManagedEnv() {
			continue
		}
		switch l.GetEnvVarKey() {
		case RealShellEnv:
			if real == "" {
				real = strings.TrimSpace(l.GetEnvVarValue())
			}
		case "SHELL":
			v := strings.TrimSpace(l.GetEnvVarValue())
			if v != "" && !IsShimShellPath(v) && real == "" {
				real = v
			}
		}
	}
	// CRONITOR_SHELL is written only when the crontab had its own shell. With
	// none, the wrapper uses /bin/sh, the same as cron.
	var out []*Line
	if real != "" {
		out = append(out, &Line{FullLine: RealShellEnv + "=" + real})
	}
	out = append(out, &Line{FullLine: "SHELL=" + wrapper})
	for _, l := range c.Lines {
		if l.isManagedEnv() {
			switch l.GetEnvVarKey() {
			case "SHELL", RealShellEnv:
				continue
			}
		}
		out = append(out, l)
	}
	return out
}

func (c Crontab) withoutShimShell() []*Line {
	shellIdx := -1
	realIdx := -1
	realVal := ""
	for i, l := range c.Lines {
		if !l.isManagedEnv() {
			continue
		}
		switch l.GetEnvVarKey() {
		case "SHELL":
			if IsShimShellPath(l.GetEnvVarValue()) {
				shellIdx = i
			}
		case RealShellEnv:
			realIdx = i
			realVal = strings.TrimSpace(l.GetEnvVarValue())
		}
	}
	if shellIdx < 0 {
		return c.Lines
	}
	out := make([]*Line, 0, len(c.Lines))
	for i, l := range c.Lines {
		if i == realIdx {
			continue
		}
		if i == shellIdx {
			if realVal == "" {
				continue // the crontab had no SHELL line before sync
			}
			cp := *l
			cp.FullLine = "SHELL=" + realVal
			out = append(out, &cp)
			continue
		}
		out = append(out, l)
	}
	return out
}

var shimEuid = os.Geteuid

func ResolveShimInstallPath() string {
	if ShimShellPathOverride != "" {
		return ShimShellPathOverride
	}
	if shimEuid() == 0 {
		return DefaultShimShellPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cronitor", ShimWrapperBase)
	}
	return filepath.Join(home, ".cronitor", ShimWrapperBase)
}

func InstallShimWrapper(dest, cronitorBin string) (string, error) {
	if dest == "" {
		dest = ResolveShimInstallPath()
	}
	if strings.HasPrefix(dest, "/etc/") && !filepath.IsAbs(cronitorBin) {
		return "", fmt.Errorf("refusing to install %s: cronitor path %q is relative", dest, cronitorBin)
	}
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "."+ShimWrapperBase+".*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	_, werr := tmp.WriteString(renderShimWrapper(cronitorBin))
	cerr := tmp.Chmod(0755)
	tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(name)
		if werr != nil {
			return "", werr
		}
		return "", cerr
	}
	if err := os.Rename(name, dest); err != nil {
		os.Remove(name)
		return "", err
	}
	return dest, nil
}

func renderShimWrapper(cronitorBin string) string {
	quoted := strings.ReplaceAll(cronitorBin, `'`, `'\''`)
	return strings.ReplaceAll(shimWrapperTemplate, "__CRONITOR_BIN__", quoted)
}

const shimWrapperTemplate = `#!/bin/sh
set -f
CRONITOR_BIN='__CRONITOR_BIN__'
REAL_SHELL=${CRONITOR_SHELL:-/bin/sh}
case $REAL_SHELL in
  */cronitor/shell|*/.cronitor/shell) REAL_SHELL=/bin/sh ;;
esac
export SHELL="$REAL_SHELL"
if [ "$#" -ne 2 ] || [ "$1" != "-c" ]; then
  exec "$REAL_SHELL" "$@"
fi
cmd=$2
case $cmd in
  CRONITOR=*) rest=${cmd#CRONITOR=} ;;
  MONITORIO=*) rest=${cmd#MONITORIO=} ;;
  *) exec "$REAL_SHELL" "$@" ;;
esac
key=${rest%%[[:space:]]*}
after=${rest#"$key"}
case $after in
  [[:space:]]*) ;;
  *) exec "$REAL_SHELL" "$@" ;;
esac
if [ -z "$key" ]; then
  exec "$REAL_SHELL" "$@"
fi
trim() {
  t=$1
  while :; do
    case $t in
      [[:space:]]*) t=${t#?} ;;
      *) break ;;
    esac
  done
}
trim "$after"; after=$t
# Settings after the key become cronitor exec flags. Value flags go before
# exec, as --flag=value, because exec reads the first word after it as the key.
set --
nostdout=
while :; do
  word=${after%%[[:space:]]*}
  next=${after#"$word"}
  case $next in [[:space:]]*) ;; *) break ;; esac
  trim "$next"
  [ -n "$t" ] || break
  case $word in *=*) ;; *) break ;; esac
  name=${word%%=*}
  value=${word#*=}
  case $value in ''|*[!A-Za-z0-9._:@/+-]*) break ;; esac
  case $name in
    CRONITOR_NO_STDOUT) [ "$value" = 1 ] && nostdout=1 ;;
    CRONITOR_ENV) set -- "$@" "--env=$value" ;;
    CRONITOR_API_KEY) set -- "$@" "--api-key=$value" ;;
    CRONITOR_PING_API_KEY) set -- "$@" "--ping-api-key=$value" ;;
    CRONITOR_HOSTNAME) set -- "$@" "--hostname=$value" ;;
    *) break ;;
  esac
  after=$t
done
if [ -x "$CRONITOR_BIN" ]; then
  exec "$CRONITOR_BIN" "$@" exec ${nostdout:+--no-stdout} "$key" "$after"
fi
exec "$REAL_SHELL" -c "$after"
`
