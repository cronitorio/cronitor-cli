package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var errorsCmd = &cobra.Command{
	Use:     "errors",
	Aliases: []string{"error"},
	Short:   "Read error groups and error events",
	Long: `Read the errors Cronitor has received from 'cronitor exec --capture-errors',
cronitor-python, or any Sentry SDK that sends to a Cronitor DSN.

Events with the same fingerprint form an error group, identified by a
40-character hex id. Tables show its first 12 characters; 'get' and
'events' need the full id, which --json prints.

Needs an API key with the monitor:read scope.

Examples:
  cronitor errors list
  cronitor errors list --monitor nightly-job --since 24h
  cronitor errors list --level fatal --level error --order-by -count
  cronitor errors get <group>
  cronitor errors events <group>
  cronitor errors events <group> <event_id>`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var (
	errorsJSON   bool
	errorsFormat string
	// list and events filters
	errorsEnv      string
	errorsMonitor  string
	errorsService  string
	errorsLevels   []string
	errorsSince    string
	errorsStart    string
	errorsEnd      string
	errorsSearch   string
	errorsOrderBy  string
	errorsPage     int
	errorsPageSize int
	errorsSeries   string
	errorsBefore   string
	errorsLimit    int
)

func resetErrorsFlags() {
	errorsJSON = false
	errorsFormat = ""
	errorsEnv = ""
	errorsMonitor = ""
	errorsService = ""
	errorsLevels = nil
	errorsSince = ""
	errorsStart = ""
	errorsEnd = ""
	errorsSearch = ""
	errorsOrderBy = ""
	errorsPage = 1
	errorsPageSize = 0
	errorsSeries = ""
	errorsBefore = ""
	errorsLimit = 0
}

func init() {
	RootCmd.AddCommand(errorsCmd)
	errorsCmd.PersistentPreRunE = readCommandPreRun(&errorsFormat)
	errorsCmd.PersistentFlags().BoolVar(&errorsJSON, "json", false, "Print the API response as JSON")
	errorsCmd.PersistentFlags().StringVar(&errorsFormat, "format", "", "Output format: json, table (--format json is the same as --json)")
}

// --- LIST ---
var errorsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List error groups",
	Long: `List error groups, most recently seen first.

--env filters by environment key. --since takes a duration (30m, 24h, 7d,
2w) and sends start=now minus that duration. --start and --end take unix
seconds or an ISO 8601 time.

Examples:
  cronitor errors list
  cronitor errors list --env production --since 7d
  cronitor errors list --monitor nightly-job
  cronitor errors list --service billing-api --level fatal --level error
  cronitor errors list -q JSONDecodeError --order-by -count
  cronitor errors list --page 2 --page-size 100
  cronitor errors list --json`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		start, err := readAPIStart(errorsSince, errorsStart)
		if err != nil {
			failAndExit(err.Error())
			return
		}
		q := lib.ErrorGroupQuery{
			Env:      errorsEnv,
			Monitor:  errorsMonitor,
			Service:  errorsService,
			Levels:   errorsLevels,
			Start:    start,
			End:      errorsEnd,
			Search:   errorsSearch,
			OrderBy:  errorsOrderBy,
			Page:     errorsPage,
			PageSize: errorsPageSize,
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.GET(lib.ErrorGroupsPath, q.Params())
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to list error groups: %s", err))
			return
		}
		if !resp.IsSuccess() {
			failReadAPI(resp)
			return
		}
		if readJSONOutput(errorsJSON, errorsFormat) {
			fmt.Println(FormatJSON(resp.Body))
			return
		}

		var page lib.ErrorGroupPage
		if err := json.Unmarshal(resp.Body, &page); err != nil {
			failAndExit(fmt.Sprintf("Failed to parse response: %s", err))
			return
		}
		fmt.Println(renderErrorGroupTable(page, nowFn()))
	},
}

