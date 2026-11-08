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
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, `{"name":"Nightly backup","key":"backup","schedules":["0 2 * * *"],"timezone":"America/New_York","attributes":{"key":"backup","code":"abc123"}}`)
	asked := answerPrompts(t, keepName)
	questions := answerConfirm(t, true)
	body := "CRON_TZ=America/New_York\n0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n"
	path := writeCron(t, body)

	runSyncArgs(t, apiURL, path)

	if len(*asked)+len(*questions) != 0 {
		t.Fatalf("asked %v and %v; want no questions", *asked, *questions)
	}
	if puts := api.sent(); len(puts) != 0 {
		t.Fatalf("sync sent %v; want no request for an unchanged job", puts)
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("crontab changed:\n%s", got)
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
	var bodies [][]map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		var body []map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	path := writeCron(t, "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	stderr := runSync(t, srv.URL, path)

	if !strings.Contains(stderr, "Could not load your monitors") {
		t.Fatalf("stderr = %q; want a warning that the monitor list failed", stderr)
	}
	// Sync cannot compare, so it sends the schedule. It must not warn that
	// the monitor was deleted, rename it to the generated name, or add a
	// "# Name:" comment.
	if len(bodies) != 1 || strings.Contains(stderr, "no longer exists") {
		t.Fatalf("bodies = %v, stderr = %q", bodies, stderr)
	}
	if _, ok := bodies[0][0]["name"]; ok {
		t.Fatalf("payload %v renames the monitor to the generated name", bodies[0][0])
	}
	if got := readCron(t, path); strings.Contains(got, "# Name:") {
		t.Fatalf("crontab gained a name comment:\n%s", got)
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

func TestSyncDryRunSendsNothing(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	resetDryRun(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	body := "30 3 * * * cronitor exec abc123 /usr/bin/backup.sh\n0 5 * * * /usr/bin/new-job.sh\n"
	path := writeCron(t, body)

	t.Run("auto", func(t *testing.T) {
		stderr := runSyncArgs(t, apiURL, path, "--auto", "--dry-run")
		if len(api.sent()) != 0 {
			t.Fatalf("dry run sent %v", api.sent())
		}
		if got := readCron(t, path); got != body {
			t.Fatalf("dry run wrote the crontab:\n%s", got)
		}
		if !strings.Contains(lastSyncStdout, "30 3 * * * cronitor exec abc123") {
			t.Fatalf("stdout = %q; want the crontab", lastSyncStdout)
		}
		if !strings.Contains(stderr, "1 new") || !strings.Contains(stderr, "1 updated") {
			t.Fatalf("stderr missing summary:\n%s", stderr)
		}
	})

	t.Run("interactive", func(t *testing.T) {
		answerPrompts(t, keepName)
		questions := answerConfirm(t, true)
		out := runSyncStdout(t, apiURL, path, "--dry-run")
		if len(*questions) != 0 || len(api.sent()) != 0 {
			t.Fatalf("asked %v, sent %v; a dry run only shows the summary", *questions, api.sent())
		}
		if got := readCron(t, path); got != body {
			t.Fatalf("dry run wrote the crontab:\n%s", got)
		}
		if !strings.Contains(out, "1 new") {
			t.Fatalf("summary missing:\n%s", out)
		}
	})
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
	body := "0 5 * * * /usr/bin/first.sh\n0 6 * * * /usr/bin/second.sh\n"
	for _, tc := range []struct {
		name string
		save bool
	}{
		{"save", true},
		{"discard", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withSyncFixture(t)
			withNotificationList(t, "")
			resetWithMonitored(t)
			api, apiURL := newRerunAPI(t, backupMonitor)
			answerPrompts(t, quitOnLine(2))
			questions := answerConfirm(t, tc.save)
			path := writeCron(t, body)

			runSyncStdout(t, apiURL, path)

			if len(*questions) != 1 || !strings.Contains((*questions)[0], "Save what you've done so far") {
				t.Fatalf("asked %v; want only the save question", *questions)
			}
			if tc.save {
				if puts := api.sent(); len(puts) != 1 || len(puts[0]) != 1 {
					t.Fatalf("sent %v; want only the first job", puts)
				}
				got := readCron(t, path)
				if !strings.Contains(got, "CRONITOR=new") || !strings.Contains(got, "0 6 * * * /usr/bin/second.sh") {
					t.Fatalf("crontab:\n%s", got)
				}
			} else if len(api.sent()) != 0 || readCron(t, path) != body {
				t.Fatalf("sent %v, crontab:\n%s", api.sent(), readCron(t, path))
			}
		})
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
	path := writeCron(t, "TZ=UTC\n30 3 * * * cronitor exec abc123 /usr/bin/backup.sh\n")

	out := runSyncStdout(t, apiURL, path)

	if len(*asked) != 0 || len(*questions) != 1 {
		t.Fatalf("asked %v, questions %v; want no job prompt and one Apply question", *asked, *questions)
	}
	puts := api.sent()
	if len(puts) != 1 || len(puts[0]) != 1 {
		t.Fatalf("sent %v; want the new schedule", puts)
	}
	got := puts[0][0]
	if names := fieldNames(got); !reflect.DeepEqual(names, []string{"key", "schedules", "timezone", "type"}) {
		t.Fatalf("payload fields = %v (%v); want key, schedules, timezone and type only", names, got)
	}
	if got["key"] != "abc123" || !reflect.DeepEqual(got["schedules"], []interface{}{"30 3 * * *"}) || got["timezone"] != "UTC" {
		t.Fatalf("payload = %v", got)
	}
	if !strings.Contains(out, "1 updated") || !strings.Contains(out, "L2  Nightly backup: schedule 0 2 * * * → 30 3 * * *") {
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

// Pipelines such as "cronitor sync --auto file | crontab -" read stdout, so
// --auto prints the crontab even when nothing changed, including --dry-run.
func TestSyncAutoPrintsCrontabWhenNothingChanged(t *testing.T) {
	for _, extra := range [][]string{{"--auto"}, {"--auto", "--dry-run"}} {
		t.Run(strings.Join(extra, " "), func(t *testing.T) {
			withSyncFixture(t)
			withNotificationList(t, "")
			resetDryRun(t)
			_, apiURL := newRerunAPI(t, backupMonitor)
			body := "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n"
			path := writeCron(t, body)

			runSyncArgs(t, apiURL, path, extra...)

			if strings.TrimSpace(lastSyncStdout) != strings.TrimSpace(body) {
				t.Fatalf("stdout = %q; want the crontab", lastSyncStdout)
			}
		})
	}
}

func TestSyncAutoPrintsOriginalWhenShimInstallFails(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	_, apiURL := newRerunAPI(t, backupMonitor)
	body := "0 2 * * * cronitor exec abc123 /usr/bin/backup.sh\n"
	path := writeCron(t, body)
	blocked := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocked, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	lib.ShimShellPathOverride = filepath.Join(blocked, "cronitor", "shell")

	var code int
	oldExit := exitFn
	t.Cleanup(func() { exitFn = oldExit })
	exitFn = func(c int) { code = c }

	stderr := runSyncArgs(t, apiURL, path, "--auto", "--format=short")

	if code != 1 {
		t.Fatalf("exit %d; want 1 after the wrapper could not be installed", code)
	}
	if strings.Contains(lastSyncStdout, "SHELL=") || strings.Contains(lastSyncStdout, "CRONITOR=") {
		t.Fatalf("stdout is the planned crontab:\n%s", lastSyncStdout)
	}
	if strings.TrimSpace(lastSyncStdout) != strings.TrimSpace(body) {
		t.Fatalf("stdout = %q; want the original crontab", lastSyncStdout)
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("crontab on disk changed:\n%s", got)
	}
	if !strings.Contains(stderr, "cannot install") {
		t.Fatalf("stderr = %q; want the install error", stderr)
	}
}

// An API failure used to os.Exit inside applyPlan, skipping the deferred
// print. `cronitor sync --auto file | crontab -` then installed an empty crontab.
func TestSyncAutoPrintsOriginalWhenAPIFails(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			io.WriteString(w, `{"total_monitor_count":0,"page_size":50,"monitors":[]}`)
			return
		}
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	body := "0 2 * * * /usr/bin/backup.sh\n"
	path := writeCron(t, body)

	var code int
	oldExit := exitFn
	t.Cleanup(func() {
		exitFn = oldExit
		verbose = false
		if f := RootCmd.PersistentFlags().Lookup("verbose"); f != nil {
			_ = f.Value.Set("false")
			f.Changed = false
		}
	})
	exitFn = func(c int) { code = c }

	// A previous test may have viper.Set the config path, which hides --config.
	viperUnset(varConfig)
	// --verbose must not prefix the crontab. Those logs used to be written
	// before os.Exit, so the pipe installed a partial crontab.
	stderr := runSyncArgs(t, srv.URL, path, "--auto", "--verbose")

	if code != 1 {
		t.Fatalf("exit %d; want 1 after the API call fails", code)
	}
	if strings.TrimSpace(lastSyncStdout) == "" {
		t.Fatal("stdout is empty; the pipe would install an empty crontab")
	}
	if strings.TrimSpace(lastSyncStdout) != strings.TrimSpace(body) {
		t.Fatalf("stdout = %q; want the original crontab", lastSyncStdout)
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("crontab on disk changed:\n%s", got)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Fatal("stderr is empty; the API error was not reported")
	}
}

// A missing or unwritable file used to print nothing and exit 0, so
// `cronitor sync --auto file | crontab -` installed an empty crontab.
func TestSyncAutoExitsWhenCrontabMissingOrUnwritable(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	_, apiURL := newRerunAPI(t, backupMonitor)

	var code int
	oldExit := exitFn
	t.Cleanup(func() { exitFn = oldExit })
	exitFn = func(c int) { code = c }

	t.Run("missing", func(t *testing.T) {
		code = 0
		missing := filepath.Join(t.TempDir(), "no-such-crontab")
		stderr := runSyncArgs(t, apiURL, missing, "--auto")
		if code != 1 {
			t.Fatalf("exit %d; want 1 when the crontab does not exist", code)
		}
		if lastSyncStdout != "" {
			t.Fatalf("stdout = %q; want nothing when the file does not exist", lastSyncStdout)
		}
		if strings.TrimSpace(stderr) == "" {
			t.Fatal("stderr is empty; the missing file was not reported")
		}
	})

	t.Run("unwritable", func(t *testing.T) {
		code = 0
		body := "0 2 * * * /usr/bin/backup.sh\n"
		path := writeCron(t, body)
		// Root can still open a mode 0444 file, so stub the check planCrontab uses.
		old := crontabWritable
		t.Cleanup(func() { crontabWritable = old })
		crontabWritable = func(*lib.Crontab) bool { return false }

		stderr := runSyncArgs(t, apiURL, path, "--auto")
		if code != 1 {
			t.Fatalf("exit %d; want 1 when the crontab is not writable", code)
		}
		if strings.TrimSpace(lastSyncStdout) != strings.TrimSpace(body) {
			t.Fatalf("stdout = %q; want the original crontab", lastSyncStdout)
		}
		if got := readCron(t, path); got != body {
			t.Fatalf("crontab on disk changed:\n%s", got)
		}
		if strings.TrimSpace(stderr) == "" {
			t.Fatal("stderr is empty; the unwritable file was not reported")
		}
	})
}

func TestSyncSilentStillConfirms(t *testing.T) {
	withSyncFixture(t)
	withNotificationList(t, "")
	resetWithMonitored(t)
	api, apiURL := newRerunAPI(t, backupMonitor)
	asked := answerPrompts(t, keepName)
	questions := answerConfirm(t, false)
	body := "30 3 * * * cronitor exec abc123 /usr/bin/backup.sh\n"
	path := writeCron(t, body)

	runSyncArgs(t, apiURL, path, "--silent")

	if len(*asked) != 0 {
		t.Fatalf("asked about jobs %v; --silent is not --auto", *asked)
	}
	if len(*questions) != 1 {
		t.Fatalf("asked %v; --silent without --auto must still confirm before applying", *questions)
	}
	if len(api.sent()) != 0 || readCron(t, path) != body {
		t.Fatalf("applied after answering no: sent %v\n%s", api.sent(), readCron(t, path))
	}
}
