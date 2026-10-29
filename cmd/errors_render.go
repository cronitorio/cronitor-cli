package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
)

// Rendering for cronitor errors get and cronitor errors events.

// sentryPayload is the part of a normalized Sentry event that the CLI
// prints. exception, breadcrumbs and tags each come either as a list or
// wrapped in {"values": [...]}; tags can also be an object.
type sentryPayload struct {
	Exception   json.RawMessage `json:"exception"`
	Breadcrumbs json.RawMessage `json:"breadcrumbs"`
	Tags        json.RawMessage `json:"tags"`
}

type sentryException struct {
	Type       string `json:"type"`
	Value      string `json:"value"`
	Module     string `json:"module"`
	Stacktrace *struct {
		Frames []sentryFrame `json:"frames"`
	} `json:"stacktrace"`
}

type sentryFrame struct {
	Filename    string `json:"filename"`
	AbsPath     string `json:"abs_path"`
	Module      string `json:"module"`
	Function    string `json:"function"`
	Lineno      int    `json:"lineno"`
	ContextLine string `json:"context_line"`
	InApp       *bool  `json:"in_app"`
}

// sentryValues decodes a Sentry interface that is either a list or a
// {"values": [...]} object.
func sentryValues[T any](raw json.RawMessage) []T {
	if len(raw) == 0 {
		return nil
	}
	var list []T
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var wrapped struct {
		Values []T `json:"values"`
	}
	if json.Unmarshal(raw, &wrapped) == nil {
		return wrapped.Values
	}
	return nil
}

const fieldLabelWidth = 14

type field struct {
	label string
	value string
}

// writeFields writes "  Label:       value" lines. Empty values are left out
// unless keepEmpty, which prints "-".
func writeFields(sb *strings.Builder, fields []field, keepEmpty bool) {
	for _, f := range fields {
		value := f.value
		if value == "" {
			if !keepEmpty {
				continue
			}
			value = "-"
		}
		sb.WriteString("  " + mutedStyle.Render(fmt.Sprintf("%-*s", fieldLabelWidth, f.label+":")) + value + "\n")
	}
}

func stampWithAgo(stamp string, now time.Time) string {
	if stamp == "" {
		return ""
	}
	return fmt.Sprintf("%s (%s)", displayStamp(stamp), relativeTime(stamp, now))
}

func refKey(ref *lib.ResourceRef) string {
	if ref == nil {
		return ""
	}
	return ref.Key
}

func refKeyList(refs []lib.ResourceRef) string {
	return strings.ReplaceAll(refKeys(refs), ",", ", ")
}

func renderErrorGroupDetail(d lib.ErrorGroupDetail, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(boldStyle.Render(d.Title) + "\n")

	level := styleLevel(d.Level, d.Level)
	if len(d.Levels) > 1 {
		level += mutedStyle.Render(" (seen: " + strings.Join(d.Levels, ", ") + ")")
	}
	fields := []field{
		{"Group", d.Group},
		{"Culprit", d.Culprit},
		{"Type", d.ExceptionType},
		{"Level", level},
		{"Count", fmt.Sprintf("%d", d.Count)},
		{"First seen", stampWithAgo(d.FirstSeen, now)},
		{"Last seen", stampWithAgo(d.LastSeen, now)},
		{"Environment", d.Environment},
		{"Monitors", refKeyList(d.Monitors)},
		{"Services", refKeyList(d.Services)},
	}
	if len(d.Sparkline) > 0 {
		fields = append(fields, field{"Last 24h", renderSparkline(d.Sparkline)})
	}
	writeFields(&sb, fields, true)

	if d.LatestEvent != nil {
		sb.WriteString("\n" + boldStyle.Render("Latest event") + "\n")
		sb.WriteString(renderErrorEvent(*d.LatestEvent, now))
	}
	return sb.String()
}

