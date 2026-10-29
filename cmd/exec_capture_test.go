package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

const captureTestKey = "telemetrykey0123456789abcdef0123"
const captureTestMonitor = "nightly-job"

type decodedEvent struct {
	EventID     string                 `json:"event_id"`
	Timestamp   string                 `json:"timestamp"`
	Platform    string                 `json:"platform"`
	Level       string                 `json:"level"`
	ServerName  string                 `json:"server_name"`
	Environment string                 `json:"environment"`
	Tags        map[string]string      `json:"tags"`
	Extra       map[string]interface{} `json:"extra"`
	SDK         map[string]string      `json:"sdk"`
	Exception   struct {
		Values []struct {
			Type       string `json:"type"`
			Value      string `json:"value"`
			Mechanism  map[string]interface{}
			Stacktrace *struct {
				Frames []lib.Frame `json:"frames"`
			} `json:"stacktrace"`
		} `json:"values"`
	} `json:"exception"`
}

type decodedLog struct {
	Timestamp  float64                           `json:"timestamp"`
	Level      string                            `json:"level"`
	Body       string                            `json:"body"`
	Attributes map[string]map[string]interface{} `json:"attributes"`
}

// withCaptureTest points pings, log uploads and envelopes at a mock server
// and restores every global the exec path reads.
func withCaptureTest(t *testing.T) *testutil.MockAPI {
	t.Helper()
	mock := testutil.NewMockAPI()
	mock.On("POST", "/api/envelope", 200, `{"id":"ok"}`)

	oldPingHost, oldBase, oldDev := lib.PingHostOverride, lib.BaseURLOverride, dev
	oldMonitor, oldNoStdout, oldVerbose := monitorCode, noStdoutPassthru, verbose
	oldCapture, oldShip := captureErrors, shipLogs
	saved := map[string]string{}
	for _, k := range []string{varApiKey, varPingApiKey, varEnv, varHostname, varLog, varPingApiHost} {
		saved[k] = viper.GetString(k)
	}
	t.Cleanup(func() {
		mock.Close()
		lib.PingHostOverride, lib.BaseURLOverride, dev = oldPingHost, oldBase, oldDev
		monitorCode, noStdoutPassthru, verbose = oldMonitor, oldNoStdout, oldVerbose
		captureErrors, shipLogs = oldCapture, oldShip
		for k, v := range saved {
			viper.Set(k, v)
		}
	})

	lib.PingHostOverride = mock.Server.URL
	lib.BaseURLOverride = mock.Server.URL + "/api"
	dev = false
	monitorCode = captureTestMonitor
	noStdoutPassthru = false
	verbose = false
	captureErrors, shipLogs = false, false
	viper.Set(varApiKey, "")
	viper.Set(varPingApiKey, captureTestKey)
	viper.Set(varEnv, "")
	viper.Set(varHostname, "worker-7")
	viper.Set(varLog, "")
	viper.Set(varPingApiHost, "")
	return mock
}

func skipWithoutPosixShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("exec capture tests drive a POSIX shell; Windows runs commands through PowerShell")
	}
}

func runQuietly(t *testing.T, command string) int {
	t.Helper()
	var code int
	testutil.CaptureStdoutStderr(func() { code = RunCommand(command, true, true) })
	return code
}

