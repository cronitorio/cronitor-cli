package lib

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestWrapDetection(t *testing.T) {
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
		{"no stdout after exec", `0 * * * * cronitor exec --no-stdout d3x0c1 /path/to/command.sh`, "d3x0c1", "/path/to/command.sh", true},
		{"unparseable tail", `0 * * * * cronitor exec k1 echo don't`, "k1", "echo don't", true},
		{"monicli", `0 * * * * monicli exec k1 /bin/true`, "k1", "/bin/true", true},
		{"monicli path and env", `0 * * * * /usr/local/bin/monicli --env prod exec --no-stdout k1 /bin/true`, "k1", "/bin/true", true},
		{"monicli exe", `0 * * * * monicli.exe exec k1 /bin/true`, "k1", "/bin/true", true},
		{"command args", `0 * * * * cronitor exec abc /usr/bin/true --flag arg`, "abc", "/usr/bin/true --flag arg", true},
		{"end of flags", `0 * * * * cronitor exec -- k1 /bin/true`, "k1", "/bin/true", true},
		{"exec inside command", `0 * * * * cronitor exec abc /bin/echo exec me`, "abc", "/bin/echo exec me", true},
		{"reboot with flags", `@reboot cronitor --env prod exec abc /usr/bin/true`, "abc", "/usr/bin/true", true},
		{"not wrapped", `0 * * * * /usr/bin/true --flag`, "", "/usr/bin/true --flag", false},
		{"suffix is not the binary", `0 * * * * notcronitor exec abc /bin/true`, "", "notcronitor exec abc /bin/true", false},
		{"cronitor-helper", `0 * * * * cronitor-helper exec abc /bin/true`, "", "cronitor-helper exec abc /bin/true", false},
		{"unknown flag takes no value", `0 * * * * cronitor --bogus v exec k1 /bin/true`, "", "cronitor --bogus v exec k1 /bin/true", false},
		{"discover subcommand", `30 * * * * cronitor discover /etc/crontab`, "", "cronitor discover /etc/crontab", false},
		{"simple quotes kept", `0 * * * * cronitor exec abc echo "hi there"`, "abc", `echo "hi there"`, true},
		{"quoted simple word kept", `0 * * * * cronitor exec k1 'my script.sh'`, "k1", `'my script.sh'`, true},
		{"escaped word kept", `0 * * * * cronitor exec k1 \%`, "k1", `\%`, true},
		{"quoted key", `0 * * * * cronitor exec 'k1' /bin/true`, "k1", "/bin/true", true},
		{"legacy quoted complex", `0 * * * * cronitor exec k1 "cd /tmp && echo \"hi\""`, "k1", `cd /tmp && echo "hi"`, true},
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
	withCronitorEnv(t, "")
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
			line := Line{
				IsJob: true, CronExpression: "0 * * * *", CommandToRun: command,
				Code: "k1", Crontab: Crontab{IsUserCrontab: true},
			}
			written := line.Write()
			job := parseJobLine(t, written)
			if job.Write() != written {
				t.Fatalf("rewrite changed bytes\n first: %s\nsecond: %s", written, job.Write())
			}
			if job.CommandToRun != command {
				t.Fatalf("command changed\n want: %q\n  got: %q\n line: %s", command, job.CommandToRun, written)
			}
			requireShSyntax(t, commandFieldOf(written))
		})
	}
}

func TestQuotedWordRoundTrip(t *testing.T) {
	withCronitorEnv(t, "")

	lines := []string{
		`0 * * * * cronitor exec k1 'my script.sh'`,
		`0 * * * * cronitor exec k1 \%`,
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			written := strings.TrimSpace(parseContent(t, line).Write())
			if written != line {
				t.Fatalf("quoted word changed\n got %s\nwant %s", written, line)
			}
		})
	}
}

func TestUnmonitoredLineRoundTrip(t *testing.T) {
	lines := []string{
		`0 * * * * echo "a  b"`,
		`0 * * * * "/opt/my script.sh"`,
		`0 * * * * /bin/true  arg`,
		`# 0 * * * * echo "a  b"`,
		`0 * * * * run-parts  /etc/cron.hourly`,
		`0 * * * * cat%hello`,
		`0 * * * * tr a-z A-Z%quiet please`,
		`0 * * * * date +%Y`,
		`0 * * * * date +\%Y`,
		`0 * * * * echo 100\%`,
		`0 * * * * printf '\%s\n' x%more`,
		`0 * * * * echo \\%hello`,
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			got := strings.TrimSpace(parseContent(t, line).Write())
			if got != line {
				t.Fatalf("unmonitored line changed\n got %s\nwant %s", got, line)
			}
		})
	}
}

