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

// pathWritableFn is replaced on Unix. The Windows default is false.
var pathWritableFn = func(string) bool { return false }

func pathWritable(path string) bool { return pathWritableFn(path) }

const (
	// MonitorPrefix is the per-line marker. The wrapper invokes cronitor only
	// when the command field starts with MONITORIO=<key> and whitespace.
	MonitorPrefix = "MONITORIO="
	// RealShellEnv is the crontab SHELL= from before the wrapper was installed.
	RealShellEnv         = "CRONITOR_REAL_SHELL"
	DefaultRealShell     = "/bin/sh"
	ShimWrapperBase      = "cronitor-shell"
	DefaultShimShellPath = "/etc/cronitor/cronitor-shell"
)

// WriteMode selects how monitored lines are rendered. Zero keeps exec lines.
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

// ShimShellPathOverride, when set, is the wrapper path sync installs.
// Tests use it so sync does not touch /etc/cronitor.
var ShimShellPathOverride string

// ParseMonitorMarker matches a leading MONITORIO=<key> only when whitespace
// follows the key. The command text after that is returned unchanged.
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

// ResolveRealShell returns the shell to exec, including /bin/sh.
// A path that is this wrapper falls back to /bin/sh.
func ResolveRealShell(envValue string) string {
	shell := strings.TrimSpace(envValue)
	if shell == "" || IsShimShellPath(shell) {
		return DefaultRealShell
	}
	return shell
}

// formatShimCommand renders MONITORIO=<key> plus the original command bytes.
// An existing shim line's rawTail already includes the separator.
func formatShimCommand(line Line, code string) string {
	tail := shimTail(line)
	if tail == "" {
		return MonitorPrefix + code
	}
	return MonitorPrefix + code + tail
}

// shimTail is the bytes after MONITORIO=<key>. An existing shim line keeps
// rawTail, separator included. An exec line uses CommandToRun when that is
// the unquoted single argument #65 stored for a compound command.
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

// shimExecCommand is the job text for a MONITORIO line rewritten as exec.
// A compound command is one quoted argument, matching #65. Anything else
// keeps the original bytes.
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

// preservedRawCommand keeps a quoted command Fields would split, so dash
// re-enable does not rebuild "/opt/my app/run.sh".
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

// IsShimShellPath reports whether p is the fall-through wrapper.
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

// EmitsShim reports whether Write will emit at least one MONITORIO line.
func (c Crontab) EmitsShim() bool {
	for _, line := range c.Lines {
		if line.renderIntegration(c) == IntegrationShim {
			return true
		}
	}
	return false
}

// ShimShellNotice reports a SHELL layout the shim must not rewrite.
// A SHELL= after the wrapper, or more than one real SHELL=, stays as written.
func (c Crontab) ShimShellNotice() string {
	seenShim := false
	users := 0
	after := false
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
		return "notice: a SHELL= line follows the cronitor shell shim; leaving SHELL lines unchanged and keeping exec style"
	}
	if users > 1 {
		return "notice: crontab has more than one SHELL= line; keeping exec style and not installing the shell shim"
	}
	return ""
}

// ShimSkipNotices describes exec lines --convert-to-shim leaves unchanged.
// The monitor code is stable across syncs; a line number is not.
func (c Crontab) ShimSkipNotices() []string {
	if c.WriteMode != WriteModeConvertToShim || runtime.GOOS == "windows" {
		return nil
	}
	var out []string
	for _, line := range c.Lines {
		if line.Integration != IntegrationExec || line.GetCode() == "" || len(line.execFlags) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf(
			"notice: monitor %s: skipped --convert-to-shim; flags cannot be expressed as MONITORIO=<key> (%s); left as exec",
			line.GetCode(), strings.Join(line.execFlags, " ")))
	}
	return out
}

// ShimStdoutNotices describes MONITORIO lines rewritten because --no-stdout
// cannot be carried on the marker.
func (c Crontab) ShimStdoutNotices() []string {
	if !c.RewriteShimToExec || runtime.GOOS == "windows" {
		return nil
	}
	var out []string
	for _, line := range c.Lines {
		if line.Integration != IntegrationShim || line.GetCode() == "" {
			continue
		}
		out = append(out, fmt.Sprintf(
			"notice: monitor %s: --no-stdout cannot be expressed on a MONITORIO line; rewritten as exec",
			line.GetCode()))
	}
	return out
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

// ResolveShimInstallPath picks the wrapper path. It does not create directories.
func ResolveShimInstallPath() string {
	if ShimShellPathOverride != "" {
		return ShimShellPathOverride
	}
	if runtime.GOOS != "windows" && systemCronitorDirOK() {
		return DefaultShimShellPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "cronitor", ShimWrapperBase)
	}
	return filepath.Join(home, ".cronitor", ShimWrapperBase)
}

func systemCronitorDirOK() bool {
	if pathWritable("/etc/cronitor") {
		return true
	}
	if _, err := os.Stat("/etc/cronitor"); os.IsNotExist(err) {
		return pathWritable("/etc")
	}
	return false
}

// InstallShimWrapper writes the wrapper atomically. cronitorBin is the invoked
// path, symlink included, so a later upgrade is still the path cron runs.
func InstallShimWrapper(dest, cronitorBin string) (string, error) {
	if dest == "" {
		dest = ResolveShimInstallPath()
	}
	if strings.HasPrefix(dest, "/etc/") && !filepath.IsAbs(cronitorBin) {
		return "", fmt.Errorf("refusing to install %s: cronitor path %q is relative", dest, cronitorBin)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", err
	}
	content := renderShimWrapper(cronitorBin)
	if existing, err := os.ReadFile(dest); err == nil && string(existing) == content {
		if err := os.Chmod(dest, 0755); err != nil {
			return "", err
		}
		return dest, nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), "."+ShimWrapperBase+".*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return "", err
	}
	cleanup = false
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

# dash points a background command at /dev/null before redirections, so keep
# the real stdin on fd 4. Park an already-open fd 3 on fd 9 for the job.
exec 4<&0
CRONITOR_SHIM_SAVED_FD=
if ( : <&3 ) 2>/dev/null || ( : >&3 ) 2>/dev/null; then
  exec 9<&3
  CRONITOR_SHIM_SAVED_FD=9
fi
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
# The assignment has to be literal. A value produced by ${var:+...} is a
# command word, and dash tries to exec it.
CRONITOR_SHIM_FD=3 CRONITOR_SHIM_SAVED_FD="$CRONITOR_SHIM_SAVED_FD" "$CRONITOR_BIN" shell-shim -c "$cmd" 3>"$fifo" <&4 4<&- &
child=$!
exec 4<&-
# sleep is a child of this helper so a kill of the helper reaches it, and
# stdio is /dev/null so a leftover never holds the job's pipes.
(
  exec </dev/null >/dev/null 2>&1
  sleep 3 &
  sp=$!
  trap 'kill "$sp" 2>/dev/null; exit 0' TERM INT HUP
  wait "$sp"
  : >"$hs/timeout"
) &
watch=$!
trap 'kill -TERM "$child" 2>/dev/null; kill "$reader" "$watch" 2>/dev/null; wait "$child"; exit $?' TERM
trap 'kill -INT "$child" 2>/dev/null; kill "$reader" "$watch" 2>/dev/null; wait "$child"; exit $?' INT
trap 'kill -HUP "$child" 2>/dev/null; kill "$reader" "$watch" 2>/dev/null; wait "$child"; exit $?' HUP
while [ ! -s "$hs/byte" ] && [ ! -f "$hs/timeout" ]; do
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
