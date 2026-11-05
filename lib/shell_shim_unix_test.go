//go:build !windows

package lib

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

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

	// A marked command with an extra argument is not monitored; argv stays intact.
	run(t, "-c", "MONITORIO=k1 echo hi", "EXTRA")
	if got := readFile(t, out); got != "-c\nMONITORIO=k1 echo hi\nEXTRA\n" {
		t.Fatalf("marked extra args=\n%s", got)
	}

	// Exit codes survive a missing binary, and a shim real-shell does not loop.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, wrapper, "-c", "MONITORIO=abc exit 9")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL="+wrapper)
	err = cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if ctx.Err() != nil {
		t.Fatal("wrapper looped on itself instead of falling back to /bin/sh")
	}
	if !ok || exit.ExitCode() != 9 {
		t.Fatalf("shim real-shell exit=%v, want 9", err)
	}
}

func TestBrokenBinaryDoesNotAffectUnmarkedLines(t *testing.T) {
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
	plain := exec.Command(wrapper, "-c", "printf y >> "+count)
	plain.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh")
	if err := plain.Run(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, count); got != "y" {
		t.Fatalf("unmarked job ran as %q, want y", got)
	}
}

func TestWrapperExecsHealthyBinaryOnce(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "invoked")
	bin := filepath.Join(dir, "cronitor")
	stub := "#!/bin/sh\nprintf x >> \"$LOG\"\nexit 42\n"
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

func TestWrapperExecsBinaryInPlace(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "seen")
	bin := filepath.Join(dir, "cronitor")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$$\" \"$@\" > \"$OUT\"\n"
	if err := os.WriteFile(bin, []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 echo hi")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "OUT="+out)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	want := strconv.Itoa(cmd.Process.Pid) + "\nshell-shim\n-c\nMONITORIO=k1 echo hi\n"
	if got := readFile(t, out); got != want {
		t.Fatalf("binary saw:\n%s\nwant (same pid as the wrapper, so no supervisor):\n%s", got, want)
	}
}

func TestWrapperRoutesBothMarkers(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "seen")
	bin := filepath.Join(dir, "cronitor")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s|' \"$@\" >> \"$OUT\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"CRONITOR=k1 echo a", "MONITORIO=k2 echo b", "CRONITOR_X=1 printf c >> " + out} {
		cmd := exec.Command(wrapper, "-c", line)
		cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "OUT="+out)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", line, err, b)
		}
	}
	want := "shell-shim|-c|CRONITOR=k1 echo a|shell-shim|-c|MONITORIO=k2 echo b|c"
	if got := readFile(t, out); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestShimMarkers(t *testing.T) {
	ct := parseContent(t, strings.Join([]string{
		"0 1 * * * CRONITOR=k1 echo one",
		"0 2 * * * MONITORIO=k2 echo two",
		"0 3 * * * echo three",
	}, "\n"))
	ct.WriteMode = WriteModeShim
	ct.ShimShellPath = "/etc/cronitor/cronitor-shell"
	for _, l := range ct.Lines {
		switch l.CommandToRun {
		case "echo one", "echo two":
			if l.Integration != IntegrationShim {
				t.Fatalf("%q not parsed as a shim line", l.CommandToRun)
			}
			l.Code = l.Code + "x"
		case "echo three":
			l.Code = "k3"
		}
	}
	got := ct.Write()
	for _, want := range []string{
		"0 1 * * * CRONITOR=k1x echo one\n",
		"0 2 * * * MONITORIO=k2x echo two\n",
		"0 3 * * * CRONITOR=k3 echo three\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestShimLineKeepsDashEdit(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		mode WriteMode
		want string
	}{
		{"key kept", "k1", WriteModeShim, "0 1 * * * CRONITOR=k1 echo new"},
		{"monitoring off", "", WriteModeShim, "0 1 * * * echo new"},
		{"exec style", "k1", WriteModeExec, "0 1 * * * cronitor exec k1 echo new"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ct := parseContent(t, "0 1 * * * CRONITOR=k1 echo  old")
			ct.WriteMode = tc.mode
			ct.ShimShellPath = "/etc/cronitor/cronitor-shell"
			for _, l := range ct.Lines {
				if l.IsJob {
					l.CommandToRun = "echo new"
					l.Code = tc.code
				}
			}
			if got := ct.Write(); !strings.Contains(got, tc.want+"\n") {
				t.Fatalf("want line %q in:\n%s", tc.want, got)
			}
		})
	}
}

