package lib

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/kballard/go-shellquote"
	"github.com/spf13/viper"
)

func TestSplitShellWordsMatchesShellquote(t *testing.T) {
	inputs := []string{
		"hello",
		"hello goodbye",
		"hello   goodbye",
		`cronitor --env prod exec abc /usr/bin/true`,
		`cronitor exec k1 'cd /tmp && echo "hi"'`,
		`"cd /tmp && echo \"hi\""`,
		`echo "hi there"`,
		`/usr/local/bin/cronitor.exe --no-stdout exec key C:/job.bat`,
		"'don'\\''t you know'",
		`echo 100\%`,
	}
	for _, input := range inputs {
		got, _, err := splitShellWords(input)
		want, wantErr := shellquote.Split(input)
		if (err == nil) != (wantErr == nil) {
			t.Errorf("Split(%q) err=%v, shellquote err=%v", input, err, wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Split(%q)=%q, shellquote=%q", input, got, want)
		}
	}
}

func TestWrapDetection(t *testing.T) {
	// File crontabs are not user crontabs, so a leading username would be
	// peeled only when `id -u` accepts it. These commands are not usernames.
	tests := []struct {
		name    string
		line    string
		code    string
		command string
		wrapped bool
	}{
		{"plain", `0 * * * * cronitor exec abc123 /usr/bin/true`, "abc123", "/usr/bin/true", true},
		{"env flag", `0 * * * * cronitor --env prod exec def456 /usr/bin/true`, "def456", "/usr/bin/true", true},
		{"env equals", `0 * * * * cronitor --env=prod exec def456 /usr/bin/true`, "def456", "/usr/bin/true", true},
		{"no stdout", `0 * * * * cronitor --no-stdout exec ghi789 /usr/bin/true`, "ghi789", "/usr/bin/true", true},
		{"env and no stdout", `0 * * * * cronitor --env prod --no-stdout exec k1 /usr/bin/true`, "k1", "/usr/bin/true", true},
		{"no stdout then env", `0 * * * * cronitor --no-stdout --env prod exec k1 /usr/bin/true`, "k1", "/usr/bin/true", true},
		{"verbose and hostname", `0 * * * * cronitor -v --hostname my-host exec k2 /bin/echo hello`, "k2", "/bin/echo hello", true},
		{"hostname equals", `0 * * * * cronitor --hostname=box exec k2 /bin/echo hello`, "k2", "/bin/echo hello", true},
		{"log and api version", `0 * * * * cronitor --log /var/log/cronitor.log --api-version 2025-11-28 exec k3 /bin/true`, "k3", "/bin/true", true},
		{"attached short config", `0 * * * * cronitor -c/tmp/cronitor.json exec k4 /bin/true`, "k4", "/bin/true", true},
		{"path prefix", `0 * * * * /usr/local/bin/cronitor exec jkl012 /usr/bin/true`, "jkl012", "/usr/bin/true", true},
		{"dot slash", `0 * * * * ./cronitor exec abc /bin/true`, "abc", "/bin/true", true},
		{"exe", `0 * * * * cronitor.exe exec abc /bin/true`, "abc", "/bin/true", true},
		{"exe path and env", `0 * * * * /usr/local/bin/cronitor.exe --env X exec abc /bin/true`, "abc", "/bin/true", true},
		{"quoted windows path", `0 * * * * "C:/Program Files/cronitor.exe" exec abc /bin/true`, "abc", "/bin/true", true},
		{"backslash windows path", "0 * * * * \"C:\\\\Program Files\\\\cronitor.exe\" --no-stdout exec abc C:/scripts/job.bat", "abc", "C:/scripts/job.bat", true},
		{"command args", `0 * * * * cronitor exec abc /usr/bin/true --flag arg`, "abc", "/usr/bin/true --flag arg", true},
		{"exec inside command", `0 * * * * cronitor exec abc /bin/echo exec me`, "abc", "/bin/echo exec me", true},
		{"reboot with flags", `@reboot cronitor --env prod exec abc /usr/bin/true`, "abc", "/usr/bin/true", true},
		{"not wrapped", `0 * * * * /usr/bin/true --flag`, "", "/usr/bin/true --flag", false},
		{"suffix is not the binary", `0 * * * * notcronitor exec abc /bin/true`, "", "notcronitor exec abc /bin/true", false},
		{"cronitor-helper", `0 * * * * cronitor-helper exec abc /bin/true`, "", "cronitor-helper exec abc /bin/true", false},
		{"discover subcommand", `30 * * * * cronitor discover /etc/crontab`, "", "cronitor discover /etc/crontab", false},
		{"simple quotes kept", `0 * * * * cronitor exec abc echo "hi there"`, "abc", `echo "hi there"`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := parseJobLine(t, tc.line)
			if tc.wrapped && job.Code != tc.code {
				t.Fatalf("code=%q, want %q (command %q)", job.Code, tc.code, job.CommandToRun)
			}
			if !tc.wrapped && job.Code != "" {
				t.Fatalf("unexpected code %q for %q", job.Code, job.CommandToRun)
			}
			if job.CommandToRun != tc.command {
				t.Fatalf("command=%q, want %q", job.CommandToRun, tc.command)
			}
			if strings.Contains(tc.line, "--no-stdout") && tc.wrapped && !job.Mon.NoStdoutPassthru {
				t.Fatal("expected --no-stdout to be remembered on the line")
			}
		})
	}
}

