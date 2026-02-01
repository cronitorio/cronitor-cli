package cmd

import (
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
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
