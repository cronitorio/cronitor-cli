//go:build !windows

package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

// TestShimFixtureRuns runs every marked job in fixtures/shim/crontab.expected
// through the real wrapper and shell-shim, and runs the matching line from
// crontab.input with plain /bin/sh. Output and exit code must match, and the
// marked run must send a run ping and a final ping.
func TestShimFixtureRuns(t *testing.T) {
	input := fixtureJobs(t, "crontab.input")
	written := fixtureJobs(t, "crontab.expected")
	if len(input) != len(written) || len(input) == 0 {
		t.Fatalf("job count: input %d, expected %d", len(input), len(written))
	}

	fixture := t.TempDir()
	app := filepath.Join(fixture, "my app")
	if err := os.MkdirAll(app, 0755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '[%s]' \"$@\"; echo\n"
	if err := os.WriteFile(filepath.Join(app, "run.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var pings []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		pings = append(pings, r.URL.String())
		mu.Unlock()
	}))
	defer server.Close()

	wrapper := installRealShim(t, os.Args[0])
	env := shimEnv("FIXTURE="+fixture, "CRONITOR_SHIM_PING_HOST="+server.URL)

	ran := 0
	for i, field := range written {
		if _, _, ok := lib.ParseMonitorMarker(cronCommand(field)); !ok {
			continue
		}
		ran++
		t.Run(field, func(t *testing.T) {
			wantOut, wantCode := runCronField(t, "/bin/sh", input[i], env)
			gotOut, gotCode := runCronField(t, wrapper, field, env)
			if gotOut != wantOut || gotCode != wantCode {
				t.Fatalf("written line differs from the original\n  input:   %s -> %q (exit %d)\n  written: %s -> %q (exit %d)",
					input[i], wantOut, wantCode, field, gotOut, gotCode)
			}
			key, _, _ := lib.ParseMonitorMarker(cronCommand(field))
			mu.Lock()
			defer mu.Unlock()
			var run, final bool
			for _, p := range pings {
				run = run || strings.HasPrefix(p, "/"+key+"/run?")
				final = final || strings.HasPrefix(p, "/"+key+"/complete?") || strings.HasPrefix(p, "/"+key+"/fail?")
			}
			if !run || !final {
				t.Fatalf("pings for %s: run=%v final=%v\n%s", key, run, final, strings.Join(pings, "\n"))
			}
		})
	}
	if ran < 10 {
		t.Fatalf("only %d marked jobs ran; the fixture or the writer changed", ran)
	}
}

// fixtureJobs returns the command field of each job line, in file order.
func fixtureJobs(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "fixtures", "shim", name))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if field := cronField(line); field != "" {
			out = append(out, field)
		}
	}
	return out
}

// cronField drops the schedule (5 fields, or 1 for @keywords). Env lines give "".
func cronField(line string) string {
	n := 5
	if strings.HasPrefix(line, "@") {
		n = 1
	}
	rest := line
	for i := 0; i < n; i++ {
		rest = strings.TrimLeft(rest, " \t")
		j := strings.IndexAny(rest, " \t")
		if j < 0 {
			return ""
		}
		word := rest[:j]
		if i == 0 && n == 5 && strings.Contains(word, "=") {
			return ""
		}
		rest = rest[j:]
	}
	return strings.TrimLeft(rest, " \t")
}

// cronCommand applies cron's % rule: \% is a literal %, and the first bare %
// starts stdin. It returns the command part.
func cronCommand(field string) string {
	cmd, _ := cronSplit(field)
	return cmd
}

func cronSplit(field string) (command, stdin string) {
	var cmd strings.Builder
	for i := 0; i < len(field); i++ {
		switch {
		case field[i] == '\\' && i+1 < len(field) && field[i+1] == '%':
			cmd.WriteByte('%')
			i++
		case field[i] == '%':
			return cmd.String(), strings.ReplaceAll(field[i+1:], "%", "\n") + "\n"
		default:
			cmd.WriteByte(field[i])
		}
	}
	return cmd.String(), ""
}

func runCronField(t *testing.T, shell, field string, env []string) (string, int) {
	t.Helper()
	command, stdin := cronSplit(field)
	cmd := exec.Command(shell, "-c", command)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("%s: %v", field, err)
	}
	return out.String(), code
}
