//go:build !windows

package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/getsentry/raven-go"
	"github.com/spf13/viper"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "shell-shim" {
		lib.PingHostOverride = "http://127.0.0.1:1"
		viper.Set(varApiKey, "")
		viper.Set(varPingApiKey, "")
		if os.Getenv("CRONITOR_SHIM_CRASH_AFTER_BYTE") == "1" {
			shimAfterHandshake = func() { os.Exit(99) }
		}
		if os.Getenv("CRONITOR_SHIM_SIGNAL_BEFORE_START") == "1" {
			shimAfterHandshake = func() {
				time.Sleep(50 * time.Millisecond)
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
			}
		}
		os.Exit(RunShellShim(os.Args))
	}
	closeOnExecInherited()
	// Isolate wrapper mktemp from other packages running in parallel.
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

func TestShellShimDashCIsNotTheConfigFlag(t *testing.T) {
	// -c is the global --config short flag. Cron invokes
	// `cronitor shell-shim -c '<command>'`, so that -c must stay the command.
	cmd, args, err := RootCmd.Find([]string{"shell-shim", "-c", "echo hi"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd == nil || cmd.Name() != "shell-shim" {
		t.Fatalf("command = %v", cmd)
	}
	if !cmd.DisableFlagParsing {
		t.Fatal("shell-shim must not parse flags, or -c is consumed as --config")
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c") || !strings.Contains(joined, "echo hi") {
		t.Fatalf("args after find = %q", args)
	}
}

func TestExplicitShellBypassesBinShHeuristic(t *testing.T) {
	// The child environment says the wrapper, which is what cron exports.
	// The explicit real shell must still win, including when it is /bin/sh.
	childEnv := []string{"SHELL=/etc/cronitor/cronitor-shell", "CRONITOR_REAL_SHELL=/bin/sh"}
	cmd := makeSubcommandExecWithShell("true", "/bin/sh", childEnv)
	if len(cmd.Args) < 3 || cmd.Args[0] != "/bin/sh" || cmd.Args[1] != "-c" || cmd.Args[2] != "true" {
		t.Fatalf("explicit /bin/sh args = %q", cmd.Args)
	}

	name, flag := interpreterForExec("linux", "/bin/sh", func(string) (os.FileInfo, error) {
		return fakeFileInfo{mode: 0755}, nil
	})
	if name == "/bin/sh" {
		t.Fatal("exec heuristic must keep refusing /bin/sh; the shim passes it explicitly instead")
	}
	if flag != "-c" {
		t.Fatalf("flag=%q", flag)
	}

	script := filepath.Join(t.TempDir(), "real-shell")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd = makeSubcommandExecWithShell("echo hi", script, childEnv)
	if cmd.Args[0] != script || cmd.Args[1] != "-c" || cmd.Args[2] != "echo hi" {
		t.Fatalf("explicit shell args = %q", cmd.Args)
	}
}

func TestShellShimPassthroughAndNeverFail(t *testing.T) {
	if os.Getenv("CRONITOR_SHIM_CHILD") == "1" {
		raven.SetDSN("")
		lib.PingHostOverride = "http://127.0.0.1:1"
		viper.Set(varPingApiHost, "http://127.0.0.1:1")
		viper.Set(varApiKey, "")
		viper.Set(varConfig, os.Getenv("CRONITOR_SHIM_CONFIG"))
		initConfig()
		os.Exit(RunShellShim([]string{"cronitor", "shell-shim", "-c", os.Getenv("CRONITOR_SHIM_CMD")}))
	}

	dir := t.TempDir()
	badConfig := filepath.Join(dir, "not-json")
	if err := os.WriteFile(badConfig, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "ran.txt")

	run := func(t *testing.T, shell, command string, extra ...string) int {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestShellShimPassthroughAndNeverFail$")
		cmd.Env = append(os.Environ(),
			"CRONITOR_SHIM_CHILD=1",
			"CRONITOR_SHIM_CMD="+command,
			"CRONITOR_SHIM_CONFIG="+badConfig,
			"CRONITOR_REAL_SHELL="+shell,
			"CRONITOR_API_KEY=",
		)
		cmd.Env = append(cmd.Env, extra...)
		err := cmd.Run()
		if err == nil {
			return 0
		}
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run: %v", err)
		}
		return exit.ExitCode()
	}

	// Unmarked commands never reach cronitor; the wrapper handles them.
	if code := run(t, "/bin/sh", "exit 4"); code != 127 {
		t.Fatalf("unmarked shell-shim exit=%d, want usage 127", code)
	}

	// Odd key, dead ping host, unreadable config: the job still runs.
	monitored := "MONITORIO=not-a-real printf ran > " + shellQuote(sentinel) + "; exit 5"
	if code := run(t, "/bin/sh", monitored); code != 5 {
		t.Fatalf("monitored exit=%d, want 5", code)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("monitored command did not run: %v", err)
	}

	// /bin/sh is honored exactly, not rewritten to bash.
	which := filepath.Join(dir, "which-shell")
	probe := "printf %s \"$0\" > " + shellQuote(which)
	// SHELL is the wrapper, which exec's heuristic would select. The shim must
	// still run under CRONITOR_REAL_SHELL.
	if code := run(t, "/bin/sh", "MONITORIO=abc123 "+probe, "SHELL=/bin/bash"); code != 0 {
		t.Fatalf("probe exit=%d", code)
	}
	got, err := os.ReadFile(which)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "/bin/sh" {
		t.Fatalf("real shell=%q, want /bin/sh (exec's heuristic would have chosen bash)", got)
	}
}

func TestShellShimJobRunsOnceWhenTelemetryPanics(t *testing.T) {
	if os.Getenv("CRONITOR_SHIM_CHILD") == "1" && os.Getenv("CRONITOR_SHIM_PANIC") == "1" {
		telemetryTestHook = func() { panic("ping boom") }
		viper.Set(varApiKey, "")
		lib.PingHostOverride = "http://127.0.0.1:1"
		os.Exit(RunShellShim([]string{"cronitor", "shell-shim", "-c", os.Getenv("CRONITOR_SHIM_CMD")}))
	}
	if os.Getenv("CRONITOR_SHIM_CHILD") == "1" && os.Getenv("CRONITOR_SHIM_PANIC_AFTER") == "1" {
		shellShimAfterJob = func() { panic("after job") }
		viper.Set(varApiKey, "")
		lib.PingHostOverride = "http://127.0.0.1:1"
		os.Exit(RunShellShim([]string{"cronitor", "shell-shim", "-c", os.Getenv("CRONITOR_SHIM_CMD")}))
	}
	if os.Getenv("CRONITOR_SHIM_CHILD") == "1" {
		return
	}

	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	cmdText := "printf x >> " + shellQuote(count) + "; exit 5"
	run := func(env string) int {
		t.Helper()
		os.Remove(count)
		cmd := exec.Command(os.Args[0], "-test.run=^TestShellShimJobRunsOnceWhenTelemetryPanics$")
		cmd.Env = append(os.Environ(),
			"CRONITOR_SHIM_CHILD=1",
			env+"=1",
			"CRONITOR_SHIM_CMD=MONITORIO=abc123 "+cmdText,
			"CRONITOR_REAL_SHELL=/bin/sh",
			"CRONITOR_API_KEY=",
		)
		err := cmd.Run()
		if err == nil {
			return 0
		}
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		return exit.ExitCode()
	}
	if code := run("CRONITOR_SHIM_PANIC"); code != 5 {
		t.Fatalf("ping panic exit=%d, want the job's 5", code)
	}
	if got := readTestFile(t, count); got != "x" {
		t.Fatalf("ping panic ran job as %q", got)
	}
	code := run("CRONITOR_SHIM_PANIC_AFTER")
	if code == 0 || code == 5 {
		t.Fatalf("panic after the job was swallowed (exit %d); a re-run recover would hide it", code)
	}
	if got := readTestFile(t, count); got != "x" {
		t.Fatalf("panic after the job ran it as %q, want once", got)
	}
}

func TestSignalExitCode(t *testing.T) {
	code := RunCommand("kill -TERM $$", false, false, "/bin/sh")
	if code != 143 {
		t.Fatalf("signal exit=%d, want 143", code)
	}
}

func TestPlainExecSignalStatusUnchanged(t *testing.T) {
	code := RunCommand("kill -TERM $$", false, false)
	if code != -1 {
		t.Fatalf("plain exec signal status=%d, want -1", code)
	}
}

func TestMissingRealShellExits127(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	wrapper := installRealShim(t, os.Args[0])
	cmd := shimCommand(wrapper, "-c", "MONITORIO=k1 printf x >> "+shellQuote(count))
	env := make([]string, 0, len(cmd.Env))
	for _, e := range cmd.Env {
		if strings.HasPrefix(e, "CRONITOR_REAL_SHELL=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, "CRONITOR_REAL_SHELL=/bin/nonexistent")
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 127 {
		t.Fatalf("exit=%v, want 127", err)
	}
	if _, statErr := os.Stat(count); !os.IsNotExist(statErr) {
		t.Fatalf("missing shell ran the job as %q", readTestFile(t, count))
	}
}

func TestSignalBeforeStartReachesJob(t *testing.T) {
	wrapper := installRealShim(t, os.Args[0])
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	closeOnExecInherited()
	cmd := exec.CommandContext(ctx, wrapper, "-c", "MONITORIO=k1 sleep 30")
	cmd.Env = shimEnv("CRONITOR_SHIM_SIGNAL_BEFORE_START=1")
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("signal queued before Start was dropped; sleep 30 was still running")
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 143 {
		t.Fatalf("exit=%v, want 143", err)
	}
}

func TestShimTelemetryWaitIsBounded(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			time.Sleep(time.Minute)
			c.Close()
		}
	}()
	oldHost := lib.PingHostOverride
	oldCode := monitorCode
	oldKey := viper.GetString(varApiKey)
	lib.PingHostOverride = "http://" + ln.Addr().String()
	monitorCode = "abc123"
	viper.Set(varApiKey, "")
	t.Cleanup(func() {
		lib.PingHostOverride = oldHost
		monitorCode = oldCode
		viper.Set(varApiKey, oldKey)
	})
	start := time.Now()
	code := RunCommand("exit 0", false, true, "/bin/sh")
	elapsed := time.Since(start)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if elapsed > 8*time.Second {
		t.Fatalf("telemetry held the shim for %s", elapsed)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func installRealShim(t *testing.T, bin string) string {
	t.Helper()
	wrapper, err := lib.InstallShimWrapper(filepath.Join(t.TempDir(), "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	return wrapper
}

func shimEnv(extra ...string) []string {
	env := append(os.Environ(),
		"CRONITOR_REAL_SHELL=/bin/sh",
		"CRONITOR_API_KEY=",
		"CRONITOR_PING_API_KEY=",
		"CRONITOR_CONFIG=",
	)
	return append(env, extra...)
}

// closeOnExecInherited stops the agent and the test harness from leaking
// fds >= 3 into the wrapper. os/exec keeps those fds open across exec.
func closeOnExecInherited() {
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

func shimCommand(name string, args ...string) *exec.Cmd {
	closeOnExecInherited()
	cmd := exec.Command(name, args...)
	cmd.Env = shimEnv()
	return cmd
}

// TestMarkedHandshakeRunsJobOnce fails if the handshake byte is not sent:
// the job would start and the wrapper would run it again after the timeout.
func TestMarkedHandshakeRunsJobOnce(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	wrapper := installRealShim(t, os.Args[0])
	cmd := shimCommand(wrapper, "-c", "MONITORIO=k1 printf x >> "+shellQuote(count))
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, count); got != "x" {
		t.Fatalf("job ran as %q, want one x (a missing handshake byte runs it twice)", got)
	}
}

func TestHandshakeDelayAtTimeoutRunsOnce(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	script := filepath.Join(dir, "delay")
	body := "#!/bin/sh\nsleep 2.99\nexec " + shellQuote(os.Args[0]) + " \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper := installRealShim(t, script)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	closeOnExecInherited()
	cmd := exec.CommandContext(ctx, wrapper, "-c", "MONITORIO=k1 printf x >> "+shellQuote(count))
	cmd.Env = shimEnv()
	start := time.Now()
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("wrapper did not return at the timeout boundary")
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := readTestFile(t, count); got != "x" {
		t.Fatalf("delay at 2.99s ran job as %q, want exactly one", got)
	}
	if time.Since(start) > 8*time.Second {
		t.Fatal("delay test exceeded its bound")
	}
}

func TestCrashAfterHandshakeDoesNotRunJob(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	wrapper := installRealShim(t, os.Args[0])
	cmd := shimCommand(wrapper, "-c", "MONITORIO=k1 printf x >> "+shellQuote(count))
	cmd.Env = shimEnv("CRONITOR_SHIM_CRASH_AFTER_BYTE=1")
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 99 {
		t.Fatalf("exit=%v, want 99 from the crash after the byte", err)
	}
	if _, statErr := os.Stat(count); !os.IsNotExist(statErr) {
		got := readTestFile(t, count)
		t.Fatalf("crash after the byte ran the job as %q, want zero times", got)
	}
}

func TestWrapperKillsHungBinary(t *testing.T) {
	dir := t.TempDir()
	count := filepath.Join(dir, "count")
	script := filepath.Join(dir, "hung")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper := installRealShim(t, script)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	closeOnExecInherited()
	cmd := exec.CommandContext(ctx, wrapper, "-c", "MONITORIO=k1 printf x >> "+shellQuote(count))
	cmd.Env = shimEnv()
	start := time.Now()
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil || time.Since(start) > 6*time.Second {
		t.Fatal("hung cronitor was not killed at the watchdog")
	}
	if got := readTestFile(t, count); got != "x" {
		t.Fatalf("hung binary fallback ran job as %q", got)
	}
}

func TestMarkedLineGetsCronStdin(t *testing.T) {
	// Cron splits at % and feeds the rest as stdin. The wrapper must not
	// point the backgrounded cronitor at /dev/null (dash does that before
	// a plain <&0 redirection).
	wrapper := installRealShim(t, os.Args[0])
	field := "MONITORIO=k1 cat%line-one%line-two"
	command, stdin := splitCronPercent(field)
	if command != "MONITORIO=k1 cat" || stdin != "line-one\nline-two\n" {
		t.Fatalf("split = %q %q", command, stdin)
	}
	for _, sh := range []string{"dash", "bash"} {
		t.Run(sh, func(t *testing.T) {
			if _, err := exec.LookPath(sh); err != nil {
				t.Skip(sh)
			}
			closeOnExecInherited()
			cmd := exec.Command(sh, wrapper, "-c", command)
			cmd.Env = shimEnv()
			cmd.Stdin = strings.NewReader(stdin)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", sh, err, out)
			}
			if string(out) != stdin {
				t.Fatalf("%s delivered %q, want %q", sh, out, stdin)
			}
		})
	}
}

func TestMarkedJobDoesNotInheritHandshakeFd(t *testing.T) {
	wrapper := installRealShim(t, os.Args[0])
	cmd := shimCommand(wrapper, "-c", `MONITORIO=k1 if { echo hi >&3; } 2>/dev/null; then echo OPEN; else echo CLOSED; fi; printf '%s' "$CRONITOR_SHIM_FD$CRONITOR_SHIM_SAVED_FD"`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exit=%v\n%s", err, out)
	}
	if string(out) != "CLOSED\n" {
		t.Fatalf("fd 3 / env = %q, want CLOSED and no shim env", out)
	}
}

func TestMarkedJobKeepsExistingFd3(t *testing.T) {
	wrapper := installRealShim(t, os.Args[0])
	path := filepath.Join(t.TempDir(), "fd3")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := shimCommand(wrapper, "-c", "MONITORIO=k1 echo JOB >&3")
	cmd.ExtraFiles = []*os.File{f}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exit=%v\n%s", err, out)
	}
	if got := readTestFile(t, path); got != "JOB\n" {
		t.Fatalf("fd 3 = %q, want the caller's file", got)
	}
}

func TestStdoutClosesWhenHandshakeArrives(t *testing.T) {
	wrapper := installRealShim(t, os.Args[0])
	cmd := shimCommand(wrapper, "-c", "MONITORIO=k1 echo hi")
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = w
	cmd.Stderr = w
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	w.Close()
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatalf("stdout stayed open for %s", time.Since(start))
	}
	if string(out) != "hi\n" {
		t.Fatalf("output=%q", out)
	}
}

func TestSignalReachesMarkedJob(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "started")
	wrapper := installRealShim(t, os.Args[0])
	cmd := shimCommand(wrapper, "-c", "MONITORIO=k1 printf started > "+shellQuote(marker)+"; sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			t.Fatal("marked job did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		exit, ok := err.(*exec.ExitError)
		if !ok || exit.ExitCode() != 143 {
			t.Fatalf("exit=%v, want 143", err)
		}
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		t.Fatal("SIGTERM did not stop the marked job")
	}
	out, err := exec.Command("pgrep", "-f", marker).CombinedOutput()
	if err == nil {
		t.Fatalf("job still running after SIGTERM:\n%s", out)
	}
}

// splitCronPercent is cron's % rule for the stdin test, not the production splitter.
func splitCronPercent(field string) (command, stdin string) {
	i := strings.IndexByte(field, '%')
	if i < 0 {
		return field, ""
	}
	var b strings.Builder
	for _, ch := range field[i+1:] {
		if ch == '%' {
			b.WriteByte('\n')
			continue
		}
		b.WriteRune(ch)
	}
	if b.Len() == 0 || !strings.HasSuffix(b.String(), "\n") {
		b.WriteByte('\n')
	}
	return field[:i], b.String()
}
