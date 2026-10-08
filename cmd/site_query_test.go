package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Pinned Sites API bodies for every `cronitor site query` example in --help.
// Keys are the example line with surrounding whitespace removed. Values are
// the exact JSON object POST /api/sites/query must send (key order is not
// significant; encoding/json sorts keys).
var wantSiteQueryBodies = map[string]string{
	"cronitor site query --site my-site --type aggregation --metric session_count":                                                        `{"aggregate":["session_count"],"kind":"aggregation","site":"my-site","time":"24h"}`,
	"cronitor site query --site my-site --type breakdown --metric web_vital_lcp_p50 --group-by country_code":                              `{"aggregate":["web_vital_lcp_p50"],"group_by":"country_code","kind":"breakdown","site":"my-site","time":"24h"}`,
	"cronitor site query --site my-site --type timeseries --metric session_count --bucket hour":                                           `{"aggregate":["session_count"],"kind":"timeseries","site":"my-site","time":"24h","time_bucket":"hour"}`,
	"cronitor site query --site my-site --type aggregation --metric session_count,web_vital_lcp_p50":                                      `{"aggregate":["session_count","web_vital_lcp_p50"],"kind":"aggregation","site":"my-site","time":"24h"}`,
	"cronitor site query --site my-site --type breakdown --metric session_count --group-by country_code":                                  `{"aggregate":["session_count"],"group_by":"country_code","kind":"breakdown","site":"my-site","time":"24h"}`,
	"cronitor site query --site my-site --type timeseries --metric pageview_count --bucket hour --time 7d":                                `{"aggregate":["pageview_count"],"kind":"timeseries","site":"my-site","time":"7d","time_bucket":"hour"}`,
	`cronitor site query --site my-site --type breakdown --metric web_vital_lcp_p50 --group-by browser --filter "device_type:eq:desktop"`: `{"aggregate":["web_vital_lcp_p50"],"filters":[{"dimension":"device_type","operator":"eq","value":"desktop"}],"group_by":"browser","kind":"breakdown","site":"my-site","time":"24h"}`,
	"cronitor site query --site my-site --type error_groups --metric message,error_type,error_count,last_seen --time 24h":                 `{"aggregate":["message","error_type","error_count","last_seen"],"kind":"error_groups","site":"my-site","time":"24h"}`,
}

func resetSiteQueryState() {
	sitePage = 1
	sitePageSize = 0
	siteFormat = ""
	siteOutput = ""
	verbose = false
	viper.Set("CRONITOR_LOG", "")
	for _, name := range []string{
		"site", "type", "kind", "time", "start", "end", "metric", "aggregate",
		"group-by", "filter", "order-by", "timezone", "bucket", "compare",
		"environment", "filters-behavior", "search",
	} {
		flag := siteQueryCmd.Flags().Lookup(name)
		if flag == nil {
			continue
		}
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}
	for _, name := range []string{"page", "page-size", "format", "output"} {
		flag := siteCmd.PersistentFlags().Lookup(name)
		if flag == nil {
			continue
		}
		_ = flag.Value.Set(flag.DefValue)
		flag.Changed = false
	}
	// `site query --help` leaves the help flag set, and the next Execute
	// would print help instead of querying.
	for _, c := range []*cobra.Command{RootCmd, siteCmd, siteQueryCmd} {
		if flag := c.Flags().Lookup("help"); flag != nil {
			_ = flag.Value.Set("false")
			flag.Changed = false
		}
	}
}

