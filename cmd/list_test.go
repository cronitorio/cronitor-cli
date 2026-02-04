package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

func makeCrontab(filename string, isUser bool, lines []*lib.Line) *lib.Crontab {
	ct := &lib.Crontab{
		Filename:      filename,
		IsUserCrontab: isUser,
	}
	ct.Lines = lines
	return ct
}

func makeLine(expr, command string) *lib.Line {
	return &lib.Line{
		CronExpression: expr,
		CommandToRun:   command,
		IsJob:          true,
		LineNumber:     1,
		FullLine:       expr + " " + command,
	}
}

func TestJobLines(t *testing.T) {
	tables := []struct {
		name     string
		lines    []*lib.Line
		expected int
	}{
		{
			"filters lines with no command",
			[]*lib.Line{
				{CommandToRun: "/usr/bin/backup"},
				{CommandToRun: ""},
				{CommandToRun: "/usr/bin/cleanup"},
			},
			2,
		},
		{
			"returns empty for no lines",
			[]*lib.Line{},
			0,
		},
		{
			"returns empty when all lines lack commands",
			[]*lib.Line{
				{CommandToRun: ""},
				{CommandToRun: ""},
			},
			0,
		},
		{
			"returns all when all lines have commands",
			[]*lib.Line{
				{CommandToRun: "/usr/bin/a"},
				{CommandToRun: "/usr/bin/b"},
			},
			2,
		},
	}

	for _, tt := range tables {
		ct := makeCrontab("/etc/crontab", false, tt.lines)
		got := jobLines(ct)
		if len(got) != tt.expected {
			t.Errorf("jobLines %q: got %d lines, expected %d", tt.name, len(got), tt.expected)
		}
	}
}

func TestPrintListAsJSONValidOutput(t *testing.T) {
	crontabs := []*lib.Crontab{
		makeCrontab("/etc/crontab", false, []*lib.Line{
			makeLine("0 * * * *", "/usr/bin/backup"),
			makeLine("30 2 * * *", "/usr/bin/cleanup"),
		}),
	}

	var buf bytes.Buffer
	printListAsJSON(&buf, crontabs)

	if buf.Len() == 0 {
		t.Fatal("printListAsJSON produced no output")
	}

	if !json.Valid(buf.Bytes()) {
		t.Fatalf("printListAsJSON produced invalid JSON: %s", buf.String())
	}
}

func TestPrintListAsJSONStructure(t *testing.T) {
	crontabs := []*lib.Crontab{
		makeCrontab("/etc/crontab", false, []*lib.Line{
			makeLine("0 * * * *", "/usr/bin/backup"),
		}),
	}

	var buf bytes.Buffer
	printListAsJSON(&buf, crontabs)

	var parsed []json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected JSON array, got error: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("expected 1 crontab in output, got %d", len(parsed))
	}

	// Verify we can decode the crontab into a map with expected keys
	var ct map[string]interface{}
	if err := json.Unmarshal(parsed[0], &ct); err != nil {
		t.Fatalf("failed to decode crontab: %v", err)
	}

	requiredKeys := []string{"filename", "display_name", "lines"}
	for _, key := range requiredKeys {
		if _, ok := ct[key]; !ok {
			t.Errorf("expected key %q in JSON output, not found", key)
		}
	}

	lines, ok := ct["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected 'lines' to be an array")
	}
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}
}

func TestPrintListAsJSONEmptyCrontabs(t *testing.T) {
	var buf bytes.Buffer
	printListAsJSON(&buf, []*lib.Crontab{})

	var parsed []json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON array for empty input, got error: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty array, got %d elements", len(parsed))
	}
}

func TestPrintListAsJSONNilCrontabs(t *testing.T) {
	var buf bytes.Buffer
	printListAsJSON(&buf, nil)

	var parsed []json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("expected valid JSON array for nil input, got error: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty array for nil input, got %d elements", len(parsed))
	}
}

func TestPrintListAsJSONSkipsEmptyCrontabs(t *testing.T) {
	crontabs := []*lib.Crontab{
		makeCrontab("/etc/crontab", false, []*lib.Line{
			makeLine("0 * * * *", "/usr/bin/backup"),
		}),
		// This crontab has no job lines (empty command)
		makeCrontab("/etc/cron.d/empty", false, []*lib.Line{
			{CommandToRun: "", CronExpression: "0 * * * *"},
		}),
		// This crontab has no lines at all
		makeCrontab("/etc/cron.d/nada", false, []*lib.Line{}),
	}

	var buf bytes.Buffer
	printListAsJSON(&buf, crontabs)

	var parsed []json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(parsed) != 1 {
		t.Errorf("expected 1 crontab (skipping 2 empty ones), got %d", len(parsed))
	}
}

func TestPrintListAsJSONContainsCommandText(t *testing.T) {
	crontabs := []*lib.Crontab{
		makeCrontab("/etc/crontab", false, []*lib.Line{
			makeLine("0 * * * *", "/usr/bin/test arg1 arg2"),
		}),
	}

	var buf bytes.Buffer
	printListAsJSON(&buf, crontabs)

	if !bytes.Contains(buf.Bytes(), []byte("/usr/bin/test arg1 arg2")) {
		t.Errorf("expected command in JSON output, got: %s", buf.String())
	}
}

func TestPrintListAsTableNoOutput(t *testing.T) {
	// Table output for empty crontabs should produce minimal output
	crontabs := []*lib.Crontab{
		makeCrontab("/etc/cron.d/empty", false, []*lib.Line{}),
	}

	var buf bytes.Buffer
	printListAsTable(&buf, crontabs)

	// Should only contain the leading newline, no table rendered
	if buf.String() != "\n" {
		t.Errorf("expected only newline for empty crontab table output, got: %q", buf.String())
	}
}

func TestPrintListAsTableIncludesCommands(t *testing.T) {
	crontabs := []*lib.Crontab{
		makeCrontab("/etc/crontab", false, []*lib.Line{
			makeLine("0 * * * *", "/usr/bin/backup"),
			makeLine("30 2 * * *", "/usr/bin/cleanup"),
		}),
	}

	var buf bytes.Buffer
	printListAsTable(&buf, crontabs)

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("/usr/bin/backup")) {
		t.Error("table output missing /usr/bin/backup")
	}
	if !bytes.Contains([]byte(output), []byte("/usr/bin/cleanup")) {
		t.Error("table output missing /usr/bin/cleanup")
	}
	if !bytes.Contains([]byte(output), []byte("0 * * * *")) {
		t.Error("table output missing schedule '0 * * * *'")
	}
}