func init() {
	errorsCmd.AddCommand(errorsListCmd)
	resetErrorsFlags()

	f := errorsListCmd.Flags()
	f.StringVar(&errorsEnv, "env", "", "Filter by environment key")
	f.StringVar(&errorsMonitor, "monitor", "", "Filter by job monitor key or code")
	f.StringVar(&errorsService, "service", "", "Filter by service monitor key or code")
	f.StringSliceVar(&errorsLevels, "level", nil, "Filter by level: fatal, error, warning, info, debug (repeatable or comma-separated)")
	f.StringVar(&errorsSince, "since", "", "Only groups seen in this long: 30m, 24h, 7d, 2w (sets start)")
	f.StringVar(&errorsStart, "start", "", "Range start: unix seconds or ISO 8601")
	f.StringVar(&errorsEnd, "end", "", "Range end: unix seconds or ISO 8601")
	f.StringVarP(&errorsSearch, "search", "q", "", "Search titles and culprits")
	f.StringVar(&errorsOrderBy, "order-by", "", "Sort: -last_seen (default), last_seen, -count, count, -first_seen, first_seen")
	f.IntVar(&errorsPage, "page", 1, "Page number")
	f.IntVar(&errorsPageSize, "page-size", 0, "Groups per page (default 50, max 200)")
}

func renderErrorGroupTable(page lib.ErrorGroupPage, now time.Time) string {
	if len(page.Data) == 0 {
		return mutedStyle.Render("No error groups found")
	}
	table := &UITable{
		Headers: []string{"GROUP", "LEVEL", "COUNT", "LAST SEEN", "TITLE", "MONITORS", "SERVICES"},
	}
	for _, g := range page.Data {
		table.Rows = append(table.Rows, []string{
			shortGroupID(g.Group),
			styleLevel(g.Level, g.Level),
			fmt.Sprintf("%d", g.Count),
			relativeTime(g.LastSeen, now),
			truncateRunes(g.Title, 40),
			refKeys(g.Monitors),
			refKeys(g.Services),
		})
	}
	output := table.Render()
	if page.TotalCount > 0 {
		output += "\n" + mutedStyle.Render(fmt.Sprintf("Showing page %d of %d • %d error groups total",
			page.Page, page.PageCount, page.TotalCount))
	}
	return output
}

