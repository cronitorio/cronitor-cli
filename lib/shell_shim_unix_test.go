//go:build !windows

package lib

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// closeInheritedFDs keeps a leaked fd 3 from looking like cron's own fd.
// os/exec does not close fds >= 3, so the wrapper would skip monitoring.
func closeInheritedFDs() {
	f, err := os.Open("/proc/self/fd")
	if err != nil {
		return
	}
	defer f.Close()
	names, _ := f.Readdirnames(-1)
	self := int(f.Fd())
	for _, name := range names {
		fd, err := strconv.Atoi(name)
		if err != nil || fd < 3 || fd == self {
			continue
		}
		syscall.CloseOnExec(fd)
	}
}

func TestOpenFd3SkipsCronitor(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "invoked")
	bin := filepath.Join(dir, "cronitor")
	stub := "#!/bin/sh\nprintf RAN >> \"$LOG\"\nexit 42\n"
	if err := os.WriteFile(bin, []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	fdPath := filepath.Join(dir, "fd3")
	f, err := os.Create(fdPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 printf JOB >&3")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "LOG="+log)
	cmd.ExtraFiles = []*os.File{f}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exit=%v\n%s", err, out)
	}
	if got := readFile(t, fdPath); got != "JOB" {
		t.Fatalf("fd 3 = %q, want the caller's file", got)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("open fd 3 still ran cronitor: %s", readFile(t, log))
	}
}

func TestWrapperSignalRemovesTempDir(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hung")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), script)
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("TMPDIR")
	if root == "" {
		root = os.TempDir()
	}
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 printf x")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "TMPDIR="+root)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		t.Fatal("SIGTERM did not stop the wrapper")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "cronitor-shim.*")); len(leftovers) > 0 {
		t.Fatalf("signal left %s", leftovers)
	}
}

func TestWrapperIntHungBinaryStopsWithoutRunningJob(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hung")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), script)
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("TMPDIR")
	marker := filepath.Join(dir, "job-ran")
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 touch "+marker)
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "TMPDIR="+root)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		t.Fatal("SIGINT did not stop the wrapper")
	}
	elapsed := time.Since(start)
	if elapsed < 700*time.Millisecond {
		t.Fatalf("SIGINT killed the hung binary immediately (%s)", elapsed)
	}
	if elapsed > 2500*time.Millisecond {
		t.Fatalf("SIGINT left the wrapper blocked for %s", elapsed)
	}
	exit, ok := waitErr.(*exec.ExitError)
	if !ok || exit.ExitCode() != 137 {
		t.Fatalf("exit=%v, want 137", waitErr)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("hung binary still ran the job")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "cronitor-shim.*")); len(leftovers) > 0 {
		t.Fatalf("SIGINT left %s", leftovers)
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
	start := time.Now()
	if err := marked.Run(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("broken binary waited %s; the poll must stop when the child has exited", time.Since(start))
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
