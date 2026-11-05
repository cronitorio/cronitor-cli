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
)

const (
	IntegrationExec = "exec"
	IntegrationShim = "shim"
)

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
	if raw := strings.TrimLeft(line.rawTail, " \t\v\f\r\n"); raw != "" {
		cmd = raw
	}
	if cmd == "" {
		return ""
	}
	return " " + cmd
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
if [ -x "$CRONITOR_BIN" ]; then
  exec "$CRONITOR_BIN" shell-shim -c "$cmd"
fi
exec "$REAL_SHELL" -c "$after"
`