func renderErrorEventDetail(ev lib.ErrorEvent, now time.Time) string {
	title := ev.Title
	if title == "" {
		title = strings.TrimSpace(ev.ExceptionType + ": " + ev.ExceptionValue)
	}
	return boldStyle.Render(title) + "\n" + renderErrorEvent(ev, now)
}

// renderErrorEvent prints an event's fields, its stack trace, the number of
// breadcrumbs and its tags.
func renderErrorEvent(ev lib.ErrorEvent, now time.Time) string {
	var sb strings.Builder

	handled := ""
	if ev.Handled != nil {
		handled = map[bool]string{true: "yes", false: "no"}[*ev.Handled]
	}
	user := ""
	if ev.User != nil {
		var parts []string
		for _, p := range []string{ev.User.ID, ev.User.Email, ev.User.IP} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		user = strings.Join(parts, " ")
	}
	sdk := ""
	if ev.SDK != nil {
		sdk = strings.TrimSpace(ev.SDK.Name + " " + ev.SDK.Version)
	}
	message := ""
	if ev.Message != "" && ev.Message != ev.ExceptionValue {
		message = ev.Message
	}
	writeFields(&sb, []field{
		{"Event", ev.EventID},
		{"Stamp", stampWithAgo(ev.Stamp, now)},
		{"Level", styleLevel(ev.Level, ev.Level)},
		{"Message", message},
		{"Monitor", refKey(ev.Monitor)},
		{"Service", refKey(ev.Service)},
		{"Series", ev.Series},
		{"Environment", ev.Environment},
		{"Release", ev.Release},
		{"Server", ev.ServerName},
		{"Transaction", ev.Transaction},
		{"Logger", ev.Logger},
		{"Handled", handled},
		{"User", user},
		{"SDK", sdk},
	}, false)

	var payload sentryPayload
	hasPayload := len(ev.Payload) > 0 && json.Unmarshal(ev.Payload, &payload) == nil

	sb.WriteString("\n")
	sb.WriteString(renderTraceback(ev, sentryValues[sentryException](payload.Exception)))

	if hasPayload {
		sb.WriteString(fmt.Sprintf("\nBreadcrumbs: %d\n", len(sentryValues[json.RawMessage](payload.Breadcrumbs))))
	}
	tags := eventTags(ev.Tags, payload.Tags)
	if len(tags) == 0 {
		if hasPayload || ev.Tags != nil {
			sb.WriteString("Tags: none\n")
		}
	} else {
		sb.WriteString("Tags: " + strings.Join(tags, ", ") + "\n")
	}
	return sb.String()
}

// renderTraceback prints each exception like a Python traceback, frames
// oldest first. Chained exceptions print in the order Sentry stores them,
// the one that was raised last at the end.
func renderTraceback(ev lib.ErrorEvent, exceptions []sentryException) string {
	if len(exceptions) == 0 {
		if ev.ExceptionType == "" && ev.ExceptionValue == "" {
			text := ev.Message
			if text == "" {
				text = ev.Title
			}
			return text + "\n" + mutedStyle.Render("No stack trace") + "\n"
		}
		exceptions = []sentryException{{Type: ev.ExceptionType, Value: ev.ExceptionValue}}
	}

	var sb strings.Builder
	anyFrames := false
	for i, exc := range exceptions {
		if i > 0 {
			sb.WriteString("\n" + mutedStyle.Render("The above exception was followed by:") + "\n\n")
		}
		if exc.Stacktrace != nil && len(exc.Stacktrace.Frames) > 0 {
			anyFrames = true
			sb.WriteString("Traceback (most recent call last):\n")
			for _, fr := range exc.Stacktrace.Frames {
				sb.WriteString(renderFrame(fr))
			}
		}
		sb.WriteString(exceptionLine(exc) + "\n")
	}
	if !anyFrames {
		sb.WriteString(mutedStyle.Render("No stack trace") + "\n")
	}
	return sb.String()
}

