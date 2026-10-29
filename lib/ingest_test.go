package lib_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
)

func parseEnvelope(t *testing.T, data []byte) (map[string]interface{}, []testutil.EnvelopeItem) {
	t.Helper()
	header, items, err := testutil.DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	for _, item := range items {
		if length, ok := item.Header["length"].(float64); !ok || int(length) != len(item.Payload) {
			t.Fatalf("item header must carry the payload length: %v", item.Header)
		}
	}
	return header, items
}

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestIngestNewEventID(t *testing.T) {
	a, b := lib.NewEventID(), lib.NewEventID()
	if !hex32.MatchString(a) || !hex32.MatchString(b) {
		t.Fatalf("event ids must be 32 lowercase hex: %q %q", a, b)
	}
	if a == b {
		t.Fatal("event ids must be unique")
	}
}

func TestIngestEventEnvelopeMarshal(t *testing.T) {
	frames := []lib.Frame{
		{Filename: "/app/job.py", Function: "<module>", Lineno: 12, ContextLine: "main()", InApp: true},
		{Filename: "/app/job.py", Function: "main", Lineno: 8, ContextLine: `raise ValueError("boom")`, InApp: true},
	}
	ev := lib.NewExceptionEvent("ValueError", "boom", "python", frames, "33.11")
	ev.ServerName = "web-1"
	ev.Environment = "production"
	ev.Tags = map[string]string{"cronitor.monitor": "nightly", "exit_code": "1"}
	ev.Extra = map[string]interface{}{"output_tail": "line\n"}

	env, err := lib.NewEventEnvelope(ev)
	if err != nil {
		t.Fatal(err)
	}
	header, items := parseEnvelope(t, env.Marshal())

	if header["event_id"] != ev.EventID || !hex32.MatchString(ev.EventID) {
		t.Errorf("header event_id %v, event %q", header["event_id"], ev.EventID)
	}
	if _, err := time.Parse(time.RFC3339, header["sent_at"].(string)); err != nil {
		t.Errorf("sent_at not RFC3339: %v", header["sent_at"])
	}
	sdk := header["sdk"].(map[string]interface{})
	if sdk["name"] != "cronitor-cli" || sdk["version"] != "33.11" {
		t.Errorf("header sdk %v", sdk)
	}

	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	ih := items[0].Header
	if ih["type"] != "event" || ih["content_type"] != "application/json" {
		t.Errorf("item header %v", ih)
	}
	if int(ih["length"].(float64)) != len(items[0].Payload) {
		t.Errorf("length %v != payload %d", ih["length"], len(items[0].Payload))
	}

	var got map[string]interface{}
	if err := json.Unmarshal(items[0].Payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["event_id"] != ev.EventID || got["level"] != "error" || got["platform"] != "python" {
		t.Errorf("event basics: %v", got)
	}
	if got["server_name"] != "web-1" || got["environment"] != "production" {
		t.Errorf("server_name/environment: %v %v", got["server_name"], got["environment"])
	}
	if ts, err := time.Parse(time.RFC3339, got["timestamp"].(string)); err != nil || ts.Location() != time.UTC {
		t.Errorf("timestamp %v: %v", got["timestamp"], err)
	}
	values := got["exception"].(map[string]interface{})["values"].([]interface{})
	exc := values[0].(map[string]interface{})
	if exc["type"] != "ValueError" || exc["value"] != "boom" {
		t.Errorf("exception %v", exc)
	}
	mech := exc["mechanism"].(map[string]interface{})
	if mech["type"] != "cronitor.exec" || mech["handled"] != false {
		t.Errorf("mechanism %v", mech)
	}
	gotFrames := exc["stacktrace"].(map[string]interface{})["frames"].([]interface{})
	if len(gotFrames) != 2 {
		t.Fatalf("frames %v", gotFrames)
	}
	last := gotFrames[1].(map[string]interface{})
	if last["function"] != "main" || last["lineno"] != float64(8) || last["in_app"] != true ||
		last["filename"] != "/app/job.py" || last["context_line"] != `raise ValueError("boom")` {
		t.Errorf("crash frame %v", last)
	}
	if got["sdk"].(map[string]interface{})["name"] != "cronitor-cli" {
		t.Errorf("event sdk %v", got["sdk"])
	}
}

func TestIngestEventWithoutFramesOmitsStacktrace(t *testing.T) {
	ev := lib.NewExceptionEvent("ExitStatus", "exit status 3", "other", nil, "33.11")
	env, err := lib.NewEventEnvelope(ev)
	if err != nil {
		t.Fatal(err)
	}
	_, items := parseEnvelope(t, env.Marshal())
	if bytes.Contains(items[0].Payload, []byte("stacktrace")) {
		t.Errorf("stacktrace must be omitted without frames: %s", items[0].Payload)
	}
	if bytes.Contains(items[0].Payload, []byte(`"environment"`)) {
		t.Errorf("empty environment must be omitted: %s", items[0].Payload)
	}
}

func TestIngestLogLevelForLine(t *testing.T) {
	cases := map[string]string{
		"starting job":                                  "info",
		"ERROR: could not connect":                      "error",
		"Traceback (most recent call last):":            "error",
		"panic: runtime error":                          "error",
		"Fatal: disk full":                              "error",
		"raised an Exception here":                      "error",
		"WARNING: low disk":                             "warn",
		"warn: deprecated flag":                         "warn",
		"errors=0 warnings=0":                           "info", // word boundaries: "errors" and "warnings" are not matches
		"no terror here":                                "info",
		"warning and error on the same line":            "error",
		"forewarned is forearmed":                       "info",
		"[2026-10-29 10:00:00] app.WARN retry":          "warn",
		"json.decoder.JSONDecodeError: Expecting value": "error", // CamelCase exception names
		"java.lang.NullPointerException":                "error",
		"ValueError":                                    "error",
		"errorhandler registered":                       "info", // lowercase compound, not an exception name
	}
	for line, want := range cases {
		if got := lib.LogLevelForLine(line); got != want {
			t.Errorf("LogLevelForLine(%q) = %q, want %q", line, got, want)
		}
	}
}

func TestIngestLogEnvelopeMarshal(t *testing.T) {
	ts := time.Date(2026, 10, 29, 10, 0, 0, 500000000, time.UTC)
	attrs := map[string]lib.LogAttribute{
		"cronitor.monitor": lib.StringAttribute("nightly"),
		"line":             lib.IntAttribute(7),
	}
	records := []lib.LogRecord{
		lib.NewLogRecord("ERROR: boom", "error", ts, "33.11", attrs),
		lib.NewLogRecord(strings.Repeat("x", 9000), "info", ts, "33.11", nil),
	}
	env, err := lib.NewLogEnvelope(records, "33.11")
	if err != nil {
		t.Fatal(err)
	}
	header, items := parseEnvelope(t, env.Marshal())
	if _, err := time.Parse(time.RFC3339, header["sent_at"].(string)); err != nil {
		t.Errorf("sent_at %v", header["sent_at"])
	}
	if len(items) != 1 {
		t.Fatalf("want 1 log item, got %d", len(items))
	}
	ih := items[0].Header
	if ih["type"] != "log" || ih["content_type"] != "application/vnd.sentry.items.log+json" || ih["item_count"] != float64(2) {
		t.Errorf("log item header %v", ih)
	}

	var payload struct {
		Items []struct {
			Timestamp  float64                           `json:"timestamp"`
			Level      string                            `json:"level"`
			Body       string                            `json:"body"`
			Attributes map[string]map[string]interface{} `json:"attributes"`
		} `json:"items"`
	}
	if err := json.Unmarshal(items[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	first := payload.Items[0]
	if first.Timestamp != 1793268000.5 || first.Level != "error" || first.Body != "ERROR: boom" {
		t.Errorf("first record %+v", first)
	}
	want := map[string][2]interface{}{
		"cronitor.monitor":       {"nightly", "string"},
		"line":                   {float64(7), "integer"},
		"sentry.severity_number": {float64(17), "integer"},
		"sentry.severity_text":   {"error", "string"},
		"sentry.sdk.name":        {"cronitor-cli", "string"},
		"sentry.sdk.version":     {"33.11", "string"},
	}
	for k, v := range want {
		a := first.Attributes[k]
		if a["value"] != v[0] || a["type"] != v[1] {
			t.Errorf("attribute %s = %v, want %v", k, a, v)
		}
	}
	second := payload.Items[1]
	if len(second.Body) != 8000 {
		t.Errorf("body must be truncated to 8000 chars, got %d", len(second.Body))
	}
	if second.Attributes["sentry.severity_number"]["value"] != float64(9) {
		t.Errorf("info severity %v", second.Attributes["sentry.severity_number"])
	}
	if lib.NewLogRecord("w", "warn", ts, "1", nil).Attributes["sentry.severity_number"].Value != 13 {
		t.Error("warn severity must be 13")
	}
}

func gunzip(t *testing.T, body string) []byte {
	t.Helper()
	out, err := testutil.Gunzip(body)
	if err != nil {
		t.Fatalf("body is not gzip: %v", err)
	}
	return out
}

func TestIngestSendEnvelope(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("POST", "/api/envelope", 200, `{"id":"x"}`)

	ev := lib.NewExceptionEvent("ValueError", "boom", "python", nil, "33.11")
	env, _ := lib.NewEventEnvelope(ev)
	if err := lib.SendEnvelope(mock.Server.URL+"/", "telemetrykey", "CronitorCLI/33.11", env); err != nil {
		t.Fatal(err)
	}
	if mock.RequestCount() != 1 {
		t.Fatalf("want 1 request, got %d", mock.RequestCount())
	}
	req := mock.LastRequest()
	if req.Method != "POST" || req.Path != "/api/envelope" {
		t.Errorf("request %s %s", req.Method, req.Path)
	}
	h := req.Headers
	if h.Get("Content-Type") != "application/x-sentry-envelope" || h.Get("Content-Encoding") != "gzip" {
		t.Errorf("content headers %v", h)
	}
	if got := h.Get("X-Sentry-Auth"); got != "Sentry sentry_key=telemetrykey, sentry_version=7, sentry_client=CronitorCLI/33.11" {
		t.Errorf("X-Sentry-Auth %q", got)
	}
	if h.Get("User-Agent") != "CronitorCLI/33.11" {
		t.Errorf("User-Agent %q", h.Get("User-Agent"))
	}
	if !bytes.Equal(gunzip(t, req.Body), env.Marshal()) {
		t.Error("decompressed body must equal the marshaled envelope")
	}
}

func TestIngestSendEnvelopeNon2xxReturnsBody(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	mock.On("POST", "/api/envelope", 403, `{"detail":"key lacks monitor:telemetry"}`)

	env, _ := lib.NewEventEnvelope(lib.NewExceptionEvent("E", "v", "other", nil, "1"))
	err := lib.SendEnvelope(mock.Server.URL, "k", "CronitorCLI/1", env)
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "monitor:telemetry") {
		t.Fatalf("want error with status and body, got %v", err)
	}
	if mock.RequestCount() != 1 {
		t.Errorf("non-2xx must not be retried, got %d requests", mock.RequestCount())
	}
}

func TestIngestSendEnvelopeRetriesOnceOnNetworkError(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n == 1 {
			// Drop the connection without a response: a network error for the client.
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				conn.(*net.TCPConn).SetLinger(0)
				conn.Close()
			}
			return
		}
		io.Copy(io.Discard, r.Body)
		w.WriteHeader(200)
	}))
	defer server.Close()

	env, _ := lib.NewEventEnvelope(lib.NewExceptionEvent("E", "v", "other", nil, "1"))
	if err := lib.SendEnvelope(server.URL, "k", "CronitorCLI/1", env); err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 {
		t.Errorf("want 2 attempts, got %d", attempts)
	}
}

func TestIngestSendEnvelopeGivesUpAfterOneRetry(t *testing.T) {
	// Nothing listens on this port once the listener is closed.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()

	env, _ := lib.NewEventEnvelope(lib.NewExceptionEvent("E", "v", "other", nil, "1"))
	start := time.Now()
	if err := lib.SendEnvelope("http://"+addr, "k", "CronitorCLI/1", env); err == nil {
		t.Fatal("want an error when the host is unreachable")
	}
	if time.Since(start) > 5*time.Second {
		t.Errorf("unreachable host took %v", time.Since(start))
	}
}

func TestIngestTruncationKeepsCharactersWhole(t *testing.T) {
	if got := lib.TruncateChars("héllo", 2); got != "hé" {
		t.Errorf("TruncateChars = %q", got)
	}
	if got := lib.TruncateChars("abc", 10); got != "abc" {
		t.Errorf("TruncateChars short = %q", got)
	}
	// "é" is two bytes; cutting into it must skip to the next character.
	if got := lib.TailBytes("aé!", 2); got != "!" {
		t.Errorf("TailBytes = %q", got)
	}
	if got := lib.TailBytes("abcdef", 3); got != "def" {
		t.Errorf("TailBytes ascii = %q", got)
	}
}
