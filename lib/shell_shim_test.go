package lib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	closeInheritedFDs()
	shimRoot, err := os.MkdirTemp("", "cronitor-shim-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = os.Setenv("TMPDIR", shimRoot)
	code := m.Run()
	if leftovers, _ := filepath.Glob(filepath.Join(shimRoot, "cronitor-shim.*")); len(leftovers) > 0 {
		fmt.Fprintf(os.Stderr, "leftover cronitor-shim temp dirs: %s\n", strings.Join(leftovers, " "))
		if code == 0 {
			code = 1
		}
	}
	os.RemoveAll(shimRoot)
	os.Exit(code)
}

func TestParseMonitorMarker(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		key     string
		rest    string
		matched bool
	}{
		{name: "simple", in: "MONITORIO=abc123 /usr/bin/true", key: "abc123", rest: "/usr/bin/true", matched: true},
		{name: "command with args", in: "MONITORIO=k1 /bin/echo hello", key: "k1", rest: "/bin/echo hello", matched: true},
		{name: "extra spaces", in: "MONITORIO=k1   /bin/echo hello", key: "k1", rest: "/bin/echo hello", matched: true},
		{name: "tab separator", in: "MONITORIO=k1\t/bin/echo", key: "k1", rest: "/bin/echo", matched: true},
		{name: "quoted command kept", in: `MONITORIO=k1 echo "hi  there"`, key: "k1", rest: `echo "hi  there"`, matched: true},
		{name: "bare percent kept", in: "MONITORIO=k1 date +%Y", key: "k1", rest: "date +%Y", matched: true},
		{name: "odd key", in: "MONITORIO=a/b:c@d!+ /bin/true", key: "a/b:c@d!+", rest: "/bin/true", matched: true},
		{name: "no marker", in: "/usr/bin/true", matched: false},
		{name: "marker not at start", in: " echo MONITORIO=k /bin/true", matched: false},
		{name: "prefix only", in: "MONITORIO=", matched: false},
		{name: "key without separator", in: "MONITORIO=abc123", matched: false},
		{name: "similar name", in: "MONITOR=k /bin/true", matched: false},
		{name: "empty", in: "", matched: false},
		// Quoted keys are not a format sync writes. The key runs to the first space.
		{name: "quoted key is not special", in: `MONITORIO="a b" echo hi`, key: `"a`, rest: `b" echo hi`, matched: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, rest, ok := ParseMonitorMarker(tc.in)
			if ok != tc.matched {
				t.Fatalf("matched=%v, want %v (key %q rest %q)", ok, tc.matched, key, rest)
			}
			if !tc.matched {
				return
			}
			if key != tc.key || rest != tc.rest {
				t.Fatalf("key=%q rest=%q, want key=%q rest=%q", key, rest, tc.key, tc.rest)
			}
		})
	}
}

func TestUnmonitoredLinesAreNotRewritten(t *testing.T) {
	lines := []string{
		`0 * * * * cat%hello`,
		`0 * * * * date +%Y`,
		`0 * * * * date +\%Y`,
		`0 * * * * echo 100\%`,
		`0 * * * * printf '\%s\n' x%more`,
		`0 * * * * echo "hello  world"`,
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			ct := parseContent(t, line)
			ct.WriteMode = WriteModeShim
			ct.ShimShellPath = "/etc/cronitor/cronitor-shell"
			if got := strings.TrimSpace(ct.Write()); got != line {
				t.Fatalf("shim write rewrote an unmonitored line\n got %s\nwant %s", got, line)
			}
		})
	}
}

func TestExecFallbacksPlaceNoStdoutAfterExec(t *testing.T) {
	original := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", "staging")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", original) })

	ct := parseContent(t, `0 1 * * * /opt/cronitor --env staging --no-stdout exec k1 /bin/echo already`)
	ct.WriteMode = WriteModeConvertToShim
	got := ct.Write()
	if !strings.Contains(got, "/opt/cronitor --env staging exec --no-stdout k1 /bin/echo already") || strings.Contains(got, "--no-stdout exec") {
		t.Fatalf("flagged convert wrote %s", got)
	}

	// --exec-style, multiple SHELL= (legacy), and dash re-enable of a bare line
	// all take the fresh exec path.
	for _, mode := range []WriteMode{WriteModeExec, WriteModeLegacy} {
		line := Line{
			IsJob:          true,
			CronExpression: "0 * * * *",
			CommandToRun:   "/bin/true",
			Code:           "NEWCODE",
			Mon:            Monitor{NoStdoutPassthru: true},
			Crontab:        Crontab{IsUserCrontab: true, WriteMode: mode},
		}
		written := line.Write()
		if !strings.Contains(written, "cronitor --env staging exec --no-stdout NEWCODE /bin/true") || strings.Contains(written, "--no-stdout exec") {
			t.Fatalf("mode %d wrote %s", mode, written)
		}
	}
}

