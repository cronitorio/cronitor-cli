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
Each poll reads back a minute before the newest record printed, so records
that arrive late (SDKs send in batches) still print, after newer ones.
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

		f := newLogFollower(q.Start)
		records := reverseLogRecords(page.records)
		if page.HasMore && len(records) > 0 {
			// Older records were left out of the first page; polls must
			// not print them as if they had just arrived.
			f.raiseFloor(records[0].Stamp)
		}
		f.print(records, jsonOut, logsAttributes)
		if f.newest.IsZero() && f.rawStart == "" {
			// Nothing yet: follow from now.
			f.newest = nowFn()
		}

		ctx, stop := logsFollowContext()
		defer stop()
		for logsFollowWait(ctx, logsFollowInterval) {
			pq := q
			pq.Start = f.nextStart()
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
					fmt.Fprintln(os.Stderr, warningStyle.Render(fmt.Sprintf("%s More than %d pages of records since %s; older ones were skipped", iconWarning, logsFollowMaxPages, pq.Start)))
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

// logFollowLag is how far before the newest record printed each --follow
// poll starts. Stamps are set by the sender, and SDKs send records in
// batches (cronitor-python every 5 seconds), so a record can arrive after a
// newer one was printed. Reading this window again, and skipping records
// already printed, shows those late records.
const logFollowLag = 60 * time.Second

// logKey identifies a record across --follow polls, which return most
// records more than once.
type logKey struct {
	stamp, body, series, monitor, service string
}

// logFollower prints the records of each --follow poll that it has not
// printed before.
type logFollower struct {
	newest   time.Time            // newest stamp printed, or where following started
	floor    time.Time            // polls never start before this: --since/--start, or the oldest record of a first page that had more
	rawStart string               // a --start this could not parse; sent as start= until a record prints
	cutoff   time.Time            // start of the current poll; records before it are not printed
	seen     map[logKey]time.Time // records printed at or after cutoff, by stamp
}

// newLogFollower follows from start (unix seconds or an ISO time); empty
// leaves the first read unbounded.
func newLogFollower(start string) *logFollower {
	f := &logFollower{seen: map[logKey]time.Time{}}
	if start == "" {
		return f
	}
	t, ok := parseAPITime(start)
	if secs, err := strconv.ParseInt(start, 10, 64); err == nil {
		t, ok = time.Unix(secs, 0), true
	}
	if !ok {
		f.rawStart = start
		return f
	}
	f.newest, f.floor, f.cutoff = t, t, t
	return f
}

// raiseFloor keeps polls from starting before stamp.
func (f *logFollower) raiseFloor(stamp string) {
	if t, ok := parseAPITime(stamp); ok && t.After(f.floor) {
		f.floor = t
	}
}

// nextStart returns start= for the next poll, logFollowLag before the
// newest record printed but not before the floor, and forgets records
// older than that: the poll cannot return them.
func (f *logFollower) nextStart() string {
	if f.newest.IsZero() {
		return f.rawStart
	}
	start := f.newest.Add(-logFollowLag)
	if start.Before(f.floor) {
		start = f.floor
	}
	f.cutoff = start
	for k, t := range f.seen {
		if t.Before(start) {
			delete(f.seen, k)
		}
	}
	return lib.FormatIngestTime(start)
}

// print writes, in the order given (oldest first), the records it has not
// printed before.
func (f *logFollower) print(records []logRecord, jsonOut, withAttributes bool) {
	for _, r := range records {
		t, ok := parseAPITime(r.Stamp)
		if ok && t.Before(f.cutoff) {
			continue
		}
		key := logKey{stamp: r.Stamp, body: r.Body, series: r.Series, monitor: refKey(r.Monitor), service: refKey(r.Service)}
		if _, printed := f.seen[key]; printed {
			continue
		}
		if !ok {
			t = f.newest // no stamp to age it by: remember it for one window
		} else {
			if f.rawStart != "" && f.floor.IsZero() {
				// The API applied a start this could not parse; the
				// oldest record it returned stands in for it.
				f.floor = t
			}
			if t.After(f.newest) {
				f.newest = t
			}
		}
		f.seen[key] = t
		if jsonOut {
			fmt.Println(string(r.raw))
		} else {
			fmt.Println(formatLogLine(r, withAttributes))
		}
	}
}