func TestConvertExecLines(t *testing.T) {
	input := strings.Join([]string{
		"0 1 * * * cronitor exec k1 echo hi",
		"0 2 * * * /usr/local/bin/cronitor exec k2 'cd /tmp && echo x'",
		`0 3 * * * cronitor exec k3 echo "a  b"`,
		"0 4 * * * cronitor exec --no-stdout k4 echo y",
		"0 5 * * * cronitor --api-key secret123 exec k5 echo z",
		"0 7 * * * cronitor --config /etc/other.json exec k7 echo c",
		"0 8 * * * cronitor -v exec k8 echo v",
		"0 6 * * * CRONITOR=k6 echo already",
	}, "\n")
	write := func(content string) (string, []string) {
		ct := parseContent(t, content)
		ct.WriteMode = WriteModeShim
		ct.ConvertExec = true
		ct.ShimShellPath = "/etc/cronitor/cronitor-shell"
		_, notices := ct.ShimNotices()
		return ct.Write(), notices
	}
	got, notices := write(input)
	for _, want := range []string{
		"SHELL=/etc/cronitor/cronitor-shell\n",
		"0 1 * * * CRONITOR=k1 echo hi\n",
		"0 2 * * * CRONITOR=k2 cd /tmp && echo x\n",
		"0 3 * * * CRONITOR=k3 echo \"a  b\"\n",
		"0 4 * * * CRONITOR=k4 CRONITOR_NO_STDOUT=1 echo y\n",
		"0 5 * * * CRONITOR=k5 CRONITOR_API_KEY=secret123 echo z\n",
		"0 7 * * * cronitor --config /etc/other.json exec k7 echo c\n",
		"0 8 * * * cronitor -v exec k8 echo v\n",
		"0 6 * * * CRONITOR=k6 echo already\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	if t.Failed() {
		t.Fatalf("got:\n%s", got)
	}
	joined := strings.Join(notices, "\n")
	if len(notices) != 2 || !strings.Contains(joined, "k7") || !strings.Contains(joined, "k8") {
		t.Fatalf("want one notice each for k7 and k8, got:\n%s", joined)
	}
	if strings.Contains(joined, "other.json") || strings.Contains(joined, "--") {
		t.Fatalf("notice prints flags:\n%s", joined)
	}
	if again, _ := write(got); again != got {
		t.Fatalf("second convert changed the output\n--- first\n%s\n--- second\n%s", got, again)
	}
}

func TestShimSettingsOnMarkerLines(t *testing.T) {
	line := "0 1 * * * CRONITOR=k1 CRONITOR_ENV=staging CRONITOR_API_KEY=abc123 CRONITOR_NO_STDOUT=1 echo  hi"
	parse := func(mode WriteMode) (*Crontab, *Line) {
		ct := parseContent(t, line)
		ct.WriteMode = mode
		ct.ShimShellPath = "/etc/cronitor/cronitor-shell"
		for _, l := range ct.Lines {
			if l.IsJob {
				return ct, l
			}
		}
		t.Fatal("no job")
		return nil, nil
	}

	ct, job := parse(WriteModeShim)
	if job.CommandToRun != "echo  hi" {
		t.Fatalf("CommandToRun = %q; settings must not reach the dashboard or the monitor key", job.CommandToRun)
	}
	if !job.Mon.NoStdoutPassthru {
		t.Fatal("CRONITOR_NO_STDOUT=1 did not set NoStdoutPassthru")
	}
	if got := ct.Write(); !strings.Contains(got, line+"\n") {
		t.Fatalf("unedited line changed:\n%s", got)
	}

	job.CommandToRun = "echo new"
	if got := ct.Write(); !strings.Contains(got, "0 1 * * * CRONITOR=k1 CRONITOR_ENV=staging CRONITOR_API_KEY=abc123 CRONITOR_NO_STDOUT=1 echo new\n") {
		t.Fatalf("dash edit lost the settings:\n%s", got)
	}

	job.Code = ""
	if got := ct.Write(); !strings.Contains(got, "0 1 * * * echo new\n") {
		t.Fatalf("monitoring off kept the settings:\n%s", got)
	}

	ct, _ = parse(WriteModeExec)
	if got := ct.Write(); !strings.Contains(got, "0 1 * * * cronitor --env staging --api-key abc123 exec --no-stdout k1 echo  hi\n") {
		t.Fatalf("--format=exec did not turn settings into flags:\n%s", got)
	}
}
