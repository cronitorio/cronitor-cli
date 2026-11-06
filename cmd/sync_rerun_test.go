//go:build !windows

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
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

func TestSyncWarnsWhenMonitorListFails(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	var puts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		puts++
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	stderr := runSync(t, srv.URL, path)

	if !strings.Contains(stderr, "Could not load your monitors") {
		t.Fatalf("stderr = %q; want a warning that the monitor list failed", stderr)
	}
	// Sync cannot compare, so it sends the schedule. It must not warn that
	// the monitor was deleted.
	if puts != 1 || strings.Contains(stderr, "no longer exists") {
		t.Fatalf("puts = %d, stderr = %q", puts, stderr)
	}
}

// answerPrompts replaces the job prompt. It records the name shown for each
// line and answers with answer(line number).
func answerPrompts(t *testing.T, answer func(lineNumber int, shown string) promptResult) *[]string {
	t.Helper()
	old := askAboutJob
	t.Cleanup(func() { askAboutJob = old })
	var asked []string
	askAboutJob = func(line *lib.Line, name string, monitored bool) promptResult {
		asked = append(asked, fmt.Sprintf("L%d %s", line.LineNumber, name))
		return answer(line.LineNumber, name)
	}
	return &asked
}

func keepName(_ int, shown string) promptResult {
	return promptResult{choice: choiceMonitor, name: shown}
}

func resetWithMonitored(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		withMonitored = false
		_ = discoverCmd.Flags().Set("with-monitored", "false")
		discoverCmd.Flags().Lookup("with-monitored").Changed = false
	})
}

func TestSyncAsksOnlyAboutNewJobs(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	_, apiURL := newRerunAPI(t, backupMonitor)
	asked := answerPrompts(t, keepName)
	answerConfirm(t, true)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n0 5 * * * /usr/bin/new-job.sh\n")

	runSyncArgs(t, apiURL, path)

	if len(*asked) != 1 || !strings.HasPrefix((*asked)[0], "L2 ") {
		t.Fatalf("asked about %v; want only the new job on line 2", *asked)
	}
}

func TestSyncWithMonitoredShowsMonitorNameAndRenames(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, `{"name":"Prune events","key":"prune-events","schedules":["0 1 * * *"],"attributes":{"key":"prune-events","code":"xyz789"}}`)
	asked := answerPrompts(t, func(_ int, _ string) promptResult {
		return promptResult{choice: choiceMonitor, name: "Prune old events"}
	})
	answerConfirm(t, true)
	path := writeCron(t, "0 1 * * * CRONITOR=prune-events /usr/bin/prune\n")

	runSyncArgs(t, apiURL, path, "--with-monitored")

	if len(*asked) != 1 || (*asked)[0] != "L1 Prune events" {
		t.Fatalf("asked %v; want the monitor's name from Cronitor", *asked)
	}
	puts := api.sent()
	if len(puts) != 1 {
		t.Fatalf("sync sent %v; want one rename", puts)
	}
	got := puts[0][0]
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"key", "name", "type"}) || got["name"] != "Prune old events" {
		t.Fatalf("payload = %v; want key, name and type only", got)
	}
}

func TestSyncWithMonitoredIgnoredByAuto(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	asked := answerPrompts(t, keepName)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	runSync(t, apiURL, path, "--with-monitored")

	if len(*asked) != 0 || len(api.sent()) != 0 {
		t.Fatalf("asked %v, sent %v; --auto must ignore --with-monitored", *asked, api.sent())
	}
}

// answerConfirm replaces the final "Apply?" prompt and records each question.
func answerConfirm(t *testing.T, yes bool) *[]string {
	t.Helper()
	old := confirm
	t.Cleanup(func() { confirm = old })
	var asked []string
	confirm = func(question string, defaultYes bool) bool {
		asked = append(asked, question)
		return yes
	}
	return &asked
}

func runSyncStdout(t *testing.T, serverURL, path string, extra ...string) string {
	t.Helper()
	runSyncArgs(t, serverURL, path, extra...)
	return lastSyncStdout
}

func TestSyncSummaryThenOnePrompt(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	answerPrompts(t, keepName)
	questions := answerConfirm(t, true)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n0 5 * * * /usr/bin/new-job.sh\n0 6 * * * /usr/bin/other.sh\n")

	out := runSyncStdout(t, apiURL, path)

	if len(*questions) != 1 {
		t.Fatalf("asked %v; want one final question", *questions)
	}
	if !strings.Contains(out, "2 new · 1 unchanged") {
		t.Fatalf("summary missing from output:\n%s", out)
	}
	if len(api.sent()) != 1 || len(api.sent()[0]) != 2 {
		t.Fatalf("sent %v; want both new monitors in one request", api.sent())
	}
}

