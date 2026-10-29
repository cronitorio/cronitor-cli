package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:     "logs",
	Aliases: []string{"log"},
	Short:   "Search log records",
	Long: `Search the log records Cronitor has received from 'cronitor exec --ship-logs',
cronitor-python, or any Sentry SDK that sends logs to a Cronitor DSN.

Needs an API key with the monitor:read scope.

Examples:
  cronitor logs search --monitor nightly-job --since 1h
  cronitor logs search --level warn -q timeout
  cronitor logs search --series run-123 --attributes
  cronitor logs search --monitor nightly-job --follow`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var (
	logsJSON       bool
	logsFormat     string
	logsEnv        string
	logsMonitor    string
	logsService    string
	logsSeries     string
	logsLevel      string
	logsSince      string
	logsStart      string
	logsEnd        string
	logsSearch     []string
	logsTraceID    string
	logsBefore     string
	logsLimit      int
	logsFollow     bool
	logsAttributes bool
)

func resetLogsFlags() {
	logsJSON = false
	logsFormat = ""
	logsEnv = ""
	logsMonitor = ""
	logsService = ""
	logsSeries = ""
	logsLevel = ""
	logsSince = ""
	logsStart = ""
	logsEnd = ""
	logsSearch = nil
	logsTraceID = ""
	logsBefore = ""
	logsLimit = 0
	logsFollow = false
	logsAttributes = false
}

// Test hooks for --follow.
var (
	logsFollowInterval = 2 * time.Second

	// logsFollowWait waits d and reports whether to keep following.
	logsFollowWait = func(ctx context.Context, d time.Duration) bool {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-t.C:
			return true
		}
	}

	logsFollowContext = func() (context.Context, context.CancelFunc) {
		return signal.NotifyContext(context.Background(), os.Interrupt)
	}
)

// logsFollowMaxPages caps how many pages one --follow poll reads.
const logsFollowMaxPages = 10

var logsSearchCmd = &cobra.Command{
	Use:     "search",
	Aliases: []string{"list"},
	Short:   "Search log records",
	Long: `Search log records. Records print one per line, oldest first:

  STAMP  LEVEL  MONITOR  SERIES  BODY

MONITOR is the job or service monitor key, and "-" marks an empty field.
Line breaks in a body print as \n. When there are older records, a "next:"
cursor for --before is printed to stderr, so stdout stays one record per
line. --json prints the API response, newest first.

--level is a minimum: trace, debug, info, warn, error, fatal. -q matches
whole tokens; repeat it or pass several words and every token must match.
--since takes a duration (30m, 24h, 7d) and sends start=now minus that.
--env filters by environment key.

--follow keeps polling every 2 seconds for newer records until Ctrl-C.
With --json it prints each record as one JSON object per line.

Examples:
  cronitor logs search
  cronitor logs search --monitor nightly-job --since 1h
  cronitor logs search --service billing-api --level error
  cronitor logs search -q timeout -q database --env production
  cronitor logs search --series run-123 --attributes
  cronitor logs search --trace-id 4bf92f3577b34da6a3ce929d0e0e4736
  cronitor logs search --before 2026-10-29T03:00:10.000000Z --limit 1000
  cronitor logs search --monitor nightly-job --follow
  cronitor logs list --json`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if logsFollow && (logsBefore != "" || logsEnd != "") {
			failAndExit("--follow cannot be combined with --before or --end")
			return
		}
		start, err := readAPIStart(logsSince, logsStart)
		if err != nil {
			failAndExit(err.Error())
			return
		}
		q := lib.LogQuery{
			Env:     logsEnv,
			Monitor: logsMonitor,
			Service: logsService,
			Series:  logsSeries,
			Level:   logsLevel,
			Start:   start,
			End:     logsEnd,
			Search:  strings.Join(logsSearch, " "),
			TraceID: logsTraceID,
			Before:  logsBefore,
			Limit:   logsLimit,
		}
		jsonOut := readJSONOutput(logsJSON, logsFormat)
		client := lib.NewAPIClient(dev, log)

		resp := getLogs(client, q)
		if jsonOut && !logsFollow {
			fmt.Println(FormatJSON(resp.Body))
			return
		}
		page := decodeLogPage(resp.Body)

		if !logsFollow {
			if len(page.records) == 0 {
				fmt.Fprintln(os.Stderr, mutedStyle.Render("No log records found"))
				return
			}
			for i := len(page.records) - 1; i >= 0; i-- {
				fmt.Println(formatLogLine(page.records[i], logsAttributes))
			}
			if page.HasMore && page.Next != "" {
				fmt.Fprintln(os.Stderr, mutedStyle.Render(fmt.Sprintf("next: %s  (pass it to --before for older records)", page.Next)))
			}
			return
		}

		f := &logFollower{seen: map[string]bool{}}
		f.startAt(q.Start)
		f.print(reverseLogRecords(page.records), jsonOut, logsAttributes)
		if f.lastStamp == "" {
			// Nothing yet: follow from now.
			f.startAt(strconv.FormatInt(nowFn().Unix(), 10))
		}

		ctx, stop := logsFollowContext()
		defer stop()
		for logsFollowWait(ctx, logsFollowInterval) {
			pq := q
			pq.Start = f.lastStamp
			pq.Before = ""
			var batch []logRecord
			for pages := 0; ; pages++ {
				resp, err := client.GET(lib.LogsPath, pq.Params())
				if err != nil {
					fmt.Fprintln(os.Stderr, warningStyle.Render(fmt.Sprintf("%s Failed to poll for log records: %s", iconWarning, err)))
					break
				}
				if resp.StatusCode == 429 || resp.StatusCode >= 500 {
					fmt.Fprintln(os.Stderr, warningStyle.Render(fmt.Sprintf("%s Polling for log records got API Error (%d); retrying", iconWarning, resp.StatusCode)))
					break
				}
				if !resp.IsSuccess() {
					failReadAPI(resp)
					return
				}
				p := decodeLogPage(resp.Body)
				batch = append(batch, p.records...)
				if !p.HasMore || p.Next == "" {
					break
				}
				if pages+1 >= logsFollowMaxPages {
					fmt.Fprintln(os.Stderr, warningStyle.Render(fmt.Sprintf("%s More than %d pages of new records since the last poll; older ones were skipped", iconWarning, logsFollowMaxPages)))
					break
				}
				pq.Before = string(p.Next)
			}
			f.print(reverseLogRecords(batch), jsonOut, logsAttributes)
		}
	},
}

