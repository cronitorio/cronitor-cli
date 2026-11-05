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
