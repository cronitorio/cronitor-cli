package lib

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// This file builds and sends Sentry-format envelopes to the Cronitor
// telemetry host (POST {host}/api/envelope). It has no dependencies beyond
// the standard library and no platform-specific code.

// IngestSDKName identifies this client in envelope and event "sdk" blocks.
const IngestSDKName = "cronitor-cli"

const (
	ingestTimeout       = 10 * time.Second
	maxExceptionType    = 200
	maxExceptionValue   = 4000
	maxEventFrames      = 250
	maxFrameContextLine = 1000
	maxLogBody          = 8000
)

// SDKInfo is the "sdk" block of an envelope header or an event.
type SDKInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// EnvelopeHeader is the first line of an envelope.
type EnvelopeHeader struct {
	EventID string  `json:"event_id,omitempty"`
	SentAt  string  `json:"sent_at"`
	SDK     SDKInfo `json:"sdk"`
}

// Item is one envelope item: a JSON header line followed by its payload.
type Item struct {
	Type        string
	ContentType string
	ItemCount   int // set for log items
	Payload     []byte
}

// Envelope is a Sentry envelope.
type Envelope struct {
	Header EnvelopeHeader
	Items  []Item
}

type itemHeader struct {
	Type        string `json:"type"`
	ItemCount   int    `json:"item_count,omitempty"`
	ContentType string `json:"content_type"`
	Length      int    `json:"length"`
}