func TestComplexCommandRoundTrip(t *testing.T) {
	commands := []string{
		`cd /tmp && echo "hi"`,
		`cd /tmp && echo 'hi'`,
		`cd /tmp && echo $HOME`,
		`cd /tmp && echo foo\bar`,
		"printf '%s\\n' \"hi\" && true",
		`echo "a'b" | cat`,
		`test -n "$VAR" && echo ok || echo no`,
		`echo 100% && date +%Y`,
		`awk '{print $1}' | head`,
		`echo "say \"hi\"" && true`,
		`cd /tmp; echo done`,
		`echo 100\% && true`,
		`echo don't && true`,
		`echo  hi && true`,
		`true&&echo "hi"`,
		`date +%Y`,
		`echo "100%"`,
		`/usr/bin/true --flag arg`,
	}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			written, again, parsed := rewriteCommand(t, command)
			if again != written {
				t.Fatalf("rewrite changed bytes\n first: %s\nsecond: %s", written, again)
			}
			if parsed != command {
				t.Fatalf("command changed\n want: %q\n  got: %q\n line: %s", command, parsed, written)
			}
			assertNoUnescapedPercent(t, written)
			requireShSyntax(t, commandFieldOf(written))
		})
	}
}

func TestHistoricalComplexQuotingParses(t *testing.T) {
	// Issue #2: wrapping cd /tmp && echo "hi" used to reparse into a truncated
	// command ending in a backslash and an unterminated quote.
	line := `0 * * * * cronitor exec k1 "cd /tmp && echo \"hi\""`
	job := parseJobLine(t, line)
	want := `cd /tmp && echo "hi"`
	if job.Code != "k1" {
		t.Fatalf("code=%q", job.Code)
	}
	if job.CommandToRun != want {
		t.Fatalf("command=%q, want %q", job.CommandToRun, want)
	}
	if strings.HasSuffix(job.CommandToRun, `\`) {
		t.Fatalf("command still truncated: %q", job.CommandToRun)
	}

	rewritten := job.Write()
	requireShSyntax(t, commandFieldOf(rewritten))
	second := parseJobLine(t, rewritten)
	if second.Write() != rewritten {
		t.Fatalf("rewritten line is not stable\n first: %s\nsecond: %s", rewritten, second.Write())
	}
	if second.CommandToRun != want {
		t.Fatalf("after rewrite command=%q", second.CommandToRun)
	}
}

func TestSyncRewriteIsIdempotent(t *testing.T) {
	originalEnv := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", "prod")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", originalEnv) })

	input := strings.Join([]string{
		"SHELL=/bin/bash",
		"# Name: Nightly",
		"0 2 * * * /usr/local/bin/backup.sh --full",
		"# cronitor: ignore",
		"15 2 * * * echo skip",
		"30 2 * * * cd /tmp && echo \"hi\"",
		"45 2 * * * cronitor --env prod --no-stdout exec abc123 /bin/echo already",
		"@hourly /usr/bin/true",
		"",
	}, "\n")

	first := parseContent(t, input)
	for _, line := range first.Lines {
		if !line.IsMonitorable() || line.Ignored || line.IsComment || line.Code != "" {
			continue
		}
		line.Code = "sync" + strconv.Itoa(line.LineNumber)
	}

	w1 := first.Write()
	if strings.Count(w1, "cronitor") < 3 {
		t.Fatalf("expected wrapped jobs in first write:\n%s", w1)
	}
	if strings.Contains(w1, "cronitor exec") && strings.Contains(w1, "cronitor --env prod exec cronitor") {
		t.Fatalf("double wrap in first write:\n%s", w1)
	}

	second := parseContent(t, w1)
	assertSameJobs(t, first, second)
	w2 := second.Write()
	if w1 != w2 {
		t.Fatalf("second sync changed the crontab\n--- first\n%s\n--- second\n%s", w1, w2)
	}

	third := parseContent(t, w2)
	w3 := third.Write()
	if w2 != w3 {
		t.Fatalf("third sync changed the crontab\n--- second\n%s\n--- third\n%s", w2, w3)
	}
	for _, line := range strings.Split(w3, "\n") {
		if strings.Count(line, " exec ") > 1 {
			t.Fatalf("double wrap: %s", line)
		}
	}
}

func TestRunAsUserStaysOutsideWrap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("system crontab user field is unix-only")
	}
	out, err := exec.Command("id", "-u", "root").Output()
	if err != nil {
		t.Skip("id -u root is not available")
	}
	if _, err := strconv.Atoi(strings.TrimSpace(string(out))); err != nil {
		t.Skip("root is not a uid")
	}

	line := Line{
		IsJob:          true,
		CronExpression: "0 * * * *",
		RunAs:          "root",
		CommandToRun:   `cd /tmp && echo "hi"`,
		Code:           "abc",
		Crontab:        Crontab{IsUserCrontab: false},
	}
	written := line.Write()
	if !strings.Contains(written, "0 * * * * root cronitor exec abc ") {
		t.Fatalf("runAs not kept in front of cronitor: %s", written)
	}
	job := parseJobLine(t, written)
	if job.RunAs != "root" || job.Code != "abc" || job.CommandToRun != line.CommandToRun {
		t.Fatalf("parsed runAs=%q code=%q command=%q", job.RunAs, job.Code, job.CommandToRun)
	}
	if job.Write() != written {
		t.Fatalf("runAs line not stable\n first: %s\nsecond: %s", written, job.Write())
	}
	requireShSyntax(t, skipWSFields(written, 6))
}

func parseContent(t *testing.T, content string) *Crontab {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "crontab")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	ct := CrontabFactory("crontab-wrap-test", path)
	err, code := ct.Parse(true)
	if err != nil {
		t.Fatalf("parse: %v (%d)", err, code)
	}
	return ct
}

func parseJobLine(t *testing.T, line string) *Line {
	t.Helper()
	ct := parseContent(t, line)
	var jobs []*Line
	for _, candidate := range ct.Lines {
		if candidate.IsJob {
			jobs = append(jobs, candidate)
		}
	}
	if len(jobs) != 1 {
		t.Fatalf("line %q parsed to %d jobs", line, len(jobs))
	}
	return jobs[0]
}

func rewriteCommand(t *testing.T, command string) (written, again, parsed string) {
	t.Helper()
	originalEnv := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", "")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", originalEnv) })

	line := Line{
		IsJob:          true,
		CronExpression: "0 * * * *",
		CommandToRun:   command,
		Code:           "k1",
		Crontab:        Crontab{IsUserCrontab: true},
	}
	written = line.Write()
	job := parseJobLine(t, written)
	return written, job.Write(), job.CommandToRun
}

func assertSameJobs(t *testing.T, before, after *Crontab) {
	t.Helper()
	bj, aj := jobLines(before), jobLines(after)
	if len(bj) != len(aj) {
		t.Fatalf("job count %d then %d", len(bj), len(aj))
	}
	for i := range bj {
		if bj[i].Code != aj[i].Code || bj[i].CommandToRun != aj[i].CommandToRun || bj[i].Ignored != aj[i].Ignored || bj[i].Name != aj[i].Name {
			t.Fatalf("job %d changed\n before code=%q cmd=%q name=%q ignored=%v\n after  code=%q cmd=%q name=%q ignored=%v",
				i, bj[i].Code, bj[i].CommandToRun, bj[i].Name, bj[i].Ignored,
				aj[i].Code, aj[i].CommandToRun, aj[i].Name, aj[i].Ignored)
		}
	}
}

func jobLines(ct *Crontab) []*Line {
	var jobs []*Line
	for _, line := range ct.Lines {
		if line.IsJob {
			jobs = append(jobs, line)
		}
	}
	return jobs
}

func commandFieldOf(crontabLine string) string {
	line := crontabLine
	if i := strings.LastIndex(crontabLine, "\n"); i >= 0 {
		line = crontabLine[i+1:]
	}
	fields := 5
	if strings.HasPrefix(strings.TrimSpace(line), "@") {
		fields = 1
	}
	return skipWSFields(line, fields)
}

func assertNoUnescapedPercent(t *testing.T, s string) {
	t.Helper()
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && (i == 0 || s[i-1] != '\\') {
			t.Fatalf("unescaped %% in %q", s)
		}
	}
}

func requireShSyntax(t *testing.T, commandField string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := exec.Command("sh", "-n", "-c", commandField).CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n failed: %v\n%s\nfield: %s", err, out, commandField)
	}
}
