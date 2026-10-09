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

func TestDashDisableReenableAndKeyChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req []struct {
			Key string `json:"key"`
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
			resp[i].Attributes.Code = "NEWCODE"
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cleanup := setupIntegrationTest(srv.URL + "/api")
	defer cleanup()
	withCronitorEnv(t, "")

	plain := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(plain, []byte("0 * * * * cronitor exec k1 /bin/true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: plain, Code: "k1", Monitored: false, Command: "/bin/true", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, plain)); got != "0 * * * * /bin/true" {
		t.Fatalf("disable wrote %q", got)
	}

	ct, err := lib.GetCrontab(plain)
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for _, line := range ct.Lines {
		if line.IsJob {
			key = line.Key(ct.CanonicalName())
		}
	}
	putDashJob(t, Job{CrontabFilename: plain, Key: key, Monitored: true, Command: "/bin/true", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, plain)); got != "0 * * * * cronitor exec NEWCODE /bin/true" {
		t.Fatalf("re-enable wrote %q", got)
	}

	prefixed := filepath.Join(t.TempDir(), "crontab")
	original := "0 * * * * /opt/cronitor --env staging -c alt.json exec k1 /bin/true\n"
	if err := os.WriteFile(prefixed, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: prefixed, Code: "k1", Monitored: true, Command: "/bin/true", Expression: "0 * * * *"})
	want := "0 * * * * /opt/cronitor --env staging -c alt.json exec NEWCODE /bin/true"
	if got := strings.TrimSpace(readCrontab(t, prefixed)); got != want {
		t.Fatalf("key change wrote %q", got)
	}
}

func TestDashShimDisableReenableAndKeyChange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req []struct {
			Key string `json:"key"`
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
			resp[i].Attributes.Code = "NEWCODE"
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cleanup := setupIntegrationTest(srv.URL + "/api")
	defer cleanup()
	originalEnv := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", "")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", originalEnv) })

	plain := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(plain, []byte("0 * * * * MONITORIO=k1 /bin/true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: plain, Code: "k1", Monitored: false, Command: "/bin/true", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, plain)); got != "0 * * * * /bin/true" {
		t.Fatalf("disable wrote %q", got)
	}

	ct, err := lib.GetCrontab(plain)
	if err != nil {
		t.Fatal(err)
	}
	var key string
	for _, line := range ct.Lines {
		if line.IsJob {
			key = line.Key(ct.CanonicalName())
		}
	}
	putDashJob(t, Job{CrontabFilename: plain, Key: key, Monitored: true, Command: "/bin/true", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, plain)); got != "0 * * * * cronitor exec NEWCODE /bin/true" {
		t.Fatalf("re-enable wrote %q", got)
	}

	percent := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(percent, []byte("0 * * * * MONITORIO=k1 date +%Y\n"), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: percent, Code: "k1", Monitored: false, Command: "date +%Y", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, percent)); got != "0 * * * * date +%Y" {
		t.Fatalf("disable rewrote a bare percent: %q", got)
	}

	escaped := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(escaped, []byte("0 * * * * MONITORIO=k1 date +\\%Y\n"), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: escaped, Code: "k1", Monitored: false, Command: `date +\%Y`, Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, escaped)); got != `0 * * * * date +\%Y` {
		t.Fatalf("disable rewrote an escaped percent: %q", got)
	}

	keyed := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(keyed, []byte("0 * * * * MONITORIO=k1 /bin/true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: keyed, Code: "k1", Monitored: true, Command: "/bin/true", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, keyed)); got != "0 * * * * MONITORIO=NEWCODE /bin/true" {
		t.Fatalf("key change wrote %q", got)
	}

	// Disable keeps the raw command. Re-enable must not split it on spaces.
	quoted := filepath.Join(t.TempDir(), "crontab")
	original := "0 * * * * MONITORIO=shimk \"/opt/my app/run.sh\" --flag \"x y\"\n"
	if err := os.WriteFile(quoted, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	putDashJob(t, Job{CrontabFilename: quoted, Code: "shimk", Monitored: false, Command: "/opt/my app/run.sh", Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, quoted)); got != `0 * * * * "/opt/my app/run.sh" --flag "x y"` {
		t.Fatalf("disable broke a quoted path: %q", got)
	}
	ct, err = lib.GetCrontab(quoted)
	if err != nil {
		t.Fatal(err)
	}
	key = ""
	for _, line := range ct.Lines {
		if line.IsJob {
			key = line.Key(ct.CanonicalName())
		}
	}
	putDashJob(t, Job{CrontabFilename: quoted, Key: key, Monitored: true, Expression: "0 * * * *"})
	if got := strings.TrimSpace(readCrontab(t, quoted)); got != `0 * * * * cronitor exec NEWCODE "/opt/my app/run.sh" --flag "x y"` {
		t.Fatalf("re-enable broke a quoted path: %q", got)
	}
}

func putDashJob(t *testing.T, job Job) {
	t.Helper()
	body, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/jobs", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handlePutJob(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handlePutJob status %d: %s", rec.Code, rec.Body.String())
	}
}

func readCrontab(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