func envelopes(t *testing.T, mock *testutil.MockAPI) [][]testutil.EnvelopeItem {
	t.Helper()
	var out [][]testutil.EnvelopeItem
	for _, req := range mock.AllRequests() {
		if req.Path != "/api/envelope" {
			continue
		}
		if req.Method != "POST" || req.Headers.Get("Content-Encoding") != "gzip" ||
			req.Headers.Get("Content-Type") != "application/x-sentry-envelope" {
			t.Fatalf("envelope request %s with headers %v", req.Method, req.Headers)
		}
		if got := req.Headers.Get("X-Sentry-Auth"); got != "Sentry sentry_key="+captureTestKey+", sentry_version=7, sentry_client="+userAgent {
			t.Fatalf("X-Sentry-Auth %q", got)
		}
		if req.Headers.Get("User-Agent") != userAgent {
			t.Fatalf("User-Agent %q", req.Headers.Get("User-Agent"))
		}
		body, err := testutil.Gunzip(req.Body)
		if err != nil {
			t.Fatalf("envelope body is not gzip: %v", err)
		}
		header, items, err := testutil.DecodeEnvelope(body)
		if err != nil {
			t.Fatal(err)
		}
		if sdk, _ := header["sdk"].(map[string]interface{}); sdk["name"] != "cronitor-cli" || sdk["version"] != Version {
			t.Fatalf("envelope sdk %v", header["sdk"])
		}
		out = append(out, items)
	}
	return out
}

func onlyEvent(t *testing.T, mock *testutil.MockAPI) decodedEvent {
	t.Helper()
	envs := envelopes(t, mock)
	if len(envs) != 1 || len(envs[0]) != 1 || envs[0][0].Header["type"] != "event" {
		t.Fatalf("want exactly one envelope with one event item, got %d envelopes", len(envs))
	}
	var ev decodedEvent
	if err := json.Unmarshal(envs[0][0].Payload, &ev); err != nil {
		t.Fatal(err)
	}
	if len(ev.Exception.Values) != 1 {
		t.Fatalf("want one exception value, got %+v", ev.Exception)
	}
	return ev
}

func logRecords(t *testing.T, mock *testutil.MockAPI) ([]decodedLog, []int) {
	t.Helper()
	var records []decodedLog
	var batchSizes []int
	for _, items := range envelopes(t, mock) {
		for _, item := range items {
			if item.Header["type"] != "log" {
				t.Fatalf("unexpected item type %v", item.Header["type"])
			}
			if item.Header["content_type"] != "application/vnd.sentry.items.log+json" {
				t.Fatalf("log content_type %v", item.Header["content_type"])
			}
			var payload struct {
				Items []decodedLog `json:"items"`
			}
			if err := json.Unmarshal(item.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if int(item.Header["item_count"].(float64)) != len(payload.Items) {
				t.Fatalf("item_count %v != %d records", item.Header["item_count"], len(payload.Items))
			}
			batchSizes = append(batchSizes, len(payload.Items))
			records = append(records, payload.Items...)
		}
	}
	return records, batchSizes
}

func runPingSeries(t *testing.T, mock *testutil.MockAPI) string {
	t.Helper()
	for _, req := range mock.AllRequests() {
		if strings.HasPrefix(req.Path, "/ping/") && req.QueryParams.Get("state") == "run" {
			return req.QueryParams.Get("series")
		}
	}
	t.Fatal("no run ping recorded")
	return ""
}

func TestCaptureErrorsPythonTraceback(t *testing.T) {
	skipWithoutPosixShell(t)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed")
	}
	mock := withCaptureTest(t)
	captureErrors = true
	viper.Set(varEnv, "staging")

	script := filepath.Join(t.TempDir(), "job.py")
	src := "def main():\n    raise ValueError(\"boom\")\n\nprint(\"starting\", flush=True)\nmain()\n"
	if err := os.WriteFile(script, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}

	if code := runQuietly(t, python+" "+script); code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}

	ev := onlyEvent(t, mock)
	exc := ev.Exception.Values[0]
	if exc.Type != "ValueError" || exc.Value != "boom" || ev.Platform != "python" || ev.Level != "error" {
		t.Errorf("event %s: %s platform=%s level=%s", exc.Type, exc.Value, ev.Platform, ev.Level)
	}
	if exc.Mechanism["type"] != "cronitor.exec" || exc.Mechanism["handled"] != false {
		t.Errorf("mechanism %v", exc.Mechanism)
	}
	if exc.Stacktrace == nil {
		t.Fatal("want a stacktrace parsed from the traceback")
	}
	want := []lib.Frame{
		{Filename: script, Function: "<module>", Lineno: 5, ContextLine: "main()", InApp: true},
		{Filename: script, Function: "main", Lineno: 2, ContextLine: `raise ValueError("boom")`, InApp: true},
	}
	if len(exc.Stacktrace.Frames) != len(want) {
		t.Fatalf("frames %+v", exc.Stacktrace.Frames)
	}
	for i := range want {
		if exc.Stacktrace.Frames[i] != want[i] {
			t.Errorf("frame %d = %+v, want %+v", i, exc.Stacktrace.Frames[i], want[i])
		}
	}

	if ev.Tags["cronitor.monitor"] != captureTestMonitor || ev.Tags["exit_code"] != "1" ||
		ev.Tags["cronitor.series"] != runPingSeries(t, mock) || ev.Tags["host"] != "worker-7" ||
		!strings.Contains(ev.Tags["command"], "job.py") {
		t.Errorf("tags %v", ev.Tags)
	}
	if ev.ServerName != "worker-7" || ev.Environment != "staging" {
		t.Errorf("server_name %q environment %q", ev.ServerName, ev.Environment)
	}
	if ev.SDK["name"] != "cronitor-cli" || ev.SDK["version"] != Version {
		t.Errorf("sdk %v", ev.SDK)
	}
	tail, _ := ev.Extra["output_tail"].(string)
	if !strings.HasPrefix(tail, "starting\n") || !strings.Contains(tail, "ValueError: boom") {
		t.Errorf("output_tail %q", tail)
	}
	if _, err := time.Parse(time.RFC3339, ev.Timestamp); err != nil {
		t.Errorf("timestamp %q", ev.Timestamp)
	}
}

