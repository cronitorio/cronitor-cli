//go:build !windows

package cmd

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

// TestShimFixtureRuns runs every marked job in the default and
// --format=short outputs through the real wrapper and cronitor exec, and runs
// the matching line from crontab.input with /bin/sh. Output and exit code must
// match, and the marked run must send a run ping and a final ping. The test
// binary is on PATH as cronitor and monicli, so original exec lines run
// through the real `cronitor exec`.
func TestShimFixtureRuns(t *testing.T) {
	fixture := t.TempDir()
	app := filepath.Join(fixture, "my app")
	if err := os.MkdirAll(app, 0755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '[%s]' \"$@\"; echo\n"
	if err := os.WriteFile(filepath.Join(app, "run.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	for _, name := range []string{"cronitor", "monicli"} {
		if err := os.Symlink(os.Args[0], filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
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
	env := shimEnv("FIXTURE="+fixture, "CRONITOR_SHIM_PING_HOST="+server.URL,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	input := fixtureJobs(t, "crontab.input")
	outputs := map[string]fixtureOutput{}
	for _, file := range []string{"crontab.expected", "crontab.format-short.expected"} {
		t.Run(file, func(t *testing.T) {
			written := fixtureJobs(t, file)
			if len(input) != len(written) || len(input) == 0 {
				t.Fatalf("job count: input %d, %s %d", len(input), file, len(written))
			}
			ran := 0
			for i, field := range written {
				key, _, ok := lib.ParseMonitorMarker(cronCommand(field))
				if !ok {
					continue
				}
				ran++
				// The fixture's absolute cronitor path does not exist here.
				original := strings.Replace(input[i], "/usr/local/bin/cronitor ", "cronitor ", 1)
				t.Run(field, func(t *testing.T) {
					mu.Lock()
					pings = nil
					mu.Unlock()
					wantOut, wantCode := runCronField(t, "/bin/sh", original, env)
					mu.Lock()
					wantPings := normalizePings(pings)
					pings = nil
					mu.Unlock()
					gotOut, gotCode := runCronField(t, wrapper, field, env)
					_, rest, _ := lib.ParseMonitorMarker(field)
					_, job := lib.SplitShimSettings(rest)
					mu.Lock()
					outputs[job] = fixtureOutput{Exit: wantCode, Output: wantOut}
					mu.Unlock()
					if gotOut != wantOut || gotCode != wantCode {
						t.Fatalf("written line differs from the original\n  input:   %s -> %q (exit %d)\n  written: %s -> %q (exit %d)",
							original, wantOut, wantCode, field, gotOut, gotCode)
					}
					mu.Lock()
					defer mu.Unlock()
					var run, final bool
					for _, p := range pings {
						state := pingState(p, key)
						run = run || state == "run"
						final = final || state == "complete" || state == "fail"
					}
					if !run || !final {
						t.Fatalf("pings for %s: run=%v final=%v\n%s", key, run, final, strings.Join(pings, "\n"))
					}
					// An original cronitor exec line pings too. Both must send the same pings.
					if got := normalizePings(pings); len(wantPings) > 0 && got != wantPings {
						t.Fatalf("pings differ from the original exec line\n  input:   %s\n%s\n  written: %s\n%s", original, wantPings, field, got)
					}
					// A CRONITOR_ENV setting must reach the pings.
					if strings.Contains(field, " CRONITOR_ENV=staging ") && !strings.Contains(normalizePings(pings), "env=staging") {
						t.Fatalf("CRONITOR_ENV did not reach the pings:\n%s", strings.Join(pings, "\n"))
					}
				})
			}
			if ran < 10 {
				t.Fatalf("only %d marked jobs ran; the fixture or the writer changed", ran)
			}
		})
	}
	checkFixtureOutputs(t, outputs)
}

var updateFixtureOutputs = flag.Bool("update", false, "rewrite fixtures/shim/crontab.outputs")

// fixtureOutput is what a fixture job prints and its exit code. tests/real-cron.sh
// reads crontab.outputs and checks real cron's pings against it.
type fixtureOutput struct {
	Exit   int    `json:"exit"`
	Output string `json:"output"`
}

func checkFixtureOutputs(t *testing.T, outputs map[string]fixtureOutput) {
	t.Helper()
	path := filepath.Join("..", "fixtures", "shim", "crontab.outputs")
	got, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if *updateFixtureOutputs {
		if err := os.WriteFile(path, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd -run TestShimFixtureRuns -update)", err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s is out of date (rerun with -update and review the diff)\n%s", path, got)
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

// normalizePings keeps the parts of each ping that must match between an
// exec line and its CRONITOR= line: path, env, message and exit code.
func normalizePings(pings []string) string {
	var out []string
	for _, p := range pings {
		u, err := url.Parse(p)
		if err != nil {
			out = append(out, p)
			continue
		}
		q := u.Query()
		out = append(out, fmt.Sprintf("%s env=%s msg=%q status=%s", u.Path, q.Get("env"), q.Get("msg"), q.Get("status_code")))
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

// pingState returns the state of a ping for key, in either URL form:
// /<key>/<state>?... or, with a ping API key, /ping/<pingkey>/<key>?state=<state>.
func pingState(ping, key string) string {
	u, err := url.Parse(ping)
	if err != nil {
		return ""
	}
	if state := u.Query().Get("state"); state != "" && strings.HasSuffix(u.Path, "/"+key) {
		return state
	}
	if dir, state := path.Split(u.Path); strings.HasSuffix(dir, "/"+key+"/") {
		return state
	}
	return ""
}
