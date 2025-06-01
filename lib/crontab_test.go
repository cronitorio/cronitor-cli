package lib

import (
	"regexp"
	"strings"
	"testing"
)

func TestCronitorIgnoreComment(t *testing.T) {
	// Create a temporary crontab with cronitor: ignore comment
	crontabContent := `# cronitor: ignore
0 * * * * echo "this job should be ignored"

# Name: Test Job
0 * * * * echo "this job should not be ignored"`

	// Create a crontab object
	crontab := &Crontab{
		IsUserCrontab: true,
		Filename:      "test",
	}

	// Mock the load function by creating lines directly
	lines := strings.Split(crontabContent, "\n")

	// Parse the content
	var name string
	var ignored bool

	for lineNumber, fullLine := range lines {
		fullLine = strings.TrimSpace(fullLine)

		// Skip empty lines
		if fullLine == "" {
			continue
		}

		// Check for special Name: comment
		if nameMatch := regexp.MustCompile(`^#\s*Name:\s*(.+)$`).FindStringSubmatch(fullLine); nameMatch != nil {
			name = strings.TrimSpace(nameMatch[1])
			continue
		}

		// Check for special cronitor: ignore comment
		if ignoreMatch := regexp.MustCompile(`^#\s*cronitor:\s*ignore\s*$`).FindStringSubmatch(fullLine); ignoreMatch != nil {
			ignored = true
			continue
		}

		// Skip other comments
		if strings.HasPrefix(fullLine, "#") {
			continue
		}

		// Parse cron line
		splitLine := strings.Fields(fullLine)
		if len(splitLine) >= 6 {
			cronExpression := strings.Join(splitLine[0:5], " ")
			command := splitLine[5:]

			line := Line{
				IsJob:          true,
				Name:           name,
				CronExpression: cronExpression,
				CommandToRun:   strings.Join(command, " "),
				FullLine:       fullLine,
				LineNumber:     lineNumber,
				Ignored:        ignored,
				Crontab:        crontab.lightweightCopy(),
			}

			crontab.Lines = append(crontab.Lines, &line)

			// Reset for next line
			name = ""
			ignored = false
		}
	}

	// Verify that we have 2 lines
	if len(crontab.Lines) != 2 {
		t.Errorf("Expected 2 lines, got %d", len(crontab.Lines))
	}

	// Verify first line is marked as ignored
	firstLine := crontab.Lines[0]
	if !firstLine.Ignored {
		t.Errorf("Expected first line to be ignored, but Ignored was %v", firstLine.Ignored)
	}
	if firstLine.CommandToRun != `echo "this job should be ignored"` {
		t.Errorf("Expected first line command to be 'echo \"this job should be ignored\"', got '%s'", firstLine.CommandToRun)
	}

	// Verify second line is not ignored but has a name
	secondLine := crontab.Lines[1]
	if secondLine.Ignored {
		t.Errorf("Expected second line to not be ignored, but Ignored was %v", secondLine.Ignored)
	}
	if secondLine.Name != "Test Job" {
		t.Errorf("Expected second line name to be 'Test Job', got '%s'", secondLine.Name)
	}
	if secondLine.CommandToRun != `echo "this job should not be ignored"` {
		t.Errorf("Expected second line command to be 'echo \"this job should not be ignored\"', got '%s'", secondLine.CommandToRun)
	}

	// Test that Write() includes the cronitor: ignore comment
	firstLineOutput := firstLine.Write()
	if !strings.Contains(firstLineOutput, "# cronitor: ignore") {
		t.Errorf("Expected Write() output to contain '# cronitor: ignore', got: %s", firstLineOutput)
	}

	// Test that Write() for second line includes name but not ignore
	secondLineOutput := secondLine.Write()
	if !strings.Contains(secondLineOutput, "# Name: Test Job") {
		t.Errorf("Expected Write() output to contain '# Name: Test Job', got: %s", secondLineOutput)
	}
	if strings.Contains(secondLineOutput, "# cronitor: ignore") {
		t.Errorf("Expected Write() output to NOT contain '# cronitor: ignore' for second line, got: %s", secondLineOutput)
	}
}