func exceptionLine(exc sentryException) string {
	name := exc.Type
	if exc.Module != "" && exc.Module != "builtins" && exc.Module != "__builtin__" && exc.Module != "exceptions" {
		name = exc.Module + "." + name
	}
	if name == "" {
		name = "Error"
	}
	if exc.Value == "" {
		return errorStyle.Render(name)
	}
	return errorStyle.Render(name+":") + " " + exc.Value
}

func renderFrame(fr sentryFrame) string {
	file := fr.Filename
	if file == "" {
		file = fr.AbsPath
	}
	if file == "" {
		file = fr.Module
	}
	if file == "" {
		file = "<unknown>"
	}
	location := fmt.Sprintf("File %q", file)
	if fr.Lineno > 0 {
		location += fmt.Sprintf(", line %d", fr.Lineno)
	}
	function := fr.Function
	if function == "" {
		function = "?"
	}
	location += ", in " + function

	library := fr.InApp != nil && !*fr.InApp
	var sb strings.Builder
	if library {
		sb.WriteString("  " + mutedStyle.Render("[lib] "+location) + "\n")
	} else {
		sb.WriteString("  " + location + "\n")
	}
	if line := strings.TrimSpace(fr.ContextLine); line != "" {
		if library {
			line = mutedStyle.Render(line)
		}
		sb.WriteString("    " + line + "\n")
	}
	return sb.String()
}

// eventTags returns "key=value" pairs sorted by key: the event's tags, or
// the payload's when the event has none.
func eventTags(tags map[string]interface{}, payloadTags json.RawMessage) []string {
	values := map[string]string{}
	for k, v := range tags {
		values[k] = tagValue(v)
	}
	if len(values) == 0 && len(payloadTags) > 0 {
		var obj map[string]interface{}
		var pairs [][]interface{}
		if json.Unmarshal(payloadTags, &obj) == nil {
			for k, v := range obj {
				values[k] = tagValue(v)
			}
		} else if json.Unmarshal(payloadTags, &pairs) == nil {
			for _, p := range pairs {
				if len(p) == 2 {
					values[tagValue(p[0])] = tagValue(p[1])
				}
			}
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+values[k])
	}
	return out
}

func tagValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// renderSparkline draws one block per hour, oldest first. Hours with no
// events use the lowest block; any event lifts an hour above it.
func renderSparkline(points []lib.SparklinePoint) string {
	var peak, total int64
	for _, p := range points {
		total += p.Count
		if p.Count > peak {
			peak = p.Count
		}
	}
	var sb strings.Builder
	top := len(sparkBlocks) - 1
	for _, p := range points {
		idx := 0
		if p.Count > 0 && peak > 0 {
			idx = int(math.Ceil(float64(p.Count) * float64(top) / float64(peak)))
			if idx < 1 {
				idx = 1
			}
			if idx > top {
				idx = top
			}
		}
		sb.WriteRune(sparkBlocks[idx])
	}
	noun := "events"
	if total == 1 {
		noun = "event"
	}
	return fmt.Sprintf("%s  %d %s, peak %d/h", sb.String(), total, noun, peak)
}

func renderErrorEventTable(page lib.ErrorEventPage) string {
	if len(page.Data) == 0 {
		return mutedStyle.Render("No events found")
	}
	table := &UITable{
		Headers: []string{"STAMP", "LEVEL", "EVENT ID", "MONITOR", "SERIES", "TITLE"},
	}
	for _, ev := range page.Data {
		source := refKey(ev.Monitor)
		if source == "" {
			source = refKey(ev.Service)
		}
		table.Rows = append(table.Rows, []string{
			displayStamp(ev.Stamp),
			styleLevel(ev.Level, ev.Level),
			ev.EventID,
			dashIfEmpty(source),
			dashIfEmpty(ev.Series),
			truncateRunes(ev.Title, 40),
		})
	}
	output := table.Render()
	if page.HasMore && page.Next != "" {
		output += mutedStyle.Render(fmt.Sprintf("\nnext: %s  (pass it to --before for older events)", page.Next))
	}
	return output
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
