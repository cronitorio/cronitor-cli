package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Read API for error groups, error events and log records: GET /api/errors
// and GET /api/logs. These read what exec --capture-errors/--ship-logs,
// cronitor-python and Sentry SDKs sent. The cronitor errors and cronitor
// logs commands and the MCP tools share these types and query builders.

// ErrorGroupsPath and LogsPath are relative to APIBaseURL.
const (
	ErrorGroupsPath = "/errors"
	LogsPath        = "/logs"
)

// ResourceRef names the monitor or service monitor a group, event or
// record belongs to.
type ResourceRef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ErrorGroup is one row of GET /api/errors.
type ErrorGroup struct {
	Group         string        `json:"group"`
	Title         string        `json:"title"`
	Culprit       string        `json:"culprit"`
	ExceptionType string        `json:"exception_type"`
	Level         string        `json:"level"`
	Levels        []string      `json:"levels"`
	Environment   string        `json:"environment"`
	Count         int64         `json:"count"`
	FirstSeen     string        `json:"first_seen"`
	LastSeen      string        `json:"last_seen"`
	Monitors      []ResourceRef `json:"monitors"`
	Services      []ResourceRef `json:"services"`
	LastEventID   string        `json:"last_event_id"`
}

// ErrorGroupPage is the GET /api/errors response.
type ErrorGroupPage struct {
	Page       int          `json:"page"`
	PageCount  int          `json:"page_count"`
	PageSize   int          `json:"page_size"`
	TotalCount int          `json:"total_count"`
	Data       []ErrorGroup `json:"data"`
}

// SparklinePoint is one hourly bucket of a group's sparkline.
type SparklinePoint struct {
	Stamp string `json:"stamp"`
	Count int64  `json:"count"`
}

// ErrorGroupDetail is the GET /api/errors/{group} response: the group, 24
// hourly counts (oldest first) and the latest event with its payload.
type ErrorGroupDetail struct {
	ErrorGroup
	Sparkline   []SparklinePoint `json:"sparkline"`
	LatestEvent *ErrorEvent      `json:"latest_event"`
}

// EventUser is the user an error event was reported for.
type EventUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	IP    string `json:"ip"`
}

// EventSDK is the SDK that sent an error event.
type EventSDK struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ErrorEvent is one error event. Payload, the normalized Sentry event, is
// present on GET /api/errors/{group}/events/{event_id} and on a group's
// latest_event.
type ErrorEvent struct {
	EventID        string                 `json:"event_id"`
	Group          string                 `json:"group"`
	Stamp          string                 `json:"stamp"`
	Level          string                 `json:"level"`
	Platform       string                 `json:"platform"`
	Title          string                 `json:"title"`
	Message        string                 `json:"message"`
	ExceptionType  string                 `json:"exception_type"`
	ExceptionValue string                 `json:"exception_value"`
	Culprit        string                 `json:"culprit"`
	Transaction    string                 `json:"transaction"`
	Logger         string                 `json:"logger"`
	ServerName     string                 `json:"server_name"`
	Release        string                 `json:"release"`
	Environment    string                 `json:"environment"`
	Monitor        *ResourceRef           `json:"monitor"`
	Service        *ResourceRef           `json:"service"`
	Series         string                 `json:"series"`
	Handled        *bool                  `json:"handled"`
	Tags           map[string]interface{} `json:"tags"`
	User           *EventUser             `json:"user"`
	SDK            *EventSDK              `json:"sdk"`
	Client         json.RawMessage        `json:"client,omitempty"`
	IP             string                 `json:"ip"`
	Payload        json.RawMessage        `json:"payload,omitempty"`
}

// ErrorEventPage is the GET /api/errors/{group}/events response.
type ErrorEventPage struct {
	Data    []ErrorEvent `json:"data"`
	HasMore bool         `json:"has_more"`
	Next    Cursor       `json:"next"`
}

// LogEntry is one log record from GET /api/logs.
type LogEntry struct {
	Stamp          string                 `json:"stamp"`
	Level          string                 `json:"level"`
	SeverityNumber int                    `json:"severity_number"`
	Body           string                 `json:"body"`
	Template       string                 `json:"template"`
	Logger         string                 `json:"logger"`
	ServerName     string                 `json:"server_name"`
	Release        string                 `json:"release"`
	Environment    string                 `json:"environment"`
	Monitor        *ResourceRef           `json:"monitor"`
	Service        *ResourceRef           `json:"service"`
	Series         string                 `json:"series"`
	TraceID        string                 `json:"trace_id"`
	SpanID         string                 `json:"span_id"`
	Attributes     map[string]interface{} `json:"attributes"`
	SDK            json.RawMessage        `json:"sdk,omitempty"`
	Client         json.RawMessage        `json:"client,omitempty"`
}

// LogPage is the GET /api/logs response, newest record first.
type LogPage struct {
	Data    []LogEntry `json:"data"`
	HasMore bool       `json:"has_more"`
	Next    Cursor     `json:"next"`
}

// Cursor is the "next" value of a cursor-paged response, passed back as
// before=. The API sends a string, a number of unix seconds, or null.
type Cursor string

func (c *Cursor) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*c = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*c = Cursor(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("cursor must be a string, number or null: %s", b)
	}
	*c = Cursor(n.String())
	return nil
}

// ErrorGroupQuery holds the filters of GET /api/errors.
type ErrorGroupQuery struct {
	Env      string
	Monitor  string // job key or code
	Service  string // service monitor key or code
	Levels   []string
	Start    string // unix seconds or ISO 8601
	End      string
	Search   string // matches title and culprit
	OrderBy  string
	Page     int
	PageSize int
}

