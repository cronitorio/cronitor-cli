package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/kballard/go-shellquote"
)

func TestCreateDefaultNameHasAddCandidateSideEffect(t *testing.T) {
	allNameCandidates := map[string]bool{"something": true}
	line := &lib.Line{
		CommandToRun: "/var/some/command arg1 arg2",
		LineNumber:   11,
	}
	crontab := &lib.Crontab{Filename: "/discover/test"}
	createDefaultName(line, crontab, "localhost", nil, allNameCandidates)

	if len(allNameCandidates) == 0 || allNameCandidates["[localhost] /var/some/command arg1 arg2"] != true {
		t.Error("Name candidate not added to allNameCandidates")
	}
}

func TestCreateDefaultName(t *testing.T) {
	allNameCandidates := map[string]bool{
		"[localhost] /var/some/command arg1 arg2": true,
		"[localhost] cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2": true,
	}

	tables := []struct {
		caseName          string
		command           string
		runAs             string
		lineNumber        int
		hostname          string
		excludeFromName   []string
		allNameCandidates map[string]bool
		crontabFilename   string
		expected          string
	}{
		{"short command",
			"/var/some/command arg1 arg2",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2"},

		{"short command with name conflict",
			"/var/some/command arg1 arg2",
			"",
			11,
			"localhost",
			nil,
			allNameCandidates,
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2 L11"},

		{"short command with runAs name",
			"/var/some/command arg1 arg2",
			"rando",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] rando /var/some/command arg1 arg2"},

		{"short command with runAs name doesn't conflict",
			"/var/some/command arg1 arg2",
			"rando",
			11,
			"localhost",
			nil,
			allNameCandidates,
			"/discover/test",
			"[localhost] rando /var/some/command arg1 arg2"},

		{"stdout redirection is trimmed from name",
			"/var/some/command arg1 arg2 > /dev/null",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2"},

		{"stdout redirection append is trimmed from name",
			"/var/some/command arg1 arg2 >> /tmp/logfile",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2"},

		{"output redirection is trimmed from name",
			"/var/some/command arg1 arg2 2>&1 > /dev/null",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2"},

		{"excluded strings are removed from name",
			"/some/boilerplate/prefix /var/some/command arg1 argToRemove arg2",
			"",
			11,
			"localhost",
			[]string{"/some/boilerplate/prefix", "argToRemove"},
			map[string]bool{},
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2"},

		{"escape characters and quotes are removed from name",
			"/var/some/command \\'arg1\\' arg2",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] /var/some/command arg1 arg2"},

		{"hostname section is omitted when hostname is blank",
			"/var/some/command arg1 arg2",
			"",
			11,
			"",
			nil,
			map[string]bool{},
			"/discover/test",
			"/var/some/command arg1 arg2"},

		{"auto discover name is created",
			"cronitor d3x0c1 cronitor discover --auto /discover/test",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] Auto discover /discover/test"},

		{"long names are truncated when command is long",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] cd /var/some/deeply/...; FOO=BAR run-command-here arg1 arg2 L11"},

		{"exclusion text is applied before truncation",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"",
			11,
			"localhost",
			[]string{"/var/some/deeply/nested/"},
			map[string]bool{},
			"/discover/test",
			"[localhost] cd custom/app/direct...; FOO=BAR run-command-here arg1 arg2 L11"},

		{"long command with name conflict",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"",
			11,
			"localhost",
			nil,
			allNameCandidates,
			"/discover/test",
			"[localhost] cd /var/some/deeply/...O=BAR run-command-here arg1 arg2 L11 L11"},

		{"long command with runAs",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"rando",
			11,
			"localhost",
			nil,
			map[string]bool{},
			"/discover/test",
			"[localhost] rando cd /var/some/deeply/...BAR run-command-here arg1 arg2 L11"},
	}

	for _, table := range tables {
		line := &lib.Line{
			CommandToRun: table.command,
			RunAs:        table.runAs,
			LineNumber:   table.lineNumber,
		}
		crontab := &lib.Crontab{Filename: table.crontabFilename}
		defaultName := createDefaultName(line, crontab, table.hostname, table.excludeFromName, table.allNameCandidates)
		if defaultName != table.expected {
			t.Errorf("Test case '%s' failed, got: %s, expected: %s.", table.caseName, defaultName, table.expected)
		}
	}
}

func TestCreateDefaultNameAutoDiscover(t *testing.T) {
	line := &lib.Line{
		CommandToRun: "cronitor d3x0c1 cronitor discover --auto /discover/test",
		LineNumber:   11,
		RunAs:        "",
	}
	crontab := &lib.Crontab{
		Filename: "/discover/test",
	}

	defaultName := createDefaultName(line, crontab, "localhost", nil, map[string]bool{})

	expected := "[localhost] Auto discover /discover/test"
	if defaultName != expected {
		t.Errorf("Auto discover test failed, got: %s, expected: %s.", defaultName, expected)
	}
}

func TestDiscoverMissingAPIKeyGuidancePointsAtAuthLogin(t *testing.T) {
	msg := discoverMissingAPIKeyGuidance()
	if !strings.Contains(msg, "cronitor auth login") {
		t.Errorf("missing-key copy should point at auth login:\n%s", msg)
	}
	if !strings.Contains(msg, "cronitor signup") {
		t.Errorf("missing-key copy should still mention signup as an alias:\n%s", msg)
	}
	if !strings.Contains(msg, "device authorization") || !strings.Contains(msg, "alias") {
		t.Errorf("missing-key copy should describe the device-auth alias, not a separate sign-up:\n%s", msg)
	}
	if strings.Contains(msg, "to create an account") {
		t.Errorf("missing-key copy still implies the old website sign-up path:\n%s", msg)
	}
}

func monitorWith(name, key, code string) lib.Monitor {
	m := lib.Monitor{Name: name}
	m.Attributes.Key = key
	m.Attributes.Code = code
	return m
}

// The monitor being renamed is not a duplicate of itself.
func TestHasMonitorByNameSkipsCurrent(t *testing.T) {
	em := ExistingMonitors{Monitors: []lib.Monitor{
		monitorWith("Other", "other", "zzz999"),
		monitorWith("Prune events", "prune-events", "abc123"),
	}}
	em.CurrentKey, em.CurrentCode = "hash-of-line", "prune-events"
	if em.HasMonitorByName("Prune events") {
		t.Fatal("HasMonitorByName counted the current monitor as a duplicate")
	}
	if !em.HasMonitorByName("Other") {
		t.Fatal("HasMonitorByName missed a different monitor's name")
	}
}

func TestOptInAutoDiscoverLine(t *testing.T) {
	t.Cleanup(resetSyncFlags)
	legacy := "15 * * * * cronitor discover --auto /etc/crontab\n"
	for _, tc := range []struct {
		name, file, extra, host, ping string
		argv                          []string
		rel                           bool
	}{
		{"meta-link", "cron.daily", "0 * * * * echo hello\n", "", "eu.cronitor.link", []string{"jobs", "sync", "--no-auto-discover=false", "--ping-api-host", "eu.cronitor.link"}, false},
		{"hostname-sync", "crontab", "", "sync", "", []string{"jobs", "--hostname", "sync", "sync", "--no-auto-discover=false"}, false},
		{"vn-sync", "crontab", "", "sync", "", []string{"jobs", "-vn", "sync", "sync", "--no-auto-discover=false"}, false},
		{"relative-jobs", "jobs", "", "", "", []string{"jobs", "sync", "--no-auto-discover=false"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.file), []byte(legacy+tc.extra), 0644); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(dir, tc.file)
			if tc.rel {
				t.Chdir(dir)
				filename = tc.file
			}
			resetSyncFlags()
			cmd, rest, err := RootCmd.Find(append(append([]string{}, tc.argv...), filename))
			if err != nil || cmd != discoverCmd || cmd.ParseFlags(rest) != nil {
				t.Fatal(err)
			}
			ct := lib.CrontabFactory("rewrite", filename)
			if err, code := ct.Parse(false, autoDiscoverFlagArgs()...); err != nil {
				t.Fatalf("%v %d", err, code)
			}
			got := ct.Write()
			words, err := shellquote.Split(strings.TrimPrefix(strings.Split(got, "\n")[0], "15 * * * * "))
			abs, _ := filepath.Abs(filename)
			if err != nil || len(words) < 4 || words[1] != "jobs" || words[2] != "discover" || words[3] != "--auto" || words[len(words)-1] != abs || strings.Contains(got, "/etc/crontab") || strings.Count(got, "discover --auto") != 1 || tc.host != "" && !strings.Contains(got, "--hostname "+tc.host) || tc.ping != "" && !strings.Contains(got, tc.ping) || tc.extra != "" && !strings.Contains(got, "echo hello") {
				t.Fatal(got)
			}
		})
	}
}

func resetSyncFlags() {
	hostname, pingApiHostFlag, environment, users, apiKey, pingApiKey, debugLog, cfgFile = "", "", "", "", "", "", "", ""
	notificationList, syncFormat, excludeFromName = "", "", nil
	verbose, noStdoutPassthru, noAutoDiscover, dev, isSilent = false, false, true, false, false
}