func TestCaptureErrorsShellFailureIsExitStatus(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors = true

	code := runQuietly(t, "echo 'step 1 ok'; echo 'rsync: connection refused' >&2; echo; exit 3")
	if code != 3 {
		t.Fatalf("exit code %d, want 3", code)
	}

	ev := onlyEvent(t, mock)
	exc := ev.Exception.Values[0]
	if exc.Type != "ExitStatus" || exc.Value != "rsync: connection refused" || ev.Platform != "other" {
		t.Errorf("event %s: %q platform=%s", exc.Type, exc.Value, ev.Platform)
	}
	if exc.Stacktrace != nil {
		t.Errorf("shell failure must have no stacktrace, got %+v", exc.Stacktrace)
	}
	if ev.Tags["exit_code"] != "3" {
		t.Errorf("exit_code tag %q", ev.Tags["exit_code"])
	}
	if ev.Environment != "" {
		t.Errorf("environment must be omitted when unset, got %q", ev.Environment)
	}
}

func TestCaptureErrorsSilentFailureUsesExitStatusText(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors = true

	if code := runQuietly(t, "exit 4"); code != 4 {
		t.Fatalf("exit code %d", code)
	}
	ev := onlyEvent(t, mock)
	if v := ev.Exception.Values[0]; v.Type != "ExitStatus" || v.Value != "exit status 4" {
		t.Errorf("event %s: %q", v.Type, v.Value)
	}
	if _, ok := ev.Extra["output_tail"]; ok {
		t.Errorf("no output means no output_tail, got %v", ev.Extra)
	}
}

func TestCaptureErrorsSuccessSendsNoEvent(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors = true

	if code := runQuietly(t, "echo fine"); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if n := len(envelopes(t, mock)); n != 0 {
		t.Errorf("a successful run must send no event, got %d envelopes", n)
	}
}