// Params returns the query string parameters. Empty filters are left out.
func (q ErrorGroupQuery) Params() map[string]string {
	p := map[string]string{}
	setParam(p, "env", q.Env)
	setParam(p, "monitor", q.Monitor)
	setParam(p, "service", q.Service)
	var levels []string
	for _, l := range q.Levels {
		for _, part := range strings.Split(l, ",") {
			if part = NormalizeErrorLevel(part); part != "" {
				levels = append(levels, part)
			}
		}
	}
	setParam(p, "level", strings.Join(levels, ","))
	setParam(p, "start", q.Start)
	setParam(p, "end", q.End)
	setParam(p, "q", q.Search)
	setParam(p, "orderBy", q.OrderBy)
	if q.Page > 1 {
		p["page"] = strconv.Itoa(q.Page)
	}
	if q.PageSize > 0 {
		p["pageSize"] = strconv.Itoa(q.PageSize)
	}
	return p
}

// ErrorEventQuery holds the filters of GET /api/errors/{group}/events.
type ErrorEventQuery struct {
	Env     string
	Monitor string
	Service string
	Series  string
	Before  string // cursor: unix seconds or ISO time
	Limit   int
}

// Params returns the query string parameters. Empty filters are left out.
func (q ErrorEventQuery) Params() map[string]string {
	p := map[string]string{}
	setParam(p, "env", q.Env)
	setParam(p, "monitor", q.Monitor)
	setParam(p, "service", q.Service)
	setParam(p, "series", q.Series)
	setParam(p, "before", q.Before)
	if q.Limit > 0 {
		p["limit"] = strconv.Itoa(q.Limit)
	}
	return p
}

// LogQuery holds the filters of GET /api/logs.
type LogQuery struct {
	Env     string
	Monitor string
	Service string
	Series  string
	Level   string // minimum level
	Start   string
	End     string
	Search  string // whole tokens, all must match
	TraceID string
	Before  string
	Limit   int
}

// Params returns the query string parameters. Empty filters are left out.
func (q LogQuery) Params() map[string]string {
	p := map[string]string{}
	setParam(p, "env", q.Env)
	setParam(p, "monitor", q.Monitor)
	setParam(p, "service", q.Service)
	setParam(p, "series", q.Series)
	setParam(p, "level", NormalizeLogLevel(q.Level))
	setParam(p, "start", q.Start)
	setParam(p, "end", q.End)
	setParam(p, "q", q.Search)
	setParam(p, "trace_id", q.TraceID)
	setParam(p, "before", q.Before)
	if q.Limit > 0 {
		p["limit"] = strconv.Itoa(q.Limit)
	}
	return p
}

func setParam(p map[string]string, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		p[key] = value
	}
}

// NormalizeErrorLevel lower-cases an error level and accepts "warn" for
// "warning", the spelling error events use.
func NormalizeErrorLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "warn" {
		return "warning"
	}
	return level
}

// NormalizeLogLevel lower-cases a log level and accepts "warning" for
// "warn", the spelling log records use.
func NormalizeLogLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "warning" {
		return "warn"
	}
	return level
}

// ErrorGroupPath is the path of one error group.
func ErrorGroupPath(group string) string {
	return ErrorGroupsPath + "/" + url.PathEscape(group)
}

// ErrorEventsPath is the path of a group's event list.
func ErrorEventsPath(group string) string {
	return ErrorGroupPath(group) + "/events"
}

// ErrorEventPath is the path of one event in a group.
func ErrorEventPath(group, eventID string) string {
	return ErrorEventsPath(group) + "/" + url.PathEscape(eventID)
}

var (
	errorGroupIDPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	errorEventIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// NormalizeErrorGroupID lower-cases a group id and checks that it is the
// full 40-character hex id. The API does not resolve prefixes.
func NormalizeErrorGroupID(id string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(id))
	if !errorGroupIDPattern.MatchString(norm) {
		return "", fmt.Errorf("error group id must be the full 40-character hex id, got %q", id)
	}
	return norm, nil
}

// NormalizeErrorEventID lower-cases an event id, drops UUID dashes, and
// checks that 32 hex characters remain.
func NormalizeErrorEventID(id string) (string, error) {
	norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(id), "-", ""))
	if !errorEventIDPattern.MatchString(norm) {
		return "", fmt.Errorf("event id must be 32 hex characters, got %q", id)
	}
	return norm, nil
}

var (
	lookbackFull = regexp.MustCompile(`^(\d+[smhdw])+$`)
	lookbackPart = regexp.MustCompile(`(\d+)([smhdw])`)
	lookbackUnit = map[string]time.Duration{
		"s": time.Second,
		"m": time.Minute,
		"h": time.Hour,
		"d": 24 * time.Hour,
		"w": 7 * 24 * time.Hour,
	}
)

// ParseLookback parses a positive duration such as 30m, 24h, 7d, 2w or
// 1d12h. Units are s, m, h, d (24 hours) and w (7 days).
func ParseLookback(s string) (time.Duration, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !lookbackFull.MatchString(s) {
		return 0, fmt.Errorf("invalid duration %q: use a number and unit such as 30m, 24h, 7d or 2w", s)
	}
	var total time.Duration
	for _, m := range lookbackPart.FindAllStringSubmatch(s, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %v", s, err)
		}
		total += time.Duration(n) * lookbackUnit[m[2]]
	}
	if total <= 0 {
		return 0, fmt.Errorf("invalid duration %q: must be greater than zero", s)
	}
	return total, nil
}

// LookbackStart returns now minus the lookback as unix seconds, the form
// the read API accepts for start=.
func LookbackStart(lookback string, now time.Time) (string, error) {
	d, err := ParseLookback(lookback)
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(now.Add(-d).Unix(), 10), nil
}