// TestRawCommandIsKept checks that the command bytes from the file are reused
// while the command is not edited, and that an edit wins.
func TestRawCommandIsKept(t *testing.T) {
	withCronitorEnv(t, "")
	tests := []struct {
		name, line string
		edit       func(*Line)
		want       string
	}{
		{"wrap quoted path", `0 * * * * "/opt/my script.sh"`,
			func(l *Line) { l.Code = "NEW" }, `0 * * * * cronitor exec NEW "/opt/my script.sh"`},
		{"wrap keeps spaces in quotes", `0 * * * * echo "a  b"`,
			func(l *Line) { l.Code = "NEW" }, `0 * * * * cronitor exec NEW echo "a  b"`},
		{"wrapped keeps spaces in quotes", `0 * * * * cronitor exec k1 echo "a  b"`,
			func(l *Line) {}, `0 * * * * cronitor exec k1 echo "a  b"`},
		{"key change keeps tail", `0 * * * * cronitor exec k1 echo "a  b"`,
			func(l *Line) { l.Code = "NEW" }, `0 * * * * cronitor exec NEW echo "a  b"`},
		{"disable unquotes complex", `0 * * * * cronitor exec k1 'cd /tmp && ls'`,
			func(l *Line) { l.Code = "" }, `0 * * * * cd /tmp && ls`},
		{"wrap keeps spaces in complex", `0 2 * * * echo "two  spaces"   >  /dev/null; echo after-redirect`,
			func(l *Line) { l.Code = "K" }, `0 2 * * * cronitor exec K 'echo "two  spaces"   >  /dev/null; echo after-redirect'`},
		// Cron cuts at the first bare %, even inside quotes, so the original runs
		// `printf '` with the rest as stdin. The wrap keeps that cut where it is.
		{"wrap bare percent in quotes", `0 6 * * * printf '%s\n' a b | tr a-z A-Z`,
			func(l *Line) { l.Code = "K" }, `0 6 * * * cronitor exec K printf '%s\n' a b | tr a-z A-Z`},
		{"edit unwrapped", `0 * * * * echo "a  b"`,
			func(l *Line) { l.CommandToRun = "/bin/other" }, `0 * * * * /bin/other`},
		{"edit wrapped", `0 * * * * cronitor exec k1 echo "a  b"`,
			func(l *Line) { l.CommandToRun = "/bin/other" }, `0 * * * * cronitor exec k1 /bin/other`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ct := parseContent(t, tc.line)
			tc.edit(jobLines(ct)[0])
			got := strings.TrimSpace(ct.Write())
			if got != tc.want {
				t.Fatalf("\n got %s\nwant %s", got, tc.want)
			}
			if again := strings.TrimSpace(parseContent(t, got).Write()); again != got {
				t.Fatalf("not stable\n%s\n%s", got, again)
			}
		})
	}
}

