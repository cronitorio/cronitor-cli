//go:build !windows

package cmd

import (
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
	// The test binary also acts as the cronitor binary: as `shell-shim` for
	// the wrapper, and as `exec` for the fixture's original exec lines.
	if len(os.Args) > 1 && (os.Args[1] == "shell-shim" || os.Args[1] == "exec") {
		if host := os.Getenv("CRONITOR_SHIM_PING_HOST"); host != "" {
			lib.PingHostOverride = host
		} else {
			lib.PingHostOverride = "http://127.0.0.1:1"
		}
		viper.Set(varApiKey, "")
		viper.Set(varPingApiKey, "")
		if os.Args[1] == "shell-shim" {
			os.Exit(RunShellShim(os.Args))
		}
		RootCmd.SetArgs(os.Args[1:])
		if err := RootCmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
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
	out, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 127 {
		t.Fatalf("exit=%v, want 127\n%s", err, out)
	}
	if !strings.Contains(string(out), "/bin/nonexistent") {
		t.Fatalf("stderr missing the shell path:\n%s", out)
	}
	if _, statErr := os.Stat(count); !os.IsNotExist(statErr) {
		t.Fatalf("missing shell ran the job as %q", readTestFile(t, count))
	}
}

func TestPlainExecDropsSignalBeforeStart(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		done <- RunCommand("sleep 1; echo after", false, false)
	}()
	time.Sleep(8 * time.Millisecond)
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("plain exec exit=%d, want 0 after a signal before Start", code)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("plain exec did not finish")
	}
}

func TestPlainExecSignalLeavesBackgroundChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	script := "trap 'exit 7' TERM; sleep 20 >/dev/null 2>&1 & echo $! > " + shellQuote(pidFile) + "; wait"
	done := make(chan int, 1)
	go func() {
		done <- RunCommand(script, false, false)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	select {
	case code := <-done:
		if code != 7 {
			t.Fatalf("plain exec exit=%d, want 7", code)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("plain exec did not return after TERM")
	}
	pidText := strings.TrimSpace(readTestFile(t, pidFile))
	pid, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("background child %d was killed with the job: %v", pid, err)
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

func shimCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = shimEnv()
	return cmd
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