func init() {
	RootCmd.AddCommand(logsCmd)
	logsCmd.PersistentPreRunE = readCommandPreRun(&logsFormat)
	logsCmd.PersistentFlags().BoolVar(&logsJSON, "json", false, "Print the API response as JSON")
	logsCmd.PersistentFlags().StringVar(&logsFormat, "format", "", "Output format: json, table (--format json is the same as --json)")

	logsCmd.AddCommand(logsSearchCmd)
	resetLogsFlags()
	f := logsSearchCmd.Flags()
	f.StringVar(&logsEnv, "env", "", "Filter by environment key")
	f.StringVar(&logsMonitor, "monitor", "", "Filter by job monitor key or code")
	f.StringVar(&logsService, "service", "", "Filter by service monitor key or code")
	f.StringVar(&logsSeries, "series", "", "Filter by run series")
	f.StringVar(&logsLevel, "level", "", "Minimum level: trace, debug, info, warn, error, fatal")
	f.StringVar(&logsSince, "since", "", "Only records from this long ago: 30m, 24h, 7d (sets start)")
	f.StringVar(&logsStart, "start", "", "Range start: unix seconds or ISO 8601")
	f.StringVar(&logsEnd, "end", "", "Range end: unix seconds or ISO 8601")
	f.StringArrayVarP(&logsSearch, "search", "q", nil, "Tokens the body must contain (repeatable; all must match)")
	f.StringVar(&logsTraceID, "trace-id", "", "Filter by trace id")
	f.StringVar(&logsBefore, "before", "", "Cursor: records before this time (unix seconds or ISO 8601, or a printed next: value)")
	f.IntVar(&logsLimit, "limit", 0, "Records to return (default 200, max 1000)")
	f.BoolVar(&logsFollow, "follow", false, "Keep polling every 2s for newer records until Ctrl-C")
	f.BoolVar(&logsAttributes, "attributes", false, "Print each record's attributes as key=value after the body")
}

func getLogs(client *lib.APIClient, q lib.LogQuery) *lib.APIResponse {
	resp, err := client.GET(lib.LogsPath, q.Params())
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to search logs: %s", err))
		return nil
	}
	if !resp.IsSuccess() {
		failReadAPI(resp)
		return nil
	}
	return resp
}

