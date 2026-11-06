package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// TestDiscoverSyncKeepsWrappedPrefix drives processCrontab twice. Discover's
// update leaves Line.Code empty and the code on Mon; Write has to use GetCode
// or the first sync drops the custom binary path and flags.
func TestDiscoverSyncKeepsWrappedPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Echo the monitor key the client sent. That key is the line code, not
		// the hash PutMonitors indexes by, so Attributes.Code is not applied
		// and Line.Code becomes empty while Mon.Code keeps the code.
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
			resp[i].Attributes.Code = "SHOULD_NOT_APPLY"
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cleanup := setupIntegrationTest(srv.URL + "/api")
	defer cleanup()

	oldAuto, oldSilent, oldDry, oldNoStdout := isAutoDiscover, isSilent, dryRun, noStdoutPassthru
	oldExisting := existingMonitors
	oldNotes := notificationList
	t.Cleanup(func() {
		isAutoDiscover, isSilent, dryRun, noStdoutPassthru = oldAuto, oldSilent, oldDry, oldNoStdout
		existingMonitors = oldExisting
		notificationList = oldNotes
	})
	isAutoDiscover, isSilent, dryRun, noStdoutPassthru = true, true, false, true
	existingMonitors = ExistingMonitors{}
	notificationList = ""
	withCronitorEnv(t, "prod")

	path := filepath.Join(t.TempDir(), "crontab")
	original := "# Name: Backup\n0 * * * * /opt/cronitor --env staging -c alt.json exec k1 /bin/true\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	syncOnce := func() {
		t.Helper()
		if !processCrontab(lib.CrontabFactory("wrap-sync", path)) {
			t.Fatal("sync did not update the crontab")
		}
	}
	syncOnce()
	first := readCrontab(t, path)
	syncOnce()
	second := readCrontab(t, path)
	if first != second {
		t.Fatalf("sync output changed\n first: %s\nsecond: %s", first, second)
	}
	want := "# Name: Backup\n0 * * * * /opt/cronitor --env staging -c alt.json exec --no-stdout k1 /bin/true\n"
	if second != want {
		t.Fatalf("sync wrote %q", second)
	}
}

func withCronitorEnv(t *testing.T, env string) {
	t.Helper()
	prev := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", env)
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", prev) })
}
