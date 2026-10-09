package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

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
	if line.Integration == IntegrationShim && line.rawTail != "" {
		return MonitorPrefix + code + line.rawTail
	}
	cmd := line.rawTail
	if cmd == "" {
		cmd = line.CommandToRun
	}
	if cmd == "" {
		return MonitorPrefix + code
	}
	return MonitorPrefix + code + " " + cmd
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

// UserShellCount counts SHELL= lines that are not the wrapper.
func (c Crontab) UserShellCount() int {
	n := 0
	for _, l := range c.Lines {
		if l == nil || !l.isManagedEnv() || l.GetEnvVarKey() != "SHELL" {
			continue
		}
		if IsShimShellPath(l.GetEnvVarValue()) {
			continue
		}
		n++
	}
	return n
}

// ShimSkipNotices describes exec lines --convert-to-shim leaves unchanged.
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
			"notice: line %d: skipped --convert-to-shim; flags cannot be expressed as MONITORIO=<key> (%s); left as exec",
			line.LineNumber, strings.Join(line.execFlags, " ")))
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
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "", err
	}
	warnShimBinaryOwnership(dest, cronitorBin)
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

// The wrapper execs cronitor only for `-c` plus MONITORIO=<key> and a separator.
// Anything else is the real shell, argv untouched. No handshake byte means the
// binary never started the job, so the stripped command runs once.
const shimWrapperTemplate = `#!/bin/sh
# cronitor crontab SHELL shim. Cron runs: this-script -c '<command field>'
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

hs=$(mktemp -d "${TMPDIR:-/tmp}/cronitor-shim.XXXXXX") || exec "$REAL_SHELL" -c "$after"
fifo=$hs/started
if ! mkfifo "$fifo"; then
  rm -rf "$hs"
  exec "$REAL_SHELL" -c "$after"
fi
(
  byte=
  read byte <"$fifo" || true
  printf '%s' "$byte" >"$hs/byte"
) &
reader=$!
(
  sleep 3
  kill "$reader" 2>/dev/null
) &
watch=$!
CRONITOR_SHIM_FD=3 "$CRONITOR_BIN" shell-shim -c "$cmd" 3>"$fifo" &
child=$!
wait "$reader" 2>/dev/null
kill "$watch" 2>/dev/null
wait "$watch" 2>/dev/null
if [ -s "$hs/byte" ]; then
  rm -rf "$hs"
  wait "$child"
  exit $?
fi
kill "$child" 2>/dev/null
wait "$child" 2>/dev/null
rm -rf "$hs"
exec "$REAL_SHELL" -c "$after"
`