// logRecord is a decoded record and the JSON it came from, for --json
// output and to tell records apart.
type logRecord struct {
	lib.LogEntry
	raw json.RawMessage
}

type logPage struct {
	records []logRecord
	HasMore bool
	Next    lib.Cursor
}

func decodeLogPage(body []byte) logPage {
	var page struct {
		Data    []json.RawMessage `json:"data"`
		HasMore bool              `json:"has_more"`
		Next    lib.Cursor        `json:"next"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		failAndExit(fmt.Sprintf("Failed to parse response: %s", err))
		return logPage{}
	}
	out := logPage{HasMore: page.HasMore, Next: page.Next}
	for _, raw := range page.Data {
		var r logRecord
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&r.LogEntry); err != nil {
			failAndExit(fmt.Sprintf("Failed to parse log record: %s", err))
			return logPage{}
		}
		var compact bytes.Buffer
		if json.Compact(&compact, raw) == nil {
			r.raw = compact.Bytes()
		} else {
			r.raw = raw
		}
		out.records = append(out.records, r)
	}
	return out
}

func reverseLogRecords(records []logRecord) []logRecord {
	out := make([]logRecord, len(records))
	for i, r := range records {
		out[len(records)-1-i] = r
	}
	return out
}

var logBodyEscaper = strings.NewReplacer("\r\n", `\n`, "\n", `\n`, "\r", `\r`)

// formatLogLine renders "STAMP  LEVEL  MONITOR  SERIES  BODY".
func formatLogLine(r logRecord, withAttributes bool) string {
	source := refKey(r.Monitor)
	if source == "" {
		source = refKey(r.Service)
	}
	level := styleLevel(r.Level, fmt.Sprintf("%-5s", strings.ToUpper(r.Level)))
	line := fmt.Sprintf("%s  %s  %s  %s  %s", displayStamp(r.Stamp), level, dashIfEmpty(source), dashIfEmpty(r.Series), logBodyEscaper.Replace(r.Body))
	if withAttributes && len(r.Attributes) > 0 {
		line += "  " + mutedStyle.Render(formatLogAttributes(r.Attributes))
	}
	return line
}

// formatLogAttributes renders attributes as key=value, sorted by key.
// Typed attributes ({"value": v, "type": t}) print their value. Strings
// that are empty or hold spaces, quotes or = are quoted.
func formatLogAttributes(attrs map[string]interface{}) string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := attrs[k]
		if typed, ok := v.(map[string]interface{}); ok {
			if inner, ok := typed["value"]; ok {
				v = inner
			}
		}
		var s string
		switch val := v.(type) {
		case string:
			s = val
			if s == "" || strings.ContainsAny(s, " \t\n\"=") {
				s = strconv.Quote(s)
			}
		default:
			b, _ := json.Marshal(val)
			s = string(b)
		}
		parts = append(parts, k+"="+s)
	}
	return strings.Join(parts, " ")
}

// logFollower prints records newer than the last one it printed. Polls ask
// for start=<last stamp>, so records at that stamp come back again; they
// are recognized by their JSON and skipped.
type logFollower struct {
	lastStamp string    // sent as start= on the next poll
	last      time.Time // lastStamp parsed; zero when unknown
	seen      map[string]bool
}

// startAt sets the start= value of the next poll: unix seconds or an ISO
// time. Empty leaves the follower unbounded.
func (f *logFollower) startAt(start string) {
	if start == "" {
		return
	}
	f.lastStamp = start
	if secs, err := strconv.ParseInt(start, 10, 64); err == nil {
		f.last = time.Unix(secs, 0)
	} else if t, ok := parseAPITime(start); ok {
		f.last = t
	}
}

// print writes records, oldest first, that were not printed before.
func (f *logFollower) print(records []logRecord, jsonOut, withAttributes bool) {
	for _, r := range records {
		key := string(r.raw)
		t, ok := parseAPITime(r.Stamp)
		if ok && !f.last.IsZero() && t.Before(f.last) {
			continue
		}
		if f.seen[key] && (!ok || t.Equal(f.last)) {
			continue
		}
		if ok && (f.last.IsZero() || t.After(f.last)) {
			f.last = t
			f.lastStamp = r.Stamp
			f.seen = map[string]bool{}
		}
		f.seen[key] = true
		if jsonOut {
			fmt.Println(string(r.raw))
		} else {
			fmt.Println(formatLogLine(r, withAttributes))
		}
	}
}