func captureSiteQueryBody(t *testing.T, args []string) string {
	t.Helper()
	resetSiteQueryState()
	t.Cleanup(func() {
		resetSiteQueryState()
		RootCmd.SetArgs([]string{"--help"})
	})

	var got string
	var path string
	var method string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		path = r.URL.Path
		method = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"session_count":{"value":0},"pageview_count":{"value":0}}}`))
	}))
	t.Cleanup(server.Close)

	cleanup := setupIntegrationTest(server.URL)
	t.Cleanup(cleanup)

	out, err := executeCmd(args...)
	if err != nil {
		t.Fatalf("execute %q: %v\n%s", strings.Join(args, " "), err, out)
	}
	if method != http.MethodPost || path != "/sites/query" {
		t.Fatalf("request = %s %s, want POST /sites/query\nbody %s\noutput %s", method, path, got, out)
	}
	if got == "" {
		t.Fatalf("empty request body\n%s", out)
	}
	return got
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	var g, w interface{}
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want is not JSON: %v\n%s", err, want)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("request body\n got: %s\nwant: %s", gb, wb)
	}
}

func assertNoLegacySiteQueryKeys(t *testing.T, body string) {
	t.Helper()
	var payload interface{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("body is not JSON: %v\n%s", err, body)
	}
	var keys []string
	var walk func(interface{})
	walk = func(v interface{}) {
		switch typed := v.(type) {
		case map[string]interface{}:
			for k, child := range typed {
				keys = append(keys, k)
				walk(child)
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(payload)
	for _, key := range keys {
		switch key {
		case "type", "metrics", "dimensions":
			t.Errorf("legacy key %q reappeared in %s", key, body)
		}
	}

	obj, ok := payload.(map[string]interface{})
	if !ok {
		t.Fatalf("body is not an object: %s", body)
	}
	if _, ok := obj["kind"].(string); !ok {
		t.Errorf("kind must be a string: %s", body)
	}
	if _, ok := obj["aggregate"].([]interface{}); !ok {
		t.Errorf("aggregate must be an array: %s", body)
	}
	if groupBy, ok := obj["group_by"]; ok {
		if _, isString := groupBy.(string); !isString {
			t.Errorf("group_by must be a string, got %T in %s", groupBy, body)
		}
	}
}

func siteQueryHelpExampleLines(helpText string) []string {
	var lines []string
	for _, line := range strings.Split(helpText, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "cronitor site query ") && strings.Contains(line, "--site ") {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestSiteQueryContract_HelpExamples(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	queryHelp, err := executeCmd("site", "query", "--help")
	if err != nil {
		t.Fatalf("site query --help: %v\n%s", err, queryHelp)
	}
	siteHelp, err := executeCmd("site", "--help")
	if err != nil {
		t.Fatalf("site --help: %v\n%s", err, siteHelp)
	}

	seen := map[string]bool{}
	var examples []string
	for _, line := range append(siteQueryHelpExampleLines(queryHelp), siteQueryHelpExampleLines(siteHelp)...) {
		if seen[line] {
			continue
		}
		seen[line] = true
		examples = append(examples, line)
	}
	if len(examples) < 5 {
		t.Fatalf("expected at least 5 site query examples in --help, got %d\n%s", len(examples), queryHelp)
	}
	for line := range wantSiteQueryBodies {
		if !seen[line] {
			t.Errorf("pinned body has no matching help example: %s", line)
		}
	}

	for _, line := range examples {
		line := line
		t.Run(line, func(t *testing.T) {
			want, ok := wantSiteQueryBodies[line]
			if !ok {
				t.Fatalf("help example has no pinned request body: %s", line)
			}
			if strings.Contains(line, "lcp_p50") && !strings.Contains(line, "web_vital_lcp_p50") {
				t.Fatalf("help example uses a metric name the server rejects: %s", line)
			}
			args, err := shellquote.Split(line)
			if err != nil {
				t.Fatalf("split example: %v", err)
			}
			if len(args) == 0 || args[0] != "cronitor" {
				t.Fatalf("example must start with cronitor: %q", line)
			}
			body := captureSiteQueryBody(t, args[1:])
			assertJSONEqual(t, body, want)
			assertNoLegacySiteQueryKeys(t, body)
		})
	}
}

func TestSiteQueryContract_AliasesAndExtraFields(t *testing.T) {
	body := captureSiteQueryBody(t, []string{
		"site", "query",
		"--site", "my-site",
		"--kind", "aggregation",
		"--aggregate", "session_count,pageview_count",
		"--time", "custom",
		"--start", "2025-01-01T00:00:00Z",
		"--end", "2025-01-31T23:59:59Z",
		"--timezone", "America/Los_Angeles",
		"--order-by", "-session_count,pageview_count",
		"--compare",
		"--environment", "production",
		"--filters-behavior", "or",
		"--search", "pricing",
		"--page", "2",
		"--page-size", "10",
		"--api-version", "2025-11-28",
	})
	assertJSONEqual(t, body, `{
		"aggregate": ["session_count", "pageview_count"],
		"compare": "previous_time_range",
		"end": "2025-01-31T23:59:59Z",
		"environment": "production",
		"filters_behavior": "or",
		"kind": "aggregation",
		"order_by": ["-session_count", "pageview_count"],
		"page": 2,
		"page_size": 10,
		"search": "pricing",
		"site": "my-site",
		"start": "2025-01-01T00:00:00Z",
		"time": "custom",
		"timezone": "America/Los_Angeles"
	}`)
	assertNoLegacySiteQueryKeys(t, body)
}

func TestSiteQueryContract_KindAliasMatchesType(t *testing.T) {
	withType := captureSiteQueryBody(t, []string{
		"site", "query", "--site", "my-site", "--type", "aggregation", "--metric", "session_count",
	})
	withKind := captureSiteQueryBody(t, []string{
		"site", "query", "--site", "my-site", "--kind", "aggregation", "--aggregate", "session_count",
	})
	assertJSONEqual(t, withKind, withType)
	assertJSONEqual(t, withType, `{"aggregate":["session_count"],"kind":"aggregation","site":"my-site","time":"24h"}`)
}

func TestSiteQuery_RejectsMultipleGroupBy(t *testing.T) {
	_, err := buildSiteQueryPayload(siteQueryInput{
		Site:    "my-site",
		Kind:    "breakdown",
		Metrics: "session_count",
		GroupBy: "country_code,browser",
		Time:    "24h",
	})
	if err == nil || !strings.Contains(err.Error(), "single dimension") {
		t.Fatalf("expected single-dimension error, got %v", err)
	}
}

func TestSiteQuery_BreakdownRequiresGroupBy(t *testing.T) {
	_, err := buildSiteQueryPayload(siteQueryInput{
		Site:    "my-site",
		Kind:    "breakdown",
		Metrics: "session_count",
		Time:    "24h",
	})
	if err == nil || !strings.Contains(err.Error(), "--group-by is required") {
		t.Fatalf("expected group-by required, got %v", err)
	}
}

func TestSiteQuery_ForwardsUnknownAggregateNames(t *testing.T) {
	payload, err := buildSiteQueryPayload(siteQueryInput{
		Site:    "my-site",
		Kind:    "aggregation",
		Metrics: "bounce_rate",
		Time:    "7d",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(payload)
	assertJSONEqual(t, string(body), `{"aggregate":["bounce_rate"],"kind":"aggregation","site":"my-site","time":"7d"}`)
	assertNoLegacySiteQueryKeys(t, string(body))
}

func TestSiteQuery_FilterValueMayContainColon(t *testing.T) {
	payload, err := buildSiteQueryPayload(siteQueryInput{
		Site:    "my-site",
		Kind:    "breakdown",
		Metrics: "pageview_count",
		GroupBy: "path",
		Filters: "path:startsWith:/docs:guide",
		Time:    "7d",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(payload)
	assertJSONEqual(t, string(body), `{
		"aggregate": ["pageview_count"],
		"filters": [{"dimension": "path", "operator": "startsWith", "value": "/docs:guide"}],
		"group_by": "path",
		"kind": "breakdown",
		"site": "my-site",
		"time": "7d"
	}`)
}

func TestSiteQuery_CustomRangeRequiresBounds(t *testing.T) {
	_, err := buildSiteQueryPayload(siteQueryInput{
		Site:    "my-site",
		Kind:    "aggregation",
		Metrics: "session_count",
		Time:    "custom",
	})
	if err == nil || !strings.Contains(err.Error(), "--start and --end") {
		t.Fatalf("expected custom range error, got %v", err)
	}
}

func TestSiteQuery_SearchOptionsRequiresSearchAndAggregate(t *testing.T) {
	_, err := buildSiteQueryPayload(siteQueryInput{
		Site: "my-site",
		Kind: "search_options",
		Time: "24h",
	})
	if err == nil || !strings.Contains(err.Error(), "--search is required") {
		t.Fatalf("expected search required, got %v", err)
	}
	_, err = buildSiteQueryPayload(siteQueryInput{
		Site:      "my-site",
		Kind:      "search_options",
		Search:    "pricing",
		SearchSet: true,
		Time:      "24h",
	})
	if err == nil || !strings.Contains(err.Error(), "--metric is required") {
		t.Fatalf("expected metric required, got %v", err)
	}
}

func TestSiteQuery_EmptySearchIsSent(t *testing.T) {
	body := captureSiteQueryBody(t, []string{
		"site", "query",
		"--site", "my-site",
		"--type", "search_options",
		"--metric", "path",
		"--search", "",
	})
	assertJSONEqual(t, body, `{"aggregate":["path"],"kind":"search_options","search":"","site":"my-site","time":"24h"}`)
	assertNoLegacySiteQueryKeys(t, body)
}

func TestSiteQuery_FormatTableUsesNestedAggregationValues(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(func() {
		resetSiteQueryState()
		RootCmd.SetArgs([]string{"--help"})
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"session_count":{"value":0},"pageview_count":{"value":0}}}`))
	}))
	t.Cleanup(server.Close)
	cleanup := setupIntegrationTest(server.URL)
	t.Cleanup(cleanup)

	out, err := executeCmd("site", "query", "--site", "my-site", "--type", "aggregation", "--metric", "session_count,pageview_count", "--format", "table")
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	for _, want := range []string{"METRIC", "session_count", "pageview_count"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "map[") {
		t.Errorf("table printed a raw map\n%s", out)
	}
}