func TestCaptureErrorsNoStdoutSendsNoOutput(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors, shipLogs, noStdoutPassthru = true, true, true

	code := runQuietly(t, "printf 'Traceback (most recent call last):\\n  File \"/app/a.py\", line 1, in <module>\\nSecretError: hunter2\\n'; exit 2")
	if code != 2 {
		t.Fatalf("exit code %d", code)
	}
	ev := onlyEvent(t, mock) // also proves --ship-logs sent nothing
	if v := ev.Exception.Values[0]; v.Type != "ExitStatus" || v.Value != "exit status 2" {
		t.Errorf("with --no-stdout the event must not use output, got %s: %q", v.Type, v.Value)
	}
	if len(ev.Extra) != 0 {
		t.Errorf("with --no-stdout there must be no output_tail, got %v", ev.Extra)
	}
	if command, ok := ev.Tags["command"]; ok {
		t.Errorf("with --no-stdout there must be no command tag, got %q", command)
	}
}

// TestFailureEventCommandTag runs on every platform: it builds the event
// directly instead of running a command.
func TestFailureEventCommandTag(t *testing.T) {
	withCaptureTest(t)
	const apiKey = "fullapikey0123456789"
	viper.Set(varApiKey, apiKey)

	ev := buildFailureEvent("", "curl -fsS https://cronitor.link/p/"+captureTestKey+"/nightly-job?state=complete", "run-1", 1, nil, 1761706800)
	if got, want := ev.Tags["command"], "curl -fsS https://cronitor.link/p/[REDACTED]/nightly-job?state=complete"; got != want {
		t.Errorf("command tag %q, want %q", got, want)
	}

	// Redacting first means a key that crosses the cut at 200 characters
	// is not left half there.
	long := strings.Repeat("x", 190) + " " + apiKey
	ev = buildFailureEvent("", long, "run-1", 1, nil, 1761706800)
	if got, want := ev.Tags["command"], strings.Repeat("x", 190)+" [REDACTED"; got != want {
		t.Errorf("command tag %q, want %q", got, want)
	}

	noStdoutPassthru = true
	ev = buildFailureEvent("", "mysql -pSECRET -e 'select 1'", "run-1", 1, nil, 1761706800)
	if command, ok := ev.Tags["command"]; ok {
		t.Errorf("with --no-stdout there must be no command tag, got %q", command)
	}
	if ev.Tags["exit_code"] != "1" || ev.Tags["host"] != "worker-7" {
		t.Errorf("the other tags stay: %v", ev.Tags)
	}
}

func TestCaptureErrorsWithoutKeySendsNothing(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors, shipLogs = true, true
	viper.Set(varPingApiKey, "")
	viper.Set(varApiKey, "")

	if code := runQuietly(t, "echo out; exit 5"); code != 5 {
		t.Fatalf("missing key must not change the exit code, got %d", code)
	}
	for _, req := range mock.AllRequests() {
		if req.Path == "/api/envelope" {
			t.Fatal("no envelope may be sent without an API key")
		}
	}
}

func TestCaptureErrorsFallsBackToApiKey(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors = true
	viper.Set(varPingApiKey, "")
	viper.Set(varApiKey, "fullapikey")

	runQuietly(t, "exit 1")
	var auth string
	for _, req := range mock.AllRequests() {
		if req.Path == "/api/envelope" {
			auth = req.Headers.Get("X-Sentry-Auth")
		}
	}
	if !strings.HasPrefix(auth, "Sentry sentry_key=fullapikey,") {
		t.Errorf("want CRONITOR_API_KEY as the fallback key, got %q", auth)
	}
}

func TestCaptureFlagsOffSendNoEnvelope(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)

	if code := runQuietly(t, "echo 'ERROR: something broke'; exit 2"); code != 2 {
		t.Fatalf("exit code %d", code)
	}
	var sawFail bool
	for _, req := range mock.AllRequests() {
		if req.Path == "/api/envelope" {
			t.Fatal("flags off must not send envelopes")
		}
		if strings.HasPrefix(req.Path, "/ping/") && req.QueryParams.Get("state") == "fail" {
			sawFail = true
		}
	}
	if !sawFail {
		t.Error("the fail ping must still be sent")
	}
}

