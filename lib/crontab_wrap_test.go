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
		{"command args", `0 * * * * cronitor exec abc /usr/bin/true --flag arg`, "abc", "/usr/bin/true --flag arg", true},
		{"end of flags", `0 * * * * cronitor exec -- k1 /bin/true`, "k1", "/bin/true", true},
		{"value flag after exec is not consumed", `0 * * * * cronitor exec -c leftover k1 /bin/true`, "leftover", "k1 /bin/true", true},
		{"exec inside command", `0 * * * * cronitor exec abc /bin/echo exec me`, "abc", "/bin/echo exec me", true},
		{"reboot with flags", `@reboot cronitor --env prod exec abc /usr/bin/true`, "abc", "/usr/bin/true", true},
		{"not wrapped", `0 * * * * /usr/bin/true --flag`, "", "/usr/bin/true --flag", false},
		{"suffix is not the binary", `0 * * * * notcronitor exec abc /bin/true`, "", "notcronitor exec abc /bin/true", false},
		{"cronitor-helper", `0 * * * * cronitor-helper exec abc /bin/true`, "", "cronitor-helper exec abc /bin/true", false},
		{"discover subcommand", `30 * * * * cronitor discover /etc/crontab`, "", "cronitor discover /etc/crontab", false},
		{"simple quotes kept", `0 * * * * cronitor exec abc echo "hi there"`, "abc", `echo "hi there"`, true},
		{"quoted simple word kept", `0 * * * * cronitor exec k1 'my script.sh'`, "k1", `'my script.sh'`, true},
		{"escaped word kept", `0 * * * * cronitor exec k1 \%`, "k1", `\%`, true},
		{"quoted key", `0 * * * * cronitor exec 'k1' /bin/true`, "k1", "/bin/true", true},
		{"legacy quoted complex", `0 * * * * cronitor exec k1 "cd /tmp && echo \"hi\""`, "k1", `cd /tmp && echo "hi"`, true},
		{"unknown dash token is the key", `0 * * * * cronitor exec -zzz k1`, "-zzz", "k1", true},
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

func TestFlagTablesAndFallback(t *testing.T) {
	// The binary's exec scanner treats the first "exec" as the subcommand, so this
	// pings key "exec" and runs "realkey /bin/true".
	value := parseJobLine(t, `0 * * * * cronitor --env exec exec realkey /bin/true`)
	if value.Code != "exec" || value.CommandToRun != "realkey /bin/true" {
		t.Fatalf("value flag: code=%q command=%q", value.Code, value.CommandToRun)
	}

	boolFlag := parseJobLine(t, `0 * * * * cronitor exec --no-stdout d3x0c1 /path/to/command.sh`)
	if boolFlag.Code != "d3x0c1" || boolFlag.CommandToRun != "/path/to/command.sh" || !boolFlag.Mon.NoStdoutPassthru {
		t.Fatalf("bool flag: code=%q command=%q noStdout=%v", boolFlag.Code, boolFlag.CommandToRun, boolFlag.Mon.NoStdoutPassthru)
	}

	attached := parseJobLine(t, `0 * * * * cronitor -c/etc/alt.json leftover exec k1 /bin/true`)
	if attached.Code != "" {
		t.Fatalf("attached short flag was treated as wrapped, code=%q command=%q", attached.Code, attached.CommandToRun)
	}

	unparsed := parseJobLine(t, `0 * * * * cronitor exec k1 echo don't`)
	if unparsed.Code != "k1" || unparsed.CommandToRun != "echo don't" {
		t.Fatalf("unparseable tail: code=%q command=%q", unparsed.Code, unparsed.CommandToRun)
	}

	dashKey := parseJobLine(t, `0 * * * * cronitor exec -zzz k1`)
	if dashKey.Code != "-zzz" {
		t.Fatalf("dash token after exec was treated as a flag, code=%q", dashKey.Code)
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

func TestUnwrappedPercentRoundTrip(t *testing.T) {
	lines := []string{
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

func TestWrappedPercentMatchesOriginal(t *testing.T) {
	withCronitorEnv(t, "")

	commands := []string{
		`cat%hello`,
		`tr a-z A-Z%quiet please`,
		`date +%Y`,
		`date +\%Y`,
		`echo 100\%`,
		`printf '\%s\n' x`,
		`cd /tmp && date +\%Y`,
		`echo 100\% && true`,
		`printf '\%s\n' x%more`,
		`echo \\%hello`,
		`cd /tmp && cat%hello world`,
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			ct := parseContent(t, "0 * * * * "+command)
			job := jobLines(ct)[0]
			job.Code = "k1"
			w1 := ct.Write()
			if strings.Contains(w1, `\\%`) && !strings.Contains(command, `\\%`) {
				t.Fatalf("double escaped:\n%s", w1)
			}
			w2 := parseContent(t, w1).Write()
			if w1 != w2 {
				t.Fatalf("wrap changed\n first: %s\nsecond: %s", w1, w2)
			}
			want := cronRun(t, command)
			got := cronRun(t, commandFieldOf(w1))
			if got != want {
				t.Fatalf("cron run mismatch\n want %q\n  got %q\n line %s", want, got, w1)
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

// cronPercent is a standalone copy of cron's % rule. It must not call the
// production splitter: a broken backslash escape has to fail this oracle.
func cronPercent(field string) (command, stdin string) {
	cut := len(field)
	escaped := false
	for i := 0; i < len(field); i++ {
		if escaped {
			escaped = false
			continue
		}
		if field[i] == '\\' {
			escaped = true
			continue
		}
		if field[i] == '%' {
			cut = i
			break
		}
	}
	var cmd strings.Builder
	escaped = false
	for i := 0; i < cut; i++ {
		ch := field[i]
		if escaped {
			if ch != '%' {
				cmd.WriteByte('\\')
			}
			cmd.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		cmd.WriteByte(ch)
	}
	if escaped && cut == len(field) {
		cmd.WriteByte('\\')
	}
	if cut == len(field) {
		return cmd.String(), ""
	}
	var in strings.Builder
	escaped = false
	needNL := false
	for i := cut + 1; i < len(field); i++ {
		ch := field[i]
		if escaped {
			if ch != '%' {
				in.WriteByte('\\')
			}
		} else if ch == '%' {
			ch = '\n'
		}
		if escaped = ch == '\\'; !escaped {
			in.WriteByte(ch)
			needNL = ch != '\n'
		}
	}
	if escaped {
		in.WriteByte('\\')
	}
	if needNL {
		in.WriteByte('\n')
	}
	return cmd.String(), in.String()
}

func cronRun(t *testing.T, field string) string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	command, stdin := cronPercent(field)
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append(os.Environ(), "PATH="+cronitorStub(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "ERR " + err.Error() + "\n" + string(out)
	}
	return string(out)
}

func cronitorStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"seen=0\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$seen\" = 0 ]; then [ \"$1\" = exec ] && seen=1; shift; continue; fi\n" +
		"  case \"$1\" in --) shift; break ;; -*) shift ;; *) shift; break ;; esac\n" +
		"done\n" +
		"if [ $# -eq 1 ]; then exec sh -c \"$1\"; fi\n" +
		"if [ $# -gt 0 ]; then exec \"$@\"; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "cronitor"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func requireShSyntax(t *testing.T, commandField string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	command, stdin := cronPercent(commandField)
	if stdin != "" {
		return
	}
	out, err := exec.Command("sh", "-n", "-c", command).CombinedOutput()
	if err != nil {
		t.Fatalf("sh -n failed: %v\n%s\nfield: %s", err, out, commandField)
	}
}
