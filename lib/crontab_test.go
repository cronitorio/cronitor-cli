package lib

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/viper"
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

func TestLineWriteWithEnvFlag(t *testing.T) {
	// Save original viper value
	originalEnv := viper.GetString("CRONITOR_ENV")

	// Test cases
	testCases := []struct {
		name          string
		envValue      string
		line          Line
		shouldHaveEnv bool
	}{
		{
			name:     "With environment set",
			envValue: "production",
			line: Line{
				IsJob:          true,
				CronExpression: "0 * * * *",
				CommandToRun:   "/path/to/script.sh",
				Code:           "abc123",
				Mon:            Monitor{Code: "abc123"},
			},
			shouldHaveEnv: true,
		},
		{
			name:     "Without environment set",
			envValue: "",
			line: Line{
				IsJob:          true,
				CronExpression: "0 * * * *",
				CommandToRun:   "/path/to/script.sh",
				Code:           "def456",
				Mon:            Monitor{Code: "def456"},
			},
			shouldHaveEnv: false,
		},
		{
			name:     "With staging environment",
			envValue: "staging",
			line: Line{
				IsJob:          true,
				CronExpression: "*/5 * * * *",
				CommandToRun:   "/usr/bin/backup.sh",
				Code:           "xyz789",
				Mon:            Monitor{Code: "xyz789"},
			},
			shouldHaveEnv: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set the environment value
			viper.Set("CRONITOR_ENV", tc.envValue)

			// Write the line
			output := tc.line.Write()

			// Check if --env flag is present
			hasEnvFlag := strings.Contains(output, "--env")
			if tc.shouldHaveEnv && !hasEnvFlag {
				t.Errorf("Expected --env flag in output but not found. Output: %s", output)
			}
			if !tc.shouldHaveEnv && hasEnvFlag {
				t.Errorf("Did not expect --env flag in output but found it. Output: %s", output)
			}

			// If env should be present, verify the value is included
			if tc.shouldHaveEnv {
				expectedEnvString := "--env " + tc.envValue
				if !strings.Contains(output, expectedEnvString) {
					t.Errorf("Expected '%s' in output but not found. Output: %s", expectedEnvString, output)
				}
			}

			// Verify the cronitor exec structure is correct
			if strings.Contains(output, "cronitor") {
				// Check the order: cronitor [--env <value>] [--no-stdout] exec <code> <command>
				parts := strings.Fields(output)
				cronitorIndex := -1
				for i, part := range parts {
					if part == "cronitor" {
						cronitorIndex = i
						break
					}
				}

				if cronitorIndex >= 0 {
					execIndex := -1
					for i := cronitorIndex + 1; i < len(parts); i++ {
						if parts[i] == "exec" {
							execIndex = i
							break
						}
					}

					if execIndex < 0 {
						t.Errorf("'exec' not found after 'cronitor' in output: %s", output)
					}

					// If env flag is present, it should come before 'exec'
					if tc.shouldHaveEnv {
						envFlagIndex := -1
						for i := cronitorIndex + 1; i < execIndex; i++ {
							if parts[i] == "--env" {
								envFlagIndex = i
								break
							}
						}
						if envFlagIndex < 0 {
							t.Errorf("--env flag should appear between 'cronitor' and 'exec'. Output: %s", output)
						}
					}
				}
			}
		})
	}

	// Restore original viper value
	viper.Set("CRONITOR_ENV", originalEnv)
}

func TestLineWriteWithNoStdoutAndEnv(t *testing.T) {
	// Save original viper value
	originalEnv := viper.GetString("CRONITOR_ENV")

	// Set environment
	viper.Set("CRONITOR_ENV", "testing")

	line := Line{
		IsJob:          true,
		CronExpression: "0 0 * * *",
		CommandToRun:   "/bin/daily-task",
		Code:           "test123",
		Mon:            Monitor{Code: "test123", NoStdoutPassthru: true},
	}

	output := line.Write()

	// Check that both flags are present and in correct order
	if !strings.Contains(output, "cronitor --env testing --no-stdout exec test123") {
		t.Errorf("Expected 'cronitor --env testing --no-stdout exec test123' in output but got: %s", output)
	}

	// Restore original viper value
	viper.Set("CRONITOR_ENV", originalEnv)
}