func TestSiteQuery_MalformedFilter(t *testing.T) {
	_, err := buildSiteQueryPayload(siteQueryInput{
		Site:    "my-site",
		Kind:    "aggregation",
		Metrics: "session_count",
		Filters: "desktop",
		Time:    "24h",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid --filter") {
		t.Fatalf("expected invalid filter error, got %v", err)
	}
}

func TestSiteQuery_FlagDisagreement(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	if err := siteQueryCmd.Flags().Set("type", "aggregation"); err != nil {
		t.Fatal(err)
	}
	if err := siteQueryCmd.Flags().Set("kind", "breakdown"); err != nil {
		t.Fatal(err)
	}
	if _, err := siteQueryKindFromFlags(siteQueryCmd); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("expected kind disagreement, got %v", err)
	}

	resetSiteQueryState()
	if err := siteQueryCmd.Flags().Set("metric", "session_count"); err != nil {
		t.Fatal(err)
	}
	if err := siteQueryCmd.Flags().Set("aggregate", "pageview_count"); err != nil {
		t.Fatal(err)
	}
	if _, err := siteQueryMetricsFromFlags(siteQueryCmd); err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("expected aggregate disagreement, got %v", err)
	}
}

func TestRenderQueryTable_AggregationValueShape(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	body := []byte(`{"data":{"session_count":{"value":0},"pageview_count":{"value":0}}}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "aggregation", nil, "")
	})
	for _, want := range []string{"METRIC", "VALUE", "session_count", "pageview_count"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "map[") {
		t.Errorf("table printed a raw map instead of the metric value\n%s", out)
	}
	// Both values are zero. Require a standalone 0 cell, not the map syntax.
	if !strings.Contains(out, "0") {
		t.Errorf("table missing zero value\n%s", out)
	}
}

func TestRenderQueryTable_AggregationCompare(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	body := []byte(`{"data":{"session_count":{"value":15420,"previous_time_range_value":14200,"previous_time_range_change_rate":0.086}}}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "aggregation", nil, "")
	})
	for _, want := range []string{"PREVIOUS", "CHANGE", "session_count", "15420", "14200", "0.086"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
}

