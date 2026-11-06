//go:build !windows

package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// The dashboard lists a job as monitored when the parsed line has a code, and
// shows CommandToRun. A line sync writes as CRONITOR=<key> must list as
// monitored, show the bare command, and never be wrapped a second time.
func TestDashMarkerLinesListAndSaveAsMonitored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req []struct {
			Key  string `json:"key"`
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type monitor struct {
			Attributes struct {
				Key  string `json:"key"`
				Code string `json:"code"`
			} `json:"attributes"`
		}
		resp := make([]monitor, len(req))
		for i, m := range req {
			resp[i].Attributes.Key = m.Key
			// The dashboard sends an existing monitor by key; the API returns
			// that monitor, so its code is unchanged.
			resp[i].Attributes.Code = m.Code
			if m.Code == "" {
				resp[i].Attributes.Code = m.Key
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	cleanup := setupIntegrationTest(srv.URL + "/api")
	defer cleanup()
	originalEnv := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", "")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", originalEnv) })

	lines := []struct {
		line, code, command string
	}{
		{"0 1 * * * CRONITOR=k1 /bin/true", "k1", "/bin/true"},
		{"0 2 * * * MONITORIO=k2 echo two", "k2", "echo two"},
		{"0 3 * * * CRONITOR=k3 CRONITOR_ENV=staging CRONITOR_NO_STDOUT=1 echo three", "k3", "echo three"},
		{"0 4 * * * cronitor exec k4 echo four", "k4", "echo four"},
		{"0 5 * * * echo five", "", "echo five"},
	}
	for _, tc := range lines {
		t.Run(tc.line, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "crontab")
			if err := os.WriteFile(path, []byte(tc.line+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			ct, err := lib.GetCrontab(path)
			if err != nil {
				t.Fatal(err)
			}
			var job *lib.Line
			for _, l := range ct.Lines {
				if l.IsJob {
					job = l
				}
			}
			if job == nil {
				t.Fatal("no job parsed")
			}
			// The fields handleGetJobs reads.
			if job.Code != tc.code || job.CommandToRun != tc.command {
				t.Fatalf("listed as code=%q command=%q, want code=%q command=%q", job.Code, job.CommandToRun, tc.code, tc.command)
			}
			if tc.code == "" {
				return
			}

			// Saving a monitored job (for example a rename) keeps the line as written.
			putDashJob(t, Job{CrontabFilename: path, Code: tc.code, Monitored: true, Command: tc.command, Expression: job.CronExpression, Key: job.Key(ct.CanonicalName())})
			got := strings.TrimSpace(readCrontab(t, path))
			if got != tc.line {
				t.Fatalf("save of a monitored job rewrote the line:\n got  %q\n want %q", got, tc.line)
			}
		})
	}
}