// Marshal returns the envelope bytes: the header line, then for each item
// its header line (always with "length") and payload, each ending in "\n".
func (e *Envelope) Marshal() []byte {
	var buf bytes.Buffer
	header := e.Header
	if header.SentAt == "" {
		header.SentAt = FormatIngestTime(time.Now())
	}
	writeJSONLine(&buf, header)
	for _, item := range e.Items {
		writeJSONLine(&buf, itemHeader{
			Type:        item.Type,
			ItemCount:   item.ItemCount,
			ContentType: item.ContentType,
			Length:      len(item.Payload),
		})
		buf.Write(item.Payload)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func writeJSONLine(buf *bytes.Buffer, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte("{}")
	}
	buf.Write(b)
	buf.WriteByte('\n')
}

// Frame is one stack frame. Frames are ordered oldest first, crash site last.
type Frame struct {
	Filename    string `json:"filename,omitempty"`
	Function    string `json:"function,omitempty"`
	Lineno      int    `json:"lineno,omitempty"`
	Colno       int    `json:"colno,omitempty"`
	ContextLine string `json:"context_line,omitempty"`
	InApp       bool   `json:"in_app"`
}

// Stacktrace holds the frames of one exception.
type Stacktrace struct {
	Frames []Frame `json:"frames"`
}

// Mechanism says how an exception was captured.
type Mechanism struct {
	Type    string `json:"type"`
	Handled bool   `json:"handled"`
}

// Exception is one entry of an event's exception.values.
type Exception struct {
	Type       string      `json:"type"`
	Value      string      `json:"value"`
	Mechanism  *Mechanism  `json:"mechanism,omitempty"`
	Stacktrace *Stacktrace `json:"stacktrace,omitempty"`
}

// ExceptionList is an event's "exception" block.
type ExceptionList struct {
	Values []Exception `json:"values"`
}

// Event is a Sentry error event.
type Event struct {
	EventID     string                 `json:"event_id"`
	Timestamp   string                 `json:"timestamp"`
	Platform    string                 `json:"platform"`
	Level       string                 `json:"level"`
	ServerName  string                 `json:"server_name,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Tags        map[string]string      `json:"tags,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	SDK         SDKInfo                `json:"sdk"`
	Exception   *ExceptionList         `json:"exception,omitempty"`
}

// NewEventID returns a random 32 character lowercase hex id.
func NewEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on supported platforms; fall back to the clock.
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// NewExceptionEvent returns an unhandled error-level event with one exception.
// The stacktrace is omitted when frames is empty. When there are more than
// maxEventFrames frames, the ones nearest the crash site are kept.
func NewExceptionEvent(excType, excValue, platform string, frames []Frame, sdkVersion string) *Event {
	if platform == "" {
		platform = "other"
	}
	exc := Exception{
		Type:      TruncateChars(excType, maxExceptionType),
		Value:     TruncateChars(excValue, maxExceptionValue),
		Mechanism: &Mechanism{Type: "cronitor.exec", Handled: false},
	}
	if len(frames) > maxEventFrames {
		frames = frames[len(frames)-maxEventFrames:]
	}
	if len(frames) > 0 {
		cleaned := make([]Frame, len(frames))
		for i, f := range frames {
			f.ContextLine = TruncateChars(f.ContextLine, maxFrameContextLine)
			cleaned[i] = f
		}
		exc.Stacktrace = &Stacktrace{Frames: cleaned}
	}
	return &Event{
		EventID:   NewEventID(),
		Timestamp: FormatIngestTime(time.Now()),
		Platform:  platform,
		Level:     "error",
		SDK:       SDKInfo{Name: IngestSDKName, Version: sdkVersion},
		Exception: &ExceptionList{Values: []Exception{exc}},
	}
}

// NewEventEnvelope wraps one event in an envelope.
func NewEventEnvelope(ev *Event) (*Envelope, error) {
	payload, err := json.Marshal(ev)
	if err != nil {
		return nil, fmt.Errorf("encode event: %w", err)
	}
	return &Envelope{
		Header: EnvelopeHeader{
			EventID: ev.EventID,
			SentAt:  FormatIngestTime(time.Now()),
			SDK:     ev.SDK,
		},
		Items: []Item{{Type: "event", ContentType: "application/json", Payload: payload}},
	}, nil
}

// LogAttribute is a typed log attribute, encoded as {"value": ..., "type": ...}.
type LogAttribute struct {
	Value interface{} `json:"value"`
	Type  string      `json:"type"`
}

// StringAttribute returns a string-typed log attribute.
func StringAttribute(v string) LogAttribute {
	return LogAttribute{Value: v, Type: "string"}
}

// IntAttribute returns an integer-typed log attribute.
func IntAttribute(v int) LogAttribute {
	return LogAttribute{Value: v, Type: "integer"}
}

// LogRecord is one structured log record in a log item.
type LogRecord struct {
	Timestamp  float64                 `json:"timestamp"`
	Level      string                  `json:"level"`
	Body       string                  `json:"body"`
	Attributes map[string]LogAttribute `json:"attributes"`
}

var logSeverityNumbers = map[string]int{
	"trace": 1,
	"debug": 5,
	"info":  9,
	"warn":  13,
	"error": 17,
	"fatal": 21,
}

var (
	errorLinePattern = regexp.MustCompile(`(?i)\b(error|exception|traceback|fatal|panic)\b`)
	warnLinePattern  = regexp.MustCompile(`(?i)\bwarn(ing)?\b`)
)

// LogLevelForLine classifies a line of command output as error, warn or info.
func LogLevelForLine(line string) string {
	if errorLinePattern.MatchString(line) {
		return "error"
	}
	if warnLinePattern.MatchString(line) {
		return "warn"
	}
	return "info"
}

// NewLogRecord returns a log record with the given attributes plus the
// severity and sdk attributes. The body is truncated to 8,000 characters.
func NewLogRecord(body, level string, ts time.Time, sdkVersion string, attrs map[string]LogAttribute) LogRecord {
	severity, ok := logSeverityNumbers[level]
	if !ok {
		level, severity = "info", logSeverityNumbers["info"]
	}
	all := make(map[string]LogAttribute, len(attrs)+4)
	for k, v := range attrs {
		all[k] = v
	}
	all["sentry.severity_number"] = IntAttribute(severity)
	all["sentry.severity_text"] = StringAttribute(level)
	all["sentry.sdk.name"] = StringAttribute(IngestSDKName)
	all["sentry.sdk.version"] = StringAttribute(sdkVersion)
	return LogRecord{
		Timestamp:  float64(ts.Unix()) + float64(ts.Nanosecond())/1e9,
		Level:      level,
		Body:       TruncateChars(body, maxLogBody),
		Attributes: all,
	}
}

// NewLogEnvelope wraps log records in an envelope with one log item.
func NewLogEnvelope(records []LogRecord, sdkVersion string) (*Envelope, error) {
	payload, err := json.Marshal(struct {
		Items []LogRecord `json:"items"`
	}{Items: records})
	if err != nil {
		return nil, fmt.Errorf("encode log records: %w", err)
	}
	return &Envelope{
		Header: EnvelopeHeader{
			EventID: NewEventID(),
			SentAt:  FormatIngestTime(time.Now()),
			SDK:     SDKInfo{Name: IngestSDKName, Version: sdkVersion},
		},
		Items: []Item{{
			Type:        "log",
			ContentType: "application/vnd.sentry.items.log+json",
			ItemCount:   len(records),
			Payload:     payload,
		}},
	}, nil
}

// SendEnvelope gzips the envelope and POSTs it to {host}/api/envelope.
// userAgent is sent as the User-Agent and as sentry_client. Any 2xx is
// success. A network error is retried once; the whole call is capped at
// 10 seconds. A non-2xx response is not retried and its body is returned in
// the error so the caller can log it.
func SendEnvelope(host, apiKey, userAgent string, env *Envelope) error {
	if env == nil {
		return fmt.Errorf("no envelope to send")
	}
	body, err := gzipBytes(env.Marshal())
	if err != nil {
		return fmt.Errorf("compress envelope: %w", err)
	}

	endpoint := strings.TrimRight(host, "/") + "/api/envelope"
	auth := fmt.Sprintf("Sentry sentry_key=%s, sentry_version=7, sentry_client=%s", apiKey, userAgent)

	ctx, cancel := context.WithTimeout(context.Background(), ingestTimeout)
	defer cancel()
	client := &http.Client{Timeout: ingestTimeout}

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build envelope request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-sentry-envelope")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("X-Sentry-Auth", auth)
		req.Header.Set("User-Agent", userAgent)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("envelope rejected by %s: HTTP %d: %s", endpoint, resp.StatusCode,
			TruncateChars(strings.TrimSpace(string(respBody)), 500))
	}
	return fmt.Errorf("envelope not sent: %w", lastErr)
}

func gzipBytes(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FormatIngestTime formats t as RFC 3339 in UTC with microseconds, the
// format used for envelope and event timestamps.
func FormatIngestTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000Z07:00")
}

// TruncateChars truncates s to at most n characters (runes), never
// splitting a multi-byte character.
func TruncateChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

// TailBytes returns the last n bytes of s, starting on a character boundary.
func TailBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	for i := 0; i < len(s) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(s[i]) {
			return s[i:]
		}
	}
	return s
}