func TestShipLogsLevelsAndLineNumbers(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	shipLogs = true
	viper.Set(varEnv, "production")

	command := "echo 'starting job'; echo 'WARNING: disk at 91 percent'; echo; " +
		"echo 'ERROR: upload failed' >&2; printf 'done'"
	if code := runQuietly(t, command); code != 0 {
		t.Fatalf("exit code %d", code)
	}

	records, _ := logRecords(t, mock)
	want := []struct {
		line  int
		level string
		body  string
	}{
		{1, "info", "starting job"},
		{2, "warn", "WARNING: disk at 91 percent"},
		{4, "error", "ERROR: upload failed"},
		{5, "info", "done"},
	}
	if len(records) != len(want) {
		t.Fatalf("want %d records (blank line skipped), got %+v", len(want), records)
	}
	series := runPingSeries(t, mock)
	severity := map[string]float64{"info": 9, "warn": 13, "error": 17}
	for i, w := range want {
		r := records[i]
		if r.Body != w.body || r.Level != w.level {
			t.Errorf("record %d = %q (%s), want %q (%s)", i, r.Body, r.Level, w.body, w.level)
		}
		attr := func(k string) interface{} { return r.Attributes[k]["value"] }
		if attr("line") != float64(w.line) || r.Attributes["line"]["type"] != "integer" {
			t.Errorf("record %d line attribute %v", i, r.Attributes["line"])
		}
		if attr("cronitor.monitor") != captureTestMonitor || attr("cronitor.series") != series {
			t.Errorf("record %d binding %v %v", i, attr("cronitor.monitor"), attr("cronitor.series"))
		}
		if attr("sentry.severity_number") != severity[w.level] || attr("sentry.severity_text") != w.level {
			t.Errorf("record %d severity %v %v", i, attr("sentry.severity_number"), attr("sentry.severity_text"))
		}
		if attr("server.address") != "worker-7" || attr("sentry.environment") != "production" ||
			attr("sentry.sdk.name") != "cronitor-cli" || attr("sentry.sdk.version") != Version {
			t.Errorf("record %d attributes %v", i, r.Attributes)
		}
		if r.Timestamp < float64(time.Now().Add(-time.Minute).Unix()) {
			t.Errorf("record %d timestamp %v", i, r.Timestamp)
		}
	}
}

func TestShipLogsBatchesAtOneHundredLines(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	shipLogs = true

	if code := runQuietly(t, "for i in $(seq 1 250); do echo \"row $i\"; done"); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	records, sizes := logRecords(t, mock)
	if len(records) != 250 {
		t.Fatalf("want 250 records, got %d", len(records))
	}
	if len(sizes) < 3 {
		t.Errorf("250 lines must take at least 3 batches, got %v", sizes)
	}
	for _, n := range sizes {
		if n > 100 {
			t.Errorf("batch of %d exceeds 100 lines: %v", n, sizes)
		}
	}
	for i, r := range records {
		if r.Attributes["line"]["value"] != float64(i+1) {
			t.Fatalf("record %d has line %v", i, r.Attributes["line"]["value"])
		}
	}
}

func TestShipLogsAndCaptureErrorsTogether(t *testing.T) {
	skipWithoutPosixShell(t)
	mock := withCaptureTest(t)
	captureErrors, shipLogs = true, true

	if code := runQuietly(t, "echo working; echo 'fatal: repository not found'; exit 128"); code != 128 {
		t.Fatalf("exit code %d", code)
	}
	var events, logs int
	for _, items := range envelopes(t, mock) {
		for _, item := range items {
			switch item.Header["type"] {
			case "event":
				events++
			case "log":
				logs++
			}
		}
	}
	if events != 1 || logs < 1 {
		t.Errorf("want 1 event and at least 1 log item, got %d events and %d log items", events, logs)
	}
}

