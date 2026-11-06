package lib

import (
	"strings"
	"testing"
)

func TestSplitShimSettings(t *testing.T) {
	for _, tc := range []struct {
		in       string
		settings string
		job      string
	}{
		{"CRONITOR_ENV=staging ./b.sh", "CRONITOR_ENV=staging", "./b.sh"},
		{"CRONITOR_NO_STDOUT=1  CRONITOR_API_KEY=abc123\t./b.sh x", "CRONITOR_NO_STDOUT=1 CRONITOR_API_KEY=abc123", "./b.sh x"},
		{"CRONITOR_PING_API_KEY=p1 CRONITOR_HOSTNAME=web-1.local ./b.sh", "CRONITOR_PING_API_KEY=p1 CRONITOR_HOSTNAME=web-1.local", "./b.sh"},
		{"CRONITOR_FOO=1 ./b.sh", "", "CRONITOR_FOO=1 ./b.sh"},
		{"CRONITOR_ENV='a b' ./b.sh", "", "CRONITOR_ENV='a b' ./b.sh"},
		{"CRONITOR_ENV=$X ./b.sh", "", "CRONITOR_ENV=$X ./b.sh"},
		{"./b.sh CRONITOR_ENV=x", "", "./b.sh CRONITOR_ENV=x"},
		{"CRONITOR_ENV=staging", "", "CRONITOR_ENV=staging"},
		{"FOO=1 CRONITOR_ENV=x ./b.sh", "", "FOO=1 CRONITOR_ENV=x ./b.sh"},
	} {
		settings, job := SplitShimSettings(tc.in)
		if got := strings.Join(settings, " "); got != tc.settings || job != tc.job {
			t.Errorf("SplitShimSettings(%q) = %q, %q; want %q, %q", tc.in, got, job, tc.settings, tc.job)
		}
	}
}

func TestExecSettings(t *testing.T) {
	for _, tc := range []struct {
		line     string
		settings string
		ok       bool
	}{
		{"cronitor exec k1 /bin/true", "", true},
		{"cronitor exec --no-stdout k1 /bin/true", "CRONITOR_NO_STDOUT=1", true},
		{"cronitor --no-stdout exec k1 /bin/true", "CRONITOR_NO_STDOUT=1", true},
		{"cronitor --env staging exec k1 /bin/true", "CRONITOR_ENV=staging", true},
		{"cronitor --env=staging exec k1 /bin/true", "CRONITOR_ENV=staging", true},
		{"/usr/local/bin/cronitor -k abc123 exec k1 /bin/true", "CRONITOR_API_KEY=abc123", true},
		{"cronitor -kabc123 exec k1 /bin/true", "CRONITOR_API_KEY=abc123", true},
		{"cronitor --api-key=abc123 exec k1 /bin/true", "CRONITOR_API_KEY=abc123", true},
		{"cronitor -p p1 --hostname web1 exec k1 /bin/true", "CRONITOR_PING_API_KEY=p1 CRONITOR_HOSTNAME=web1", true},
		{"cronitor -n web1 exec --no-stdout k1 /bin/true", "CRONITOR_HOSTNAME=web1 CRONITOR_NO_STDOUT=1", true},
		{"cronitor --config /etc/x.json exec k1 /bin/true", "", false},
		{"cronitor -v exec k1 /bin/true", "", false},
	} {
		ct := parseContent(t, "0 * * * * "+tc.line)
		var line *Line
		for _, l := range ct.Lines {
			if l.IsJob {
				line = l
			}
		}
		if line == nil || line.Integration != IntegrationExec {
			t.Fatalf("%q: not parsed as an exec line", tc.line)
		}
		settings, ok := line.execSettings()
		if got := strings.Join(settings, " "); got != tc.settings || ok != tc.ok {
			t.Errorf("%q: execSettings = %q, %v; want %q, %v", tc.line, got, ok, tc.settings, tc.ok)
		}
	}
}
