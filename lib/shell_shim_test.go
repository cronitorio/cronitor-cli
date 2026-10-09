package lib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

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

func TestShimCommandBytesSurviveWrite(t *testing.T) {
	line := `15 * * * * MONITORIO=k1 echo "hello  world" && date +%Y`
	ct := parseContent(t, line)
	ct.WriteMode = WriteModeShim
	ct.ShimShellPath = "/etc/cronitor/cronitor-shell"
	got := ct.Write()
	if !strings.Contains(got, line+"\n") && !strings.Contains(got, line) {
		t.Fatalf("shim line was rebuilt:\n%s", got)
	}
	for _, job := range ct.Lines {
		if job.Integration == IntegrationShim {
			job.Code = "NEW"
		}
	}
	changed := ct.Write()
	if !strings.Contains(changed, `MONITORIO=NEW echo "hello  world" && date +%Y`) {
		t.Fatalf("key change rebuilt the command:\n%s", changed)
	}
	if strings.Contains(changed, `+\%Y`) || strings.Contains(changed, `hello world"`) {
		t.Fatalf("whitespace or percent changed:\n%s", changed)
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

func TestResolveShimInstallPathDoesNotCreateDirectories(t *testing.T) {
	_, err := os.Stat("/etc/cronitor")
	existed := err == nil
	_ = ResolveShimInstallPath()
	_, err = os.Stat("/etc/cronitor")
	if !existed && err == nil {
		t.Fatal("ResolveShimInstallPath created /etc/cronitor")
	}
}

func TestWrapperRoutesAndFallsBack(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), filepath.Join(dir, "missing-cronitor"))
	if err != nil {
		t.Fatal(err)
	}
	recorder := filepath.Join(dir, "recorder.sh")
	out := filepath.Join(dir, "saw.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OUT\"\nprintf '%s' \"$SHELL\" > \"$OUT.shell\"\n"
	if err := os.WriteFile(recorder, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, args ...string) {
		t.Helper()
		os.Remove(out)
		cmd := exec.Command(wrapper, args...)
		cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL="+recorder, "OUT="+out, "SHELL="+wrapper)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wrapper: %v\n%s", err, output)
		}
	}
	run(t, "-c", `echo "hello  world"`)
	if got := readFile(t, out); got != "-c\necho \"hello  world\"\n" {
		t.Fatalf("passthrough args=\n%s", got)
	}
	if shell := readFile(t, out+".shell"); shell != recorder {
		t.Fatalf("SHELL=%q, want the real shell", shell)
	}
	run(t, "-lc", "echo hi")
	if got := readFile(t, out); got != "-lc\necho hi\n" {
		t.Fatalf("-lc args=\n%s", got)
	}
	run(t, "-c", "cmd", "extra")
	if got := readFile(t, out); got != "-c\ncmd\nextra\n" {
		t.Fatalf("extra args=\n%s", got)
	}
	run(t, "script.sh")
	if got := readFile(t, out); got != "script.sh\n" {
		t.Fatalf("script args=\n%s", got)
	}
	run(t, "-c", "MONITORIO=abc123")
	if got := readFile(t, out); got != "-c\nMONITORIO=abc123\n" {
		t.Fatalf("missing separator was treated as a marker:\n%s", got)
	}
	run(t, "-c", "MONITORIO=k1 echo  hi")
	if got := readFile(t, out); got != "-c\necho  hi\n" {
		t.Fatalf("stripped command=\n%s", got)
	}

	// Exit codes survive a missing binary, and a shim real-shell does not loop.
	cmd := exec.Command(wrapper, "-c", "MONITORIO=abc exit 9")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL="+wrapper)
	err = cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 9 {
		t.Fatalf("shim real-shell exit=%v, want 9", err)
	}
}

func TestWrapperTruncatedBinaryRunsJobOnce(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "cronitor")
	if err := os.WriteFile(bin, []byte("not an executable"), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	count := filepath.Join(dir, "count")
	marked := exec.Command(wrapper, "-c", "MONITORIO=k1 printf x >> "+count)
	marked.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh")
	if err := marked.Run(); err != nil {
		t.Fatal(err)
	}
	plain := exec.Command(wrapper, "-c", "printf y >> "+count)
	plain.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh")
	if err := plain.Run(); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, count)
	if got != "xy" {
		t.Fatalf("jobs ran as %q, want one each of x and y", got)
	}
}

func TestWrapperExecsHealthyBinaryOnce(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "invoked")
	bin := filepath.Join(dir, "cronitor")
	stub := "#!/bin/sh\nprintf x >> \"$LOG\"\nprintf '1\\n' >&3\nexit 42\n"
	if err := os.WriteFile(bin, []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), bin) {
		t.Fatalf("wrapper does not reference %s:\n%s", bin, body)
	}
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 printf z >> "+log)
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "LOG="+log)
	err = cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 42 {
		t.Fatalf("exit=%v, want 42 from the stub (wrapper did not exec it)", err)
	}
	if got := readFile(t, log); got != "x" {
		t.Fatalf("binary/job log=%q, want a single stub write and no fallback", got)
	}

	// Unmarked argv must not exec the binary.
	os.Remove(log)
	plain := exec.Command(wrapper, "-c", "printf y >> "+log)
	plain.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "LOG="+log)
	if err := plain.Run(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, log); got != "y" {
		t.Fatalf("unmarked log=%q", got)
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