// TestLogShipperFlushesOnInterval runs on every platform: it feeds the line
// writer directly instead of running a command.
func TestLogShipperFlushesOnInterval(t *testing.T) {
	var mu sync.Mutex
	var batches [][]lib.LogRecord
	s := newLogShipper(func(batch []lib.LogRecord) error {
		mu.Lock()
		defer mu.Unlock()
		batches = append(batches, batch)
		return nil
	}, map[string]lib.LogAttribute{"cronitor.monitor": lib.StringAttribute("m")})

	s.Write([]byte("first line\nsecond "))
	s.Write([]byte("half\r\n"))

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(batches)
		mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	if len(batches) != 1 || len(batches[0]) != 2 {
		mu.Unlock()
		t.Fatalf("want one timed batch of 2 records before close, got %v", batches)
	}
	if batches[0][1].Body != "second half" {
		t.Errorf("partial writes must join into one line without CR, got %q", batches[0][1].Body)
	}
	mu.Unlock()

	s.Write([]byte("no newline at end"))
	s.Close()
	s.Write([]byte("after close\n")) // ignored, must not panic

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 2 || len(batches[1]) != 1 || batches[1][0].Body != "no newline at end" {
		t.Fatalf("close must flush the trailing partial line, got %v", batches)
	}
	if batches[1][0].Attributes["line"].Value != 3 {
		t.Errorf("line numbers continue across batches, got %v", batches[1][0].Attributes["line"])
	}
}

func TestExecArgsAcceptCaptureFlagsInAnyOrder(t *testing.T) {
	oldArgs, oldParts, oldMonitor := os.Args, commandParts, monitorCode
	oldCapture, oldShip, oldNoStdout := captureErrors, shipLogs, noStdoutPassthru
	t.Cleanup(func() {
		os.Args, commandParts, monitorCode = oldArgs, oldParts, oldMonitor
		captureErrors, shipLogs, noStdoutPassthru = oldCapture, oldShip, oldNoStdout
	})

	// main.go inserts "--" after the first token following "exec", so a
	// second flag lands after it where cobra no longer parses flags.
	os.Args = []string{"cronitor", "exec", "--capture-errors", "--", "--ship-logs", "--no-stdout", "nightly-job", "python3", "job.py"}
	commandParts, monitorCode = nil, ""
	captureErrors, shipLogs, noStdoutPassthru = false, false, false

	if err := execCmd.Args(execCmd, nil); err != nil {
		t.Fatal(err)
	}
	if monitorCode != "nightly-job" || strings.Join(commandParts, " ") != "python3 job.py" {
		t.Errorf("monitor %q command %q", monitorCode, commandParts)
	}
	if !captureErrors || !shipLogs || !noStdoutPassthru {
		t.Errorf("flags before the key must all apply: capture=%v ship=%v no-stdout=%v", captureErrors, shipLogs, noStdoutPassthru)
	}

	// Flags after the key belong to the wrapped command.
	os.Args = []string{"cronitor", "exec", "nightly-job", "--", "grep", "--ship-logs", "x"}
	commandParts, monitorCode = nil, ""
	captureErrors, shipLogs, noStdoutPassthru = false, false, false
	if err := execCmd.Args(execCmd, nil); err != nil {
		t.Fatal(err)
	}
	if shipLogs || strings.Join(commandParts, " ") != "grep --ship-logs x" {
		t.Errorf("a flag after the key must stay in the command: ship=%v command %q", shipLogs, commandParts)
	}
}

func TestLogShipperTimestampsStrictlyIncrease(t *testing.T) {
	// The read API pages with stamp < before, so lines written in one call must not share a timestamp.
	var mu sync.Mutex
	var got []lib.LogRecord
	s := newLogShipper(func(batch []lib.LogRecord) error {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, batch...)
		return nil
	}, nil)
	s.Write([]byte("one\ntwo\nthree\n"))
	s.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("want 3 records, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp <= got[i-1].Timestamp {
			t.Errorf("record %d timestamp %f is not after record %d timestamp %f", i, got[i].Timestamp, i-1, got[i-1].Timestamp)
		}
	}
}