func TestResolveRealShell(t *testing.T) {
	if got := ResolveRealShell(""); got != "/bin/sh" {
		t.Fatalf("empty=%s", got)
	}
	if got := ResolveRealShell("/bin/sh"); got != "/bin/sh" {
		t.Fatalf("/bin/sh=%s", got)
	}
	if got := ResolveRealShell("/bin/bash"); got != "/bin/bash" {
		t.Fatalf("bash=%s", got)
	}
	if got := ResolveRealShell("/etc/cronitor/cronitor-shell"); got != "/bin/sh" {
		t.Fatalf("shim shell=%s, want /bin/sh", got)
	}
}

func TestExecFlagsComeFromWrapDetector(t *testing.T) {
	ct := parseContent(t, `0 * * * * /opt/cronitor --env staging --no-stdout exec k1 echo "hi  there"`)
	var line *Line
	for _, l := range ct.Lines {
		if l.IsJob {
			line = l
		}
	}
	if line == nil {
		t.Fatal("no job")
	}
	if got := strings.Join(line.execFlags, " "); got != "--env staging --no-stdout" {
		t.Fatalf("flags=%q", got)
	}
	if line.rawTail != `echo "hi  there"` {
		t.Fatalf("raw tail=%q", line.rawTail)
	}
}

func TestShimInstallPathRootUsesEtc(t *testing.T) {
	old := shimEuid
	t.Cleanup(func() { shimEuid = old })
	shimEuid = func() int { return 0 }
	if ShimShellPathOverride != "" {
		t.Fatal("override is set")
	}
	if got := ResolveShimInstallPath(); got != DefaultShimShellPath {
		t.Fatalf("root path=%s", got)
	}
	shimEuid = func() int { return 1000 }
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got := ResolveShimInstallPath(); got != filepath.Join(home, ".cronitor", ShimWrapperBase) {
		t.Fatalf("user path=%s", got)
	}
}

func TestRedactAttachedAPIKey(t *testing.T) {
	got := redactExecFlags([]string{"-ksupersecretvalue", "--env", "staging"})
	if strings.Contains(got, "supersecretvalue") || !strings.Contains(got, "-k<redacted>") {
		t.Fatalf("redacted=%q", got)
	}
}

func TestRedactPingKeyAndCluster(t *testing.T) {
	got := redactExecFlags([]string{
		"--ping-api-key", "pingsecret",
		"-p", "psecret",
		"-vkcombined",
		"--ping-api-key=eqsecret",
		"-vk", "tailsecret",
		"--env", "staging",
	})
	for _, leak := range []string{"pingsecret", "psecret", "combined", "eqsecret", "tailsecret"} {
		if strings.Contains(got, leak) {
			t.Fatalf("leaked %s in %q", leak, got)
		}
	}
	for _, want := range []string{"--ping-api-key", "-p", "-vk<redacted>", "--ping-api-key=<redacted>", "<redacted>", "--env", "staging"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %q", want, got)
		}
	}
}

func TestResolveShimInstallPathDoesNotCreateDirectories(t *testing.T) {
	_, err := os.Stat("/etc/cronitor")
	existed := err == nil
	_ = ResolveShimInstallPath()
	_, err = os.Stat("/etc/cronitor")
	if !existed && err == nil {
		t.Fatal("ResolveShimInstallPath created /etc/cronitor")
	}
}

func TestInstallShimKeepsSymlinkPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "real-cronitor")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "cronitor")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "cronitor-shell")
	if _, err := InstallShimWrapper(dest, link); err != nil {
		t.Fatal(err)
	}
	body := readFile(t, dest)
	if !strings.Contains(body, link) {
		t.Fatalf("wrapper dropped the invoked path:\n%s", body)
	}
	if strings.Contains(body, "real-cronitor") {
		t.Fatalf("wrapper resolved the symlink:\n%s", body)
	}
}

func TestWrapperRechecksByteAfterKill(t *testing.T) {
	dir := t.TempDir()
	dest, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), "/bin/true")
	if err != nil {
		t.Fatal(err)
	}
	body := readFile(t, dest)
	// The INT trap also contains kill -KILL. The watchdog's kill is the one
	// followed immediately by wait, and the byte re-check has to follow that.
	const kill = "kill -KILL \"$child\" 2>/dev/null\nwait \"$child\""
	killAt := strings.Index(body, kill)
	if killAt < 0 {
		t.Fatal("wrapper does not SIGKILL a timed-out cronitor")
	}
	rest := body[killAt+len(kill):]
	fallback := strings.Index(rest, `exec "$REAL_SHELL" -c "$after"`)
	if fallback < 0 {
		t.Fatal("wrapper has no fallback after SIGKILL")
	}
	if !strings.Contains(rest[:fallback], `$hs/byte`) {
		t.Fatalf("no byte re-check between SIGKILL and the fallback exec:\n%s", rest[:fallback])
	}
}

func TestInstallShimRejectsRelativeCronitorUnderEtc(t *testing.T) {
	_, err := InstallShimWrapper("/etc/cronitor/cronitor-shell", "cronitor")
	if err == nil {
		t.Fatal("relative cronitor path was accepted under /etc")
	}
	if _, statErr := os.Stat("/etc/cronitor/cronitor-shell"); statErr == nil {
		t.Fatal("relative path installed a wrapper")
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
