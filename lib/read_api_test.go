package lib_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
)

func TestErrorsQueryParams(t *testing.T) {
	q := lib.ErrorGroupQuery{
		Env:      "production",
		Monitor:  "nightly-job",
		Service:  "billing-api",
		Levels:   []string{"fatal", "WARN", "error"},
		Start:    "1761700000",
		End:      "2026-10-29T00:00:00Z",
		Search:   "decode error",
		OrderBy:  "-count",
		Page:     2,
		PageSize: 100,
	}
	want := map[string]string{
		"env":      "production",
		"monitor":  "nightly-job",
		"service":  "billing-api",
		"level":    "fatal,warning,error",
		"start":    "1761700000",
		"end":      "2026-10-29T00:00:00Z",
		"q":        "decode error",
		"orderBy":  "-count",
		"page":     "2",
		"pageSize": "100",
	}
	assertParams(t, q.Params(), want)

	// Zero values send nothing, and page 1 is the API default.
	assertParams(t, lib.ErrorGroupQuery{Page: 1}.Params(), map[string]string{})
}

func TestErrorsEventQueryParams(t *testing.T) {
	q := lib.ErrorEventQuery{Env: "staging", Monitor: "m", Service: "s", Series: "run-1", Before: "1761700000", Limit: 25}
	assertParams(t, q.Params(), map[string]string{
		"env": "staging", "monitor": "m", "service": "s", "series": "run-1", "before": "1761700000", "limit": "25",
	})
}

func TestLogsQueryParams(t *testing.T) {
	q := lib.LogQuery{
		Env: "production", Monitor: "nightly-job", Service: "billing-api", Series: "run-123",
		Level: "Warning", Start: "1761700000", End: "1761703600", Search: "timeout db",
		TraceID: "abc123", Before: "2026-10-29T03:00:12.000000Z", Limit: 500,
	}
	assertParams(t, q.Params(), map[string]string{
		"env": "production", "monitor": "nightly-job", "service": "billing-api", "series": "run-123",
		"level": "warn", "start": "1761700000", "end": "1761703600", "q": "timeout db",
		"trace_id": "abc123", "before": "2026-10-29T03:00:12.000000Z", "limit": "500",
	})
}

func TestErrorsPaths(t *testing.T) {
	g := strings.Repeat("ab", 20)
	if got := lib.ErrorGroupPath(g); got != "/errors/"+g {
		t.Errorf("ErrorGroupPath = %q", got)
	}
	if got := lib.ErrorEventsPath(g); got != "/errors/"+g+"/events" {
		t.Errorf("ErrorEventsPath = %q", got)
	}
	e := strings.Repeat("cd", 16)
	if got := lib.ErrorEventPath(g, e); got != "/errors/"+g+"/events/"+e {
		t.Errorf("ErrorEventPath = %q", got)
	}
}

func TestErrorsNormalizeGroupID(t *testing.T) {
	full := "A95936E78B4B51BF4ABDBB64D45624CA804C2F4A"
	got, err := lib.NormalizeErrorGroupID(" " + full + " ")
	if err != nil || got != strings.ToLower(full) {
		t.Fatalf("NormalizeErrorGroupID(full) = %q, %v", got, err)
	}
	for _, bad := range []string{"", "a95936e78b4b", "a95936e78b4b51bf4abdbb64d45624ca804c2f4", "z95936e78b4b51bf4abdbb64d45624ca804c2f4a", "../errors"} {
		if _, err := lib.NormalizeErrorGroupID(bad); err == nil {
			t.Errorf("NormalizeErrorGroupID(%q) accepted", bad)
		} else if !strings.Contains(err.Error(), "40") {
			t.Errorf("error should name the 40-character requirement: %v", err)
		}
	}
}

func TestErrorsNormalizeEventID(t *testing.T) {
	got, err := lib.NormalizeErrorEventID("CD3DC8B6-CFFB-41E4-163D-CBD857CA87DA")
	if err != nil || got != "cd3dc8b6cffb41e4163dcbd857ca87da" {
		t.Fatalf("NormalizeErrorEventID(uuid) = %q, %v", got, err)
	}
	if _, err := lib.NormalizeErrorEventID("cd3dc8b6"); err == nil {
		t.Error("short event id accepted")
	}
}

func TestErrorsParseLookback(t *testing.T) {
	cases := map[string]time.Duration{
		"30s":   30 * time.Second,
		"15m":   15 * time.Minute,
		"24h":   24 * time.Hour,
		"7d":    7 * 24 * time.Hour,
		"2w":    14 * 24 * time.Hour,
		"1d12h": 36 * time.Hour,
		"1h30m": 90 * time.Minute,
	}
	for in, want := range cases {
		got, err := lib.ParseLookback(in)
		if err != nil || got != want {
			t.Errorf("ParseLookback(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "0h", "7", "d", "-1h", "1y", "1.5h"} {
		if _, err := lib.ParseLookback(bad); err == nil {
			t.Errorf("ParseLookback(%q) accepted", bad)
		}
	}
}

func TestErrorsDecodeDetail(t *testing.T) {
	body := `{
		"group": "a95936e78b4b51bf4abdbb64d45624ca804c2f4a", "title": "T", "count": 3,
		"monitors": [{"key": "nightly-job", "name": "Nightly"}],
		"sparkline": [{"stamp": "2026-10-29T02:00:00Z", "count": 2}],
		"latest_event": {"event_id": "cd3dc8b6cffb41e4163dcbd857ca87da", "handled": false,
			"monitor": {"key": "nightly-job", "name": "Nightly"}, "service": null,
			"tags": {"exit_code": "1"}, "payload": {"exception": {"values": []}}}
	}`
	var d lib.ErrorGroupDetail
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatal(err)
	}
	if d.Group != "a95936e78b4b51bf4abdbb64d45624ca804c2f4a" || d.Count != 3 || d.Monitors[0].Key != "nightly-job" {
		t.Errorf("group fields = %+v", d.ErrorGroup)
	}
	if len(d.Sparkline) != 1 || d.Sparkline[0].Count != 2 {
		t.Errorf("sparkline = %+v", d.Sparkline)
	}
	ev := d.LatestEvent
	if ev == nil || ev.EventID != "cd3dc8b6cffb41e4163dcbd857ca87da" || ev.Monitor == nil || ev.Service != nil {
		t.Fatalf("latest_event = %+v", ev)
	}
	if ev.Handled == nil || *ev.Handled {
		t.Errorf("handled = %v", ev.Handled)
	}
	if len(ev.Payload) == 0 {
		t.Error("payload not kept")
	}
}

func TestErrorsCursorDecodes(t *testing.T) {
	for body, want := range map[string]string{
		`{"data": [], "has_more": true, "next": "2026-10-29T03:00:12.000000Z"}`: "2026-10-29T03:00:12.000000Z",
		`{"data": [], "has_more": true, "next": 1761706812}`:                    "1761706812",
		`{"data": [], "has_more": false, "next": null}`:                         "",
	} {
		var page lib.LogPage
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if string(page.Next) != want {
			t.Errorf("next = %q, want %q", page.Next, want)
		}
	}
}

func assertParams(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("params = %v, want %v", got, want)
		return
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("param %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}
