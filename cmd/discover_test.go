package cmd

import (
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/stretchr/testify/assert"
)

func TestCreateDefaultNameHasAddCandidateSideEffect(t *testing.T) {
	allNameCandidates := map[string]bool{"something": true}

	// createDefaultName("/var/some/command arg1 arg2", "", 11, false, "localhost", nil, allNameCandidates)
	line := &lib.Line{
		Name:           "",
		FullLine:       "",
		LineNumber:     11,
		CronExpression: "",
		CommandToRun:   "/var/some/command arg1 arg2",
		Code:           "",
		RunAs:          "",
		Mon:            lib.Monitor{},
	}
	crontab := &lib.Crontab{
		User:                    "",
		IsUserCrontab:           false,
		IsSaved:                 false,
		Filename:                "",
		Lines:                   []*lib.Line{},
		TimezoneLocationName:    &lib.TimezoneLocationName{},
		UsesSixFieldExpressions: false,
	}

	_ = createDefaultName(
		line,
		crontab,
		"localhost",
		nil,
		allNameCandidates)

	assert.NotZero(t, allNameCandidates)
	assert.True(t, allNameCandidates["[localhost] /var/some/command arg1 arg2"])
}

func TestCreateDefaultName(t *testing.T) {
	tests := []struct {
		caseName              string
		command               string
		runAs                 string
		lineNumber            int
		isAutoDiscoverCommand bool
		hostname              string
		excludeFromName       []string
		allNameCandidates     map[string]bool
		expected              string
	}{
		{"short command",
			"/var/some/command arg1 arg2",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] /var/some/command arg1 arg2"},

		{"short command with name conflict",
			"/var/some/command arg1 arg2",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{
				"[localhost] /var/some/command arg1 arg2": true,
				"[localhost] cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2": true,
			},
			"[localhost] /var/some/command arg1 arg2 L11"},

		{"short command with runAs name",
			"/var/some/command arg1 arg2",
			"rando",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] rando /var/some/command arg1 arg2"},

		{"short command with runAs name doesn't conflict",
			"/var/some/command arg1 arg2",
			"rando",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{
				"[localhost] /var/some/command arg1 arg2": true,
				"[localhost] cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2": true,
			},
			"[localhost] rando /var/some/command arg1 arg2"},

		{"stdout redirection is trimmed from name",
			"/var/some/command arg1 arg2 > /dev/null",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] /var/some/command arg1 arg2"},

		{"stdout redirection append is trimmed from name",
			"/var/some/command arg1 arg2 >> /tmp/logfile",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] /var/some/command arg1 arg2"},

		{"output redirection is trimmed from name",
			"/var/some/command arg1 arg2 2>&1 > /dev/null",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] /var/some/command arg1 arg2"},

		{"excluded strings are removed from name",
			"/some/boilerplate/prefix /var/some/command arg1 argToRemove arg2",
			"",
			11,
			false,
			"localhost",
			[]string{"/some/boilerplate/prefix", "argToRemove"},
			map[string]bool{},
			"[localhost] /var/some/command arg1 arg2"},

		{"escape characters and quotes are removed from name",
			"/var/some/command \\'arg1\\' arg2",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] /var/some/command arg1 arg2"},

		{"hostname section is omitted when hostname is blank",
			"/var/some/command arg1 arg2",
			"",
			11,
			false,
			"",
			nil,
			map[string]bool{},
			"/var/some/command arg1 arg2"},

		{"auto discover name is created",
			"cronitor d3x0c1 cronitor discover --auto /discover/test",
			"",
			11,
			true,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] Auto discover /discover/test"},

		{"long names are truncated when command is long",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] cd /var/some/deeply/...directory/containing/command ; FOO=BAR run-command-here arg1 arg2"},

		{"exclusion text is applied before truncation",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"",
			11,
			false,
			"localhost",
			[]string{"/var/some/deeply/nested/"},
			map[string]bool{},
			"[localhost] cd custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2"},

		{"long command with name conflict",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{
				"[localhost] /var/some/command arg1 arg2": true,
				"[localhost] cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2": true,
			},
			"[localhost] cd /var/some/deeply/...ctory/containing/command ; FOO=BAR run-command-here arg1 arg2 L11"},

		{"long command with runAs",
			"cd /var/some/deeply/nested/custom/app/directory/containing/command ; FOO=BAR run-command-here arg1 arg2",
			"rando",
			11,
			false,
			"localhost",
			nil,
			map[string]bool{},
			"[localhost] rando cd /var/some/deeply/...ory/containing/command ; FOO=BAR run-command-here arg1 arg2"},
	}

	for _, tt := range tests {
		t.Run(tt.caseName, func(t *testing.T) {
			line := &lib.Line{
				Name:           "",
				FullLine:       "",
				LineNumber:     tt.lineNumber,
				CronExpression: "",
				CommandToRun:   tt.command,
				Code:           "",
				RunAs:          tt.runAs,
				Mon:            lib.Monitor{},
			}
			crontab := &lib.Crontab{
				User:                    tt.runAs,
				IsUserCrontab:           false,
				IsSaved:                 false,
				Filename:                "/discover/test",
				Lines:                   []*lib.Line{},
				TimezoneLocationName:    &lib.TimezoneLocationName{},
				UsesSixFieldExpressions: false,
			}

			got := createDefaultName(
				line,
				crontab,
				tt.hostname,
				tt.excludeFromName,
				tt.allNameCandidates)

			assert.Equal(t, tt.expected, got)
		})
	}
}