// --- GET ---
var errorsGetCmd = &cobra.Command{
	Use:   "get <group>",
	Short: "Show an error group and its latest event",
	Long: `Show an error group: a summary, its hourly counts for the last 24 hours, and
the latest event with its stack trace, oldest frame first. Library frames
are marked [lib].

<group> is the full 40-character hex id; 'cronitor errors list --json'
prints it.

Examples:
  cronitor errors get <group>
  cronitor errors get <group> --json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		group := errorGroupArg(args[0])
		client := lib.NewAPIClient(dev, log)
		resp, err := client.GET(lib.ErrorGroupPath(group), nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to get error group: %s", err))
			return
		}
		if !resp.IsSuccess() {
			failReadAPI(resp)
			return
		}
		if readJSONOutput(errorsJSON, errorsFormat) {
			fmt.Println(FormatJSON(resp.Body))
			return
		}
		var detail lib.ErrorGroupDetail
		if err := json.Unmarshal(resp.Body, &detail); err != nil {
			failAndExit(fmt.Sprintf("Failed to parse response: %s", err))
			return
		}
		fmt.Print(renderErrorGroupDetail(detail, nowFn()))
	},
}

// --- EVENTS ---
var errorsEventsCmd = &cobra.Command{
	Use:   "events <group> [event_id]",
	Short: "List a group's events, or show one event",
	Long: `Without an event id, list the events in an error group, newest first. When
there are more, the footer prints a cursor to pass to --before. --env
filters by environment key.

With an event id, show that event with its stack trace, breadcrumb count and
tags.

Examples:
  cronitor errors events <group>
  cronitor errors events <group> --monitor nightly-job --limit 20
  cronitor errors events <group> --before 2026-10-28T23:10:05.000001Z
  cronitor errors events <group> <event_id>
  cronitor errors events <group> <event_id> --json`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		group := errorGroupArg(args[0])
		client := lib.NewAPIClient(dev, log)

		if len(args) == 2 {
			eventID, err := lib.NormalizeErrorEventID(args[1])
			if err != nil {
				failAndExit(err.Error())
				return
			}
			resp, err := client.GET(lib.ErrorEventPath(group, eventID), nil)
			if err != nil {
				failAndExit(fmt.Sprintf("Failed to get error event: %s", err))
				return
			}
			if !resp.IsSuccess() {
				failReadAPI(resp)
				return
			}
			if readJSONOutput(errorsJSON, errorsFormat) {
				fmt.Println(FormatJSON(resp.Body))
				return
			}
			var ev lib.ErrorEvent
			if err := json.Unmarshal(resp.Body, &ev); err != nil {
				failAndExit(fmt.Sprintf("Failed to parse response: %s", err))
				return
			}
			fmt.Print(renderErrorEventDetail(ev, nowFn()))
			return
		}

		q := lib.ErrorEventQuery{
			Env:     errorsEnv,
			Monitor: errorsMonitor,
			Service: errorsService,
			Series:  errorsSeries,
			Before:  errorsBefore,
			Limit:   errorsLimit,
		}
		resp, err := client.GET(lib.ErrorEventsPath(group), q.Params())
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to list error events: %s", err))
			return
		}
		if !resp.IsSuccess() {
			failReadAPI(resp)
			return
		}
		if readJSONOutput(errorsJSON, errorsFormat) {
			fmt.Println(FormatJSON(resp.Body))
			return
		}
		var page lib.ErrorEventPage
		if err := json.Unmarshal(resp.Body, &page); err != nil {
			failAndExit(fmt.Sprintf("Failed to parse response: %s", err))
			return
		}
		fmt.Println(renderErrorEventTable(page))
	},
}

func init() {
	errorsCmd.AddCommand(errorsGetCmd)
	errorsCmd.AddCommand(errorsEventsCmd)

	f := errorsEventsCmd.Flags()
	f.StringVar(&errorsEnv, "env", "", "Filter by environment key")
	f.StringVar(&errorsMonitor, "monitor", "", "Filter by job monitor key or code")
	f.StringVar(&errorsService, "service", "", "Filter by service monitor key or code")
	f.StringVar(&errorsSeries, "series", "", "Filter by run series")
	f.StringVar(&errorsBefore, "before", "", "Cursor: events before this time (unix seconds or ISO 8601, or a printed next: value)")
	f.IntVar(&errorsLimit, "limit", 0, "Events to return (default 50, max 200)")
}

// errorGroupArg validates a group id argument and exits when it is not a
// full id.
func errorGroupArg(arg string) string {
	group, err := lib.NormalizeErrorGroupID(arg)
	if err != nil {
		failAndExit(err.Error() + ". Tables show the first 12 characters; 'cronitor errors list --json' prints full ids")
		return ""
	}
	return group
}

// --- shared by the errors and logs commands ---

// readCommandPreRun checks for an API key and a valid --format before any
// errors or logs subcommand runs.
func readCommandPreRun(format *string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("API key required. Run 'cronitor configure' or use --api-key flag")
		}
		return validateOutputFormat(*format)
	}
}

func readJSONOutput(jsonFlag bool, format string) bool {
	return jsonFlag || format == "json"
}

// readAPIStart returns the start= value: now minus --since, or --start.
func readAPIStart(since, start string) (string, error) {
	if since == "" {
		return start, nil
	}
	if start != "" {
		return "", errors.New("use --since or --start, not both")
	}
	s, err := lib.LookbackStart(since, nowFn())
	if err != nil {
		return "", fmt.Errorf("invalid --since: %s", err)
	}
	return s, nil
}

func failReadAPI(resp *lib.APIResponse) {
	msg := fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
	if resp.StatusCode == 403 {
		msg += " (errors and logs need an API key with the monitor:read scope)"
	}
	failAndExit(msg)
}

func styleLevel(level, text string) string {
	switch strings.ToLower(level) {
	case "fatal", "error", "critical":
		return errorStyle.Render(text)
	case "warning", "warn":
		return warningStyle.Render(text)
	case "debug", "trace":
		return mutedStyle.Render(text)
	}
	return text
}

func shortGroupID(group string) string {
	if len(group) > 12 {
		return group[:12]
	}
	return group
}

func refKeys(refs []lib.ResourceRef) string {
	keys := make([]string, 0, len(refs))
	for _, r := range refs {
		keys = append(keys, r.Key)
	}
	return strings.Join(keys, ",")
}

// truncateRunes shortens s to at most n characters, ending with "…".
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n-1]) + "…"
}

// parseAPITime parses a read API timestamp (ISO 8601 UTC, microseconds).
func parseAPITime(stamp string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999", "2006-01-02 15:04:05.999999Z07:00"} {
		if t, err := time.Parse(layout, stamp); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// displayStamp prints a timestamp to the second, in UTC.
func displayStamp(stamp string) string {
	if t, ok := parseAPITime(stamp); ok {
		return t.UTC().Format("2006-01-02T15:04:05Z")
	}
	return stamp
}

// relativeTime prints how long before now a timestamp was, like "3m ago".
func relativeTime(stamp string, now time.Time) string {
	t, ok := parseAPITime(stamp)
	if !ok {
		return stamp
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		if d < time.Second {
			return "just now"
		}
		return fmt.Sprintf("%ds ago", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
}
