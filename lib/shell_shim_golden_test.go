//go:build !windows

package lib

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite fixtures/shim/*.expected")

// TestShimFixtureGolden writes fixtures/shim/crontab.input with every
// monitorable job monitored, in default, --format=exec and --format=short mode, and compares the result to the .expected files.
func TestShimFixtureGolden(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("..", "fixtures", "shim", "crontab.input"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		mode    WriteMode
		convert bool
	}{
		{"crontab.expected", WriteModeShim, false},
		{"crontab.format-exec.expected", WriteModeExec, false},
		{"crontab.format-short.expected", WriteModeShim, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := writeFixture(t, string(input), tc.mode, tc.convert)
			path := filepath.Join("..", "fixtures", "shim", tc.name)
			if *updateGolden {
				if err := os.WriteFile(path, []byte(got), 0644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run go test ./lib -run TestShimFixtureGolden -update)", err)
			}
			if got != string(want) {
				t.Fatalf("output differs from %s (rerun with -update and review the diff)\n--- got\n%s", tc.name, got)
			}
			if again := writeFixture(t, got, tc.mode, tc.convert); again != got {
				t.Fatalf("second sync changed the output\n--- first\n%s\n--- second\n%s", got, again)
			}
		})
	}
}

func writeFixture(t *testing.T, content string, mode WriteMode, convert bool) string {
	t.Helper()
	ct := parseContent(t, content)
	ct.WriteMode = mode
	ct.ConvertExec = convert
	ct.ShimShellPath = "/etc/cronitor/shell"
	for i, l := range ct.Lines {
		if l.IsMonitorable() && !l.IsComment && l.GetCode() == "" {
			l.Code = fmt.Sprintf("new%02d", i)
		}
	}
	return ct.Write()
}