func TestSyncAnswerNoChangesNothing(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	answerPrompts(t, keepName)
	answerConfirm(t, false)
	body := "0 5 * * * /usr/bin/new-job.sh\n"
	path := writeCron(t, body)

	runSyncStdout(t, apiURL, path)

	if len(api.sent()) != 0 {
		t.Fatalf("sent %v after answering no", api.sent())
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("crontab written after answering no:\n%s", got)
	}
}

func TestSyncNothingToChangeAsksNothing(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	asked := answerPrompts(t, keepName)
	questions := answerConfirm(t, true)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	out := runSyncStdout(t, apiURL, path)

	if len(*asked)+len(*questions) != 0 {
		t.Fatalf("asked %v and %v; want no questions", *asked, *questions)
	}
	// "Nothing to change" goes through the color package, which writes to
	// the real stdout. Check that no summary was printed instead.
	if len(api.sent()) != 0 || strings.Contains(out, "unchanged") {
		t.Fatalf("sent %v, output:\n%s", api.sent(), out)
	}
}

func TestSyncSkipForGoodWritesIgnoreMarker(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	asked := answerPrompts(t, func(int, string) promptResult { return promptResult{choice: choiceIgnore} })
	answerConfirm(t, true)
	path := writeCron(t, "0 5 * * * /usr/bin/noisy.sh\n")

	out := runSyncStdout(t, apiURL, path)

	if want := "# cronitor: ignore\n0 5 * * * /usr/bin/noisy.sh\n"; readCron(t, path) != want {
		t.Fatalf("crontab:\n%s\nwant:\n%s", readCron(t, path), want)
	}
	if len(api.sent()) != 0 || !strings.Contains(out, "1 ignored") {
		t.Fatalf("sent %v, output:\n%s", api.sent(), out)
	}

	runSyncArgs(t, apiURL, path)
	if len(*asked) != 1 {
		t.Fatalf("asked %v; the second sync must not ask about an ignored job", *asked)
	}
}

func TestSyncUpdatesNameCommentFromCronitor(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, backupMonitor)
	path := writeCron(t, "# Name: Old name\n0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	runSync(t, apiURL, path)

	if len(api.sent()) != 0 {
		t.Fatalf("sent %v; the name in Cronitor wins, nothing is sent", api.sent())
	}
	if want := "# Name: Nightly backup\n0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n"; readCron(t, path) != want {
		t.Fatalf("crontab:\n%s\nwant:\n%s", readCron(t, path), want)
	}
}

func TestSyncDoesNotAddNameComment(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	_, apiURL := newRerunAPI(t, backupMonitor)
	body := "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n"
	path := writeCron(t, body)

	runSync(t, apiURL, path)

	if got := readCron(t, path); got != body {
		t.Fatalf("crontab changed:\n%s", got)
	}
}

func TestSyncDryRunSendsNothing(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetDryRun(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	body := "30 3 * * * cronitor exec abc123 /usr/bin/backup.sh\n0 5 * * * /usr/bin/new-job.sh\n"
	path := writeCron(t, body)

	runSync(t, apiURL, path, "--dry-run")

	if len(api.sent()) != 0 {
		t.Fatalf("dry run sent %v", api.sent())
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("dry run wrote the crontab:\n%s", got)
	}
}

func TestSyncDryRunInteractiveAsksNoFinalQuestion(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	resetDryRun(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	answerPrompts(t, keepName)
	questions := answerConfirm(t, true)
	path := writeCron(t, "0 5 * * * /usr/bin/new-job.sh\n")

	out := runSyncStdout(t, apiURL, path, "--dry-run")

	if len(*questions) != 0 || len(api.sent()) != 0 {
		t.Fatalf("asked %v, sent %v; a dry run only shows the summary", *questions, api.sent())
	}
	if !strings.Contains(out, "1 new") {
		t.Fatalf("summary missing:\n%s", out)
	}
}

func resetDryRun(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		dryRun = false
		_ = discoverCmd.Flags().Set("dry-run", "false")
		discoverCmd.Flags().Lookup("dry-run").Changed = false
	})
}

func quitOnLine(n int) func(int, string) promptResult {
	return func(line int, shown string) promptResult {
		if line == n {
			return promptResult{choice: choiceQuit}
		}
		return promptResult{choice: choiceMonitor, name: shown}
	}
}

func TestSyncQuitSavesAnswersSoFar(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	answerPrompts(t, quitOnLine(2))
	questions := answerConfirm(t, true)
	path := writeCron(t, "0 5 * * * /usr/bin/first.sh\n0 6 * * * /usr/bin/second.sh\n")

	runSyncStdout(t, apiURL, path)

	if len(*questions) != 1 || !strings.Contains((*questions)[0], "Save what you've done so far") {
		t.Fatalf("asked %v; want only the save question", *questions)
	}
	if puts := api.sent(); len(puts) != 1 || len(puts[0]) != 1 {
		t.Fatalf("sent %v; want only the first job", puts)
	}
	got := readCron(t, path)
	if !strings.Contains(got, "CRONITOR=new") || !strings.Contains(got, "0 6 * * * /usr/bin/second.sh") {
		t.Fatalf("crontab:\n%s", got)
	}
}