// TestWrappedPercent checks wraps of lines with %. Cron cuts stdin at the
// first % not escaped by a backslash (\\% is an escaped backslash and then a
// bare %) and removes the backslash from \%. The wrap must keep that cut in
// the same place and must not add or remove escapes.
func TestWrappedPercent(t *testing.T) {
	withCronitorEnv(t, "")

	tests := []struct{ command, want string }{
		{`cat%hello`, `cronitor exec k1 cat%hello`},
		{`tr a-z A-Z%quiet please`, `cronitor exec k1 tr a-z A-Z%quiet please`},
		{`date +%Y`, `cronitor exec k1 date +%Y`},
		{`date +\%Y`, `cronitor exec k1 date +\%Y`},
		{`echo 100\%`, `cronitor exec k1 echo 100\%`},
		{`printf '\%s\n' x`, `cronitor exec k1 printf '\%s\n' x`},
		{`cd /tmp && date +\%Y`, `cronitor exec k1 'cd /tmp && date +\%Y'`},
		{`echo 100\% && true`, `cronitor exec k1 'echo 100\% && true'`},
		{`printf '\%s\n' x%more`, `cronitor exec k1 printf '\%s\n' x%more`},
		{`echo \\%hello`, `cronitor exec k1 echo \\%hello`},
		{`cd /tmp && cat%hello world`, `cronitor exec k1 'cd /tmp && cat'%hello world`},
		{`cd /tmp && echo \\%hello`, `cronitor exec k1 'cd /tmp && echo \\'%hello`},
		{`cd /tmp && echo \%hello`, `cronitor exec k1 'cd /tmp && echo \%hello'`},
	}
	for _, tc := range tests {
		t.Run(tc.command, func(t *testing.T) {
			ct := parseContent(t, "0 * * * * "+tc.command)
			jobLines(ct)[0].Code = "k1"
			w1 := strings.TrimSpace(ct.Write())
			if want := "0 * * * * " + tc.want; w1 != want {
				t.Fatalf("\n got %s\nwant %s", w1, want)
			}
			if w2 := strings.TrimSpace(parseContent(t, w1).Write()); w2 != w1 {
				t.Fatalf("wrap changed\n first: %s\nsecond: %s", w1, w2)
			}
		})
	}
}

func TestSyncFlagsMergeIntoPrefix(t *testing.T) {
	withCronitorEnv(t, "prod")

	cases := []struct{ in, want string }{
		{
			`0 * * * * cronitor exec k1 /bin/true`,
			`0 * * * * cronitor --env prod exec --no-stdout k1 /bin/true`,
		},
		{
			`5 * * * * /opt/cronitor --env staging -c alt.json exec k2 /bin/echo hello`,
			`5 * * * * /opt/cronitor --env staging -c alt.json exec --no-stdout k2 /bin/echo hello`,
		},
		{
			`10 * * * * /opt/cronitor --env staging -c alt.json --no-stdout exec k3 /bin/true`,
			`10 * * * * /opt/cronitor --env staging -c alt.json exec --no-stdout k3 /bin/true`,
		},
		{
			`15 * * * * cronitor --no-stdout exec k4 /bin/true`,
			`15 * * * * cronitor --env prod exec --no-stdout k4 /bin/true`,
		},
		{
			`20 * * * * /opt/cronitor   --env staging exec k5 /bin/true`,
			`20 * * * * /opt/cronitor --env staging exec --no-stdout k5 /bin/true`,
		},
		{
			`25 * * * * /usr/local/bin/monicli exec k6 /bin/true`,
			`25 * * * * /usr/local/bin/monicli --env prod exec --no-stdout k6 /bin/true`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			ct := parseContent(t, tc.in)
			job := jobLines(ct)[0]
			job.Mon.NoStdoutPassthru = true
			written := strings.TrimSpace(ct.Write())
			if written != tc.want {
				t.Fatalf("sync flags\n got %s\nwant %s", written, tc.want)
			}
			if strings.Count(written, "--env") != 1 || strings.Count(written, "--no-stdout") != 1 {
				t.Fatalf("duplicated sync flags: %s", written)
			}
			again := strings.TrimSpace(parseContent(t, written).Write())
			if again != written {
				t.Fatalf("second sync changed bytes\n%s\n%s", written, again)
			}
		})
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
	jobs := jobLines(parseContent(t, line))
	if len(jobs) != 1 {
		t.Fatalf("line %q parsed to %d jobs", line, len(jobs))
	}
	return jobs[0]
}

func withCronitorEnv(t *testing.T, env string) {
	t.Helper()
	prev := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", env)
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", prev) })
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
	line := strings.TrimRight(crontabLine, "\n")
	if i := strings.LastIndex(line, "\n"); i >= 0 {
		line = line[i+1:]
	}
	fields := 5
	if strings.HasPrefix(strings.TrimSpace(line), "@") {
		fields = 1
	}
	return skipWSFields(line, fields)
}

func requireShSyntax(t *testing.T, commandField string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	// Skip lines with cron stdin; unescape \% the way cron does.
	if strings.Contains(strings.ReplaceAll(commandField, `\%`, ""), "%") {
		return
	}
	command := strings.ReplaceAll(commandField, `\%`, "%")
	out, err := exec.Command("sh", "-n", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n failed: %v\n%s\nfield: %s", err, out, commandField)
	}
}
