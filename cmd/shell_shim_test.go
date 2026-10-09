package cmd

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/getsentry/raven-go"
	"github.com/spf13/viper"
)

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

	run := func(t *testing.T, shell, command string) int {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestShellShimPassthroughAndNeverFail$")
		cmd.Env = append(os.Environ(),
			"CRONITOR_SHIM_CHILD=1",
			"CRONITOR_SHIM_CMD="+command,
			"CRONITOR_SHIM_CONFIG="+badConfig,
			"CRONITOR_REAL_SHELL="+shell,
			"CRONITOR_API_KEY=",
		)
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

	if code := run(t, "/bin/sh", "exit 4"); code != 4 {
		t.Fatalf("passthrough exit=%d, want 4", code)
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
	if code := run(t, "/bin/sh", "MONITORIO=abc123 "+probe); code != 0 {
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
	code := RunCommandWithShell("kill -TERM $$", false, false, "/bin/sh")
	if code != 143 {
		t.Fatalf("signal exit=%d, want 143", code)
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
	code := RunCommandWithShell("exit 0", false, true, "/bin/sh")
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