func TestSyncQuitWithoutSavingChangesNothing(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	answerPrompts(t, quitOnLine(2))
	answerConfirm(t, false)
	body := "0 5 * * * /usr/bin/first.sh\n0 6 * * * /usr/bin/second.sh\n"
	path := writeCron(t, body)

	runSyncStdout(t, apiURL, path)

	if len(api.sent()) != 0 || readCron(t, path) != body {
		t.Fatalf("sent %v, crontab:\n%s", api.sent(), readCron(t, path))
	}
}

func TestSyncOneSudoHintForUnwritableCrontabs(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	_, apiURL := newRerunAPI(t, backupMonitor)
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	os.WriteFile(locked, []byte("0 1 * * * root /usr/bin/a.sh\n0 2 * * * root /usr/bin/b.sh\n"), 0644)
	os.WriteFile(filepath.Join(dir, "open"), []byte("0 3 * * * root /usr/bin/c.sh\n"), 0644)
	old := crontabWritable
	t.Cleanup(func() { crontabWritable = old })
	crontabWritable = func(c *lib.Crontab) bool { return c.Filename != locked }

	stderr := runSync(t, apiURL, dir)

	if strings.Count(stderr, "sudo") != 1 || !strings.Contains(stderr, "2 jobs in system crontabs were not checked") {
		t.Fatalf("stderr = %q; want one hint that counts the 2 jobs", stderr)
	}
}

// A rerun without --auto is how most people pick up a schedule change. It
// must send the change without asking about the monitored job.
func TestSyncInteractiveRerunSendsScheduleChange(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	asked := answerPrompts(t, keepName)
	questions := answerConfirm(t, true)
	path := writeCron(t, "30 3 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	out := runSyncStdout(t, apiURL, path)

	if len(*asked) != 0 || len(*questions) != 1 {
		t.Fatalf("asked %v, questions %v; want no job prompt and one Apply question", *asked, *questions)
	}
	puts := api.sent()
	if len(puts) != 1 || !reflect.DeepEqual(puts[0][0]["schedules"], []interface{}{"30 3 * * *"}) {
		t.Fatalf("sent %v; want the new schedule", puts)
	}
	if !strings.Contains(out, "1 updated") || !strings.Contains(out, "L1  Nightly backup: schedule 0 2 * * * → 30 3 * * *") {
		t.Fatalf("summary does not show the change:\n%s", out)
	}
}

func TestSyncRerunTimezoneChangeSendsTimezone(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, `{"name":"Nightly backup","key":"backup","schedules":["0 2 * * *"],"timezone":"UTC","attributes":{"key":"backup","code":"abc123"}}`)
	path := writeCron(t, "CRON_TZ=America/New_York\n0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	runSync(t, apiURL, path)

	puts := api.sent()
	if len(puts) != 1 || len(puts[0]) != 1 {
		t.Fatalf("sync sent %v; want one timezone update", puts)
	}
	got := puts[0][0]
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"key", "timezone", "type"}) || got["timezone"] != "America/New_York" {
		t.Fatalf("payload = %v; want key, timezone and type only", got)
	}
}

func TestSyncRerunSameTimezoneSendsNothing(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	api, apiURL := newRerunAPI(t, `{"name":"Nightly backup","key":"backup","schedules":["0 2 * * *"],"timezone":"America/New_York","attributes":{"key":"backup","code":"abc123"}}`)
	path := writeCron(t, "CRON_TZ=America/New_York\n0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	runSync(t, apiURL, path)

	if puts := api.sent(); len(puts) != 0 {
		t.Fatalf("sync sent %v; the timezone did not change", puts)
	}
}

func TestPlanUpdateDescribesEachChange(t *testing.T) {
	old := timezone
	t.Cleanup(func() { timezone = old })
	timezone = lib.TimezoneLocationName{Name: "America/New_York"}
	withNotificationList(t, "oncall")
	existing := lib.Monitor{Name: "Old name", Schedules: &[]string{"0 2 * * *"}, Timezone: "UTC"}
	line := &lib.Line{Code: "abc123", CronExpression: "0 2 * * *"}

	p := planUpdate(line, "hash", existing, true, "Old name", "New name")

	want := []string{"timezone UTC → America/New_York", `name "Old name" → "New name"`, "alert list → oncall"}
	if !reflect.DeepEqual(p.changes, want) {
		t.Fatalf("changes = %q; want %q", p.changes, want)
	}
}