func TestRenderQueryTable_TimeseriesUsesTs(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	body := []byte(`{"data":[{"ts":"2025-12-01T00:00:00+00:00","session_count":520,"pageview_count":1240}],"time_bucket":"day"}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "timeseries", nil, "")
	})
	for _, want := range []string{"TIMESTAMP", "2025-12-01T00:00:00+00:00", "520", "1240"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
}

func TestRenderQueryTable_KeyedByKind(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	// The same aggregation document rendered as breakdown is not an aggregation table.
	body := []byte(`{"data":{"session_count":{"value":3}}}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "breakdown", nil, "")
	})
	if strings.Contains(out, "METRIC") {
		t.Fatalf("breakdown rendering treated an aggregation document as a metric table\n%s", out)
	}
}

func TestRenderQueryTable_BreakdownGroupByThenAggregates(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	// Alphabetical order would be COUNTRY_CODE, PAGEVIEW_COUNT, SESSION_COUNT.
	body := []byte(`{"data":[{"session_count":10,"pageview_count":20,"country_code":"US"}]}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "breakdown", []string{"session_count", "pageview_count"}, "country_code")
	})
	country := strings.Index(out, "COUNTRY_CODE")
	sessions := strings.Index(out, "SESSION_COUNT")
	pageviews := strings.Index(out, "PAGEVIEW_COUNT")
	if country < 0 || sessions < 0 || pageviews < 0 || !(country < sessions && sessions < pageviews) {
		t.Fatalf("expected group_by then aggregates in request order\n%s", out)
	}
}

func TestRenderQueryTable_ErrorGroupsKeyAndRequestedAggregates(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	// Realistic error_groups row: key plus the aggregates that were requested.
	// Request order is not alphabetical (that would be ERROR_COUNT, ERROR_TYPE, KEY, LAST_SEEN, MESSAGE).
	body := []byte(`{"data":[{"key":"TypeError:boom","message":"Cannot read property","error_type":"TypeError","error_count":12,"last_seen":"2025-12-01T00:00:00Z"}]}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "error_groups", []string{"message", "error_type", "error_count", "last_seen"}, "")
	})
	key := strings.Index(out, "KEY")
	message := strings.Index(out, "MESSAGE")
	errType := strings.Index(out, "ERROR_TYPE")
	errCount := strings.Index(out, "ERROR_COUNT")
	lastSeen := strings.Index(out, "LAST_SEEN")
	if key < 0 || message < 0 || errType < 0 || errCount < 0 || lastSeen < 0 ||
		!(key < message && message < errType && errType < errCount && errCount < lastSeen) {
		t.Fatalf("expected key then requested aggregates\n%s", out)
	}
	for _, want := range []string{"TypeError:boom", "Cannot read property", "TypeError", "12", "2025-12-01T00:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "FIRST SEEN") || strings.Contains(out, " COUNT ") {
		t.Errorf("table still has the old count column\n%s", out)
	}
}

func TestRenderQueryTable_ErrorGroupsKeyOnly(t *testing.T) {
	resetSiteQueryState()
	t.Cleanup(resetSiteQueryState)

	body := []byte(`{"data":[{"key":"grp_1"},{"key":"grp_2"}]}`)
	out := testutil.CaptureStdout(func() {
		renderQueryTable(body, "error_groups", nil, "")
	})
	for _, want := range []string{"KEY", "grp_1", "grp_2"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "MESSAGE") || strings.Contains(out, "COUNT") {
		t.Errorf("key-only rows rendered aggregate columns\n%s", out)
	}
}
