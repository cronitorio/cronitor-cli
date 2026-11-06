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
)

// actsAsExec reports a `cronitor [flags] exec ...` call, not a go test run.
func actsAsExec(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-test.") {
			return false
		}
	}
	for _, a := range args {
		if a == "exec" {
			return true
		}
	}
	return false
}

func TestMain(m *testing.M) {
	// The test binary also acts as the cronitor binary for `cronitor exec`,
	// which the shell wrapper calls and the fixture's exec lines use.
	if len(os.Args) > 1 && actsAsExec(os.Args[1:]) {
		if host := os.Getenv("CRONITOR_SHIM_PING_HOST"); host != "" {
			lib.PingHostOverride = host
		} else {
			lib.PingHostOverride = "http://127.0.0.1:1"
		}
		// The caller's env has empty keys, and flags such as -p must win.
		RootCmd.SetArgs(os.Args[1:])
		if err := RootCmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestPlainExecSignalStatusUnchanged(t *testing.T) {
	code := RunCommand("kill -TERM $$", false, false)
	if code != -1 {
		t.Fatalf("plain exec signal status=%d, want -1", code)
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
	wrapper, err := lib.InstallShimWrapper(filepath.Join(t.TempDir(), "cronitor", "shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	return wrapper
}

func shimEnv(extra ...string) []string {
	env := append(os.Environ(),
		"CRONITOR_SHELL=/bin/sh",
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
