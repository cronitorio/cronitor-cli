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
	MonitorPrefix        = "MONITORIO="
	RealShellEnv         = "CRONITOR_REAL_SHELL"
	DefaultRealShell     = "/bin/sh"
	ShimWrapperBase      = "cronitor-shell"
	DefaultShimShellPath = "/etc/cronitor/cronitor-shell"
)

type WriteMode int

const (
	WriteModeLegacy WriteMode = iota
	WriteModeShim
	WriteModeExec
	WriteModeConvertToShim
)

const (
	IntegrationExec = "exec"
	IntegrationShim = "shim"
)

// ShimShellPathOverride is the wrapper path tests install instead of /etc/cronitor.
var ShimShellPathOverride string

func ParseMonitorMarker(command string) (key, rest string, ok bool) {
	if !strings.HasPrefix(command, MonitorPrefix) {
		return "", "", false
	}
	body := command[len(MonitorPrefix):]
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

func ResolveRealShell(envValue string) string {
	shell := strings.TrimSpace(envValue)
	if shell == "" || IsShimShellPath(shell) {
		return DefaultRealShell
	}
	return shell
}

func formatShimCommand(line Line, code string) string {
	tail := shimTail(line)
	if tail == "" {
		return MonitorPrefix + code
	}
	return MonitorPrefix + code + tail
}

func shimTail(line Line) string {
	if line.Integration == IntegrationShim && line.rawTail != "" {
		return line.rawTail
	}
	cmd := line.CommandToRun
	if line.Integration == IntegrationExec {
		if unquoted := unquotedExecCommand(line.rawTail, line.CommandToRun); unquoted != "" {
			cmd = unquoted
		}
	} else if raw := strings.TrimLeft(line.rawTail, " \t\v\f\r\n"); raw != "" {
		cmd = raw
	}
	if cmd == "" {
		return ""
	}
	return " " + cmd
}

func unquotedExecCommand(rawTail, command string) string {
	raw := strings.TrimSpace(rawTail)
	words, err := shellquote.Split(raw)
	if err == nil && len(words) == 1 && words[0] == command {
		return command
	}
	if raw != "" {
		return raw
	}
	return command
}

func shimExecCommand(line Line) string {
	cmd := strings.TrimLeft(line.rawTail, " \t\v\f\r\n")
	if cmd == "" {
		cmd = line.CommandToRun
	}
	if cmd == "" {
		return ""
	}
	if commandIsComplex(cmd) {
		return formatWrappedCommand(cmd)
	}
	return cmd
}

func preservedRawCommand(line Line) string {
	raw := line.rawCommand
	if line.wrapPrefix != "" || line.Integration != "" || raw == "" || raw == line.CommandToRun {
		return ""
	}
	if commandIsComplex(raw) || !strings.ContainsAny(raw, `"'`) {
		return ""
	}
	return raw
}

func IsShimShellPath(p string) bool {
	return filepath.Base(strings.TrimSpace(p)) == ShimWrapperBase
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
	case WriteModeConvertToShim:
		if l.Integration == IntegrationExec && len(l.execFlags) > 0 {
			return IntegrationExec
		}
		return IntegrationShim
	case WriteModeShim:
		if l.Integration == IntegrationExec {
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
	if c.WriteMode == WriteModeConvertToShim {
		for _, line := range c.Lines {
			if line.Integration != IntegrationExec || line.GetCode() == "" || len(line.execFlags) == 0 {
				continue
			}
			notices = append(notices, fmt.Sprintf(
				"notice: monitor %s: skipped --convert-to-shim; flags cannot be expressed as MONITORIO=<key> (%s); left as exec",
				line.GetCode(), redactExecFlags(line.execFlags)))
		}
	}
	if c.RewriteShimToExec {
		for _, line := range c.Lines {
			if line.Integration != IntegrationShim || line.GetCode() == "" {
				continue
			}
			notices = append(notices, fmt.Sprintf(
				"notice: monitor %s: --no-stdout cannot be expressed on a MONITORIO line; rewritten as exec",
				line.GetCode()))
		}
	}
	return stop, notices
}

func redactExecFlags(flags []string) string {
	out := make([]string, 0, len(flags))
	for i := 0; i < len(flags); i++ {
		tok := flags[i]
		if strings.HasPrefix(tok, "--api-key=") {
			out = append(out, "--api-key=<redacted>")
			continue
		}
		if strings.HasPrefix(tok, "-k") && !strings.HasPrefix(tok, "--") && len(tok) > 2 {
			out = append(out, "-k<redacted>")
			continue
		}
		out = append(out, tok)
		if (tok == "--api-key" || tok == "-k") && i+1 < len(flags) {
			i++
			out = append(out, "<redacted>")
		}
	}
	return strings.Join(out, " ")
}

func (c Crontab) linesForWrite() []*Line {
	if runtime.GOOS == "windows" {
		return c.Lines
	}
	switch c.WriteMode {
	case WriteModeShim, WriteModeConvertToShim:
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
	if real == "" {
		real = DefaultRealShell
	}
	out := []*Line{
		{FullLine: RealShellEnv + "=" + real},
		{FullLine: "SHELL=" + wrapper},
	}
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
	if realVal == "" {
		realVal = DefaultRealShell
	}
	out := make([]*Line, 0, len(c.Lines))
	for i, l := range c.Lines {
		if i == realIdx {
			continue
		}
		if i == shellIdx {
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
REAL_SHELL=${CRONITOR_REAL_SHELL:-/bin/sh}
case $REAL_SHELL in
  */cronitor-shell|cronitor-shell) REAL_SHELL=/bin/sh ;;
esac
export SHELL="$REAL_SHELL"

if [ "$#" -ne 2 ] || [ "$1" != "-c" ]; then
  exec "$REAL_SHELL" "$@"
fi
cmd=$2
case $cmd in
  MONITORIO=*) ;;
  *) exec "$REAL_SHELL" "$@" ;;
esac
rest=${cmd#MONITORIO=}
key=${rest%%[[:space:]]*}
after=${rest#"$key"}
case $after in
  [[:space:]]*) ;;
  *) exec "$REAL_SHELL" "$@" ;;
esac
if [ -z "$key" ]; then
  exec "$REAL_SHELL" "$@"
fi
while :; do
  case $after in
    [[:space:]]*) after=${after#?} ;;
    *) break ;;
  esac
done
if [ ! -x "$CRONITOR_BIN" ]; then
  exec "$REAL_SHELL" -c "$after"
fi

# An open fd 3 belongs to the caller. Run once, unmonitored, and do not touch fds.
if ( : <&3 ) 2>/dev/null || ( : >&3 ) 2>/dev/null; then
  exec "$REAL_SHELL" -c "$after"
fi
exec 4<&0
hs=$(mktemp -d "${TMPDIR:-/tmp}/cronitor-shim.XXXXXX") || exec "$REAL_SHELL" -c "$after"
fifo=$hs/started
if ! mkfifo "$fifo"; then
  rm -rf "$hs"
  exec "$REAL_SHELL" -c "$after"
fi
(
  exec </dev/null >/dev/null 2>&1
  if read byte <"$fifo"; then
    case $byte in
      1) printf '1\n' >"$hs/byte" ;;
    esac
  fi
) &
reader=$!
CRONITOR_SHIM_FD=3 "$CRONITOR_BIN" shell-shim -c "$cmd" 3>"$fifo" <&4 4<&- &
child=$!
exec 4<&-
(
  exec </dev/null >/dev/null 2>&1
  sleep 3 &
  sp=$!
  trap 'kill "$sp" 2>/dev/null; exit 0' TERM INT HUP
  wait "$sp"
  : >"$hs/timeout"
) &
watch=$!
trap 'rm -rf "$hs"; kill -TERM "$child" 2>/dev/null; kill "$reader" "$watch" 2>/dev/null; wait "$child"; exit $?' TERM
trap 'rm -rf "$hs"; kill -INT "$child" 2>/dev/null; i=0; while kill -0 "$child" 2>/dev/null && [ "$i" -lt 20 ]; do i=$((i+1)); sleep 0.05; done; kill -KILL "$child" 2>/dev/null; kill "$reader" "$watch" 2>/dev/null; wait "$child"; exit $?' INT
trap 'rm -rf "$hs"; kill -HUP "$child" 2>/dev/null; kill "$reader" "$watch" 2>/dev/null; wait "$child"; exit $?' HUP
while [ ! -s "$hs/byte" ] && [ ! -f "$hs/timeout" ]; do
  kill -0 "$child" 2>/dev/null || break
  sleep 0.05
done
kill "$watch" 2>/dev/null
wait "$watch" 2>/dev/null
if [ -s "$hs/byte" ]; then
  kill "$reader" 2>/dev/null
  wait "$reader" 2>/dev/null
  rm -rf "$hs"
  wait "$child"
  exit $?
fi
kill -KILL "$child" 2>/dev/null
wait "$child"
status=$?
wait "$reader" 2>/dev/null
if [ -s "$hs/byte" ]; then
  rm -rf "$hs"
  exit "$status"
fi
rm -rf "$hs"
exec "$REAL_SHELL" -c "$after"
`
