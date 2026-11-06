//go:build !windows

package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// rerunAPI is a fake Cronitor API. GET returns the given monitors. Each PUT
// body is recorded, and new monitors get the code "new" + the first 3
// characters of their key.
type rerunAPI struct {
	mu   sync.Mutex
	puts [][]map[string]interface{}
}

func newRerunAPI(t *testing.T, monitors string) (*rerunAPI, string) {
	t.Helper()
	api := &rerunAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"total_monitor_count":1,"page_size":50,"monitors":[`+monitors+`]}`)
			return
		}
		var body []map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		api.mu.Lock()
		api.puts = append(api.puts, body)
		api.mu.Unlock()
		out := []map[string]interface{}{}
		for _, m := range body {
			key, _ := m["key"].(string)
			out = append(out, map[string]interface{}{"attributes": map[string]string{"key": key, "code": "new" + key[:3]}})
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return api, srv.URL
}

func (a *rerunAPI) sent() [][]map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.puts
}

// fieldNames lists the fields of a PUT payload. attributes is left out: the
// client always sends it and the API ignores it (read-only).
func fieldNames(m map[string]interface{}) []string {
	var names []string
	for k := range m {
		if k == "attributes" {
			continue
		}
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func withNotificationList(t *testing.T, list string) {
	t.Helper()
	old := notificationList
	t.Cleanup(func() {
		notificationList = old
		_ = discoverCmd.Flags().Set("notification-list", "")
		discoverCmd.Flags().Lookup("notification-list").Changed = false
	})
	notificationList = list
}

const backupMonitor = `{"name":"Nightly backup","key":"backup","schedules":["0 2 * * *"],"tags":["db","team-a"],"notify":["ops"],"attributes":{"key":"backup","code":"abc123"}}`

func TestSyncRerunUnchangedSendsNothing(t *testing.T) {
	serverURL, _ := withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, backupMonitor)
	_ = serverURL
	body := "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n"
	path := writeCron(t, body)

	runSync(t, apiURL, path)

	if puts := api.sent(); len(puts) != 0 {
		t.Fatalf("sync sent %v; want no request for an unchanged job", puts)
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("crontab changed:\n%s", got)
	}
}

func TestSyncRerunScheduleChangeSendsOnlySchedule(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, backupMonitor)
	path := writeCron(t, "TZ=UTC\n30 3 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	runSync(t, apiURL, path)

	puts := api.sent()
	if len(puts) != 1 || len(puts[0]) != 1 {
		t.Fatalf("sync sent %v; want one monitor", puts)
	}
	got := puts[0][0]
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"key", "schedules", "timezone", "type"}) {
		t.Fatalf("payload fields = %v (%v); want key, schedules, timezone and type only", names, got)
	}
	if got["key"] != "abc123" || !reflect.DeepEqual(got["schedules"], []interface{}{"30 3 * * *"}) || got["timezone"] != "UTC" {
		t.Fatalf("payload = %v", got)
	}
}

func TestSyncRerunNotificationListSentOnlyWithFlag(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, backupMonitor)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	runSync(t, apiURL, path, "--notification-list", "oncall")

	puts := api.sent()
	if len(puts) != 1 || len(puts[0]) != 1 {
		t.Fatalf("sync sent %v; want one monitor", puts)
	}
	got := puts[0][0]
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"key", "notify", "type"}) {
		t.Fatalf("payload fields = %v (%v); want key, notify and type only", names, got)
	}
	if !reflect.DeepEqual(got["notify"], []interface{}{"oncall"}) {
		t.Fatalf("notify = %v", got["notify"])
	}
}

func TestSyncRerunDeletedMonitorWarnsAndKeepsLine(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, backupMonitor)
	body := "0 4 * * * cronitor exec gone99 /usr/bin/report.sh\n"
	path := writeCron(t, body)

	stderr := runSync(t, apiURL, path)

	if puts := api.sent(); len(puts) != 0 {
		t.Fatalf("sync sent %v; it must not create the deleted monitor again", puts)
	}
	if !strings.Contains(stderr, "gone99") || !strings.Contains(stderr, "no longer exists") {
		t.Fatalf("stderr = %q; want a warning about gone99", stderr)
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("crontab changed:\n%s", got)
	}
}

func TestSyncNewJobDoesNotSendNotify(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, backupMonitor)
	path := writeCron(t, "0 5 * * * /usr/bin/new-job.sh\n")

	runSync(t, apiURL, path)

	puts := api.sent()
	if len(puts) != 1 || len(puts[0]) != 1 {
		t.Fatalf("sync sent %v; want one new monitor", puts)
	}
	got := puts[0][0]
	if _, ok := got["notify"]; ok {
		t.Fatalf("new monitor payload has notify %v; Cronitor applies the default list when it is left out", got["notify"])
	}
	if !reflect.DeepEqual(got["tags"], []interface{}{"cron-job"}) || got["type"] != "job" {
		t.Fatalf("new monitor payload = %v", got)
	}
	if !strings.Contains(readCron(t, path), "CRONITOR=new") {
		t.Fatalf("new job not wrapped:\n%s", readCron(t, path))
	}
}

func TestSyncRerunKeepsHandWrittenKey(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, `{"name":"Prune events","key":"prune-events","schedules":["0 1 * * *"],"attributes":{"key":"prune-events","code":"xyz789"}}`)
	path := writeCron(t, "0 6 * * * CRONITOR=prune-events /usr/bin/prune\n")

	runSync(t, apiURL, path)

	puts := api.sent()
	if len(puts) != 1 || puts[0][0]["key"] != "prune-events" {
		t.Fatalf("sync sent %v; want a schedule update for prune-events", puts)
	}
	if got := readCron(t, path); !strings.Contains(got, "0 6 * * * CRONITOR=prune-events /usr/bin/prune") {
		t.Fatalf("hand-written key was replaced:\n%s", got)
	}
}
