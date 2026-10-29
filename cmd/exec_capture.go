package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// Error and log capture for `cronitor exec --capture-errors / --ship-logs`.
// Both send Sentry-format envelopes to {ping host}/api/envelope. Nothing
// here may change the wrapped command's exit code or block its output.

var captureErrors bool
var shipLogs bool

const (
	captureReadBytes    = 256 * 1024 // read at most this much of the output to build an event
	captureTailLines    = 100
	captureTailBytes    = 16 * 1024
	maxTagValue         = 200
	logBatchLines       = 100
	logBatchInterval    = time.Second
	logDrainTimeout     = 10 * time.Second
	maxLogLineBytes     = 64 * 1024 // a longer run without a newline is shipped as one line
	maxQueuedLogBatches = 100
)

// ingestHost is the host pings go to, so envelopes follow
// CRONITOR_PING_API_HOST / --ping-api-host.
func ingestHost() string {
	if lib.PingHostOverride != "" {
		return lib.PingHostOverride
	}
	if dev {
		return "http://localhost:8000"
	}
	return normalizePingApiHost(viper.GetString(varPingApiHost))
}

// ingestApiKey is the ping API key, falling back to the API key, as for pings.
func ingestApiKey() string {
	if key := viper.GetString(varPingApiKey); key != "" {
		return key
	}
	return viper.GetString(varApiKey)
}

func sendEnvelope(env *lib.Envelope, apiKey string) error {
	return lib.SendEnvelope(ingestHost(), apiKey, userAgent, env)
}

// readOutputTail returns up to the last max bytes of the captured output,
// starting at a line boundary. It uses ReadAt so it does not disturb the
// file offset used by the log upload running alongside it.
func readOutputTail(tempFile *os.File, max int64) string {
	if tempFile == nil {
		return ""
	}
	size, err := getFileSize(tempFile)
	if err != nil || size == 0 {
		return ""
	}
	offset := int64(0)
	if size > max {
		offset = size - max
	}
	buf := make([]byte, size-offset)
	n, _ := tempFile.ReadAt(buf, offset)
	buf = buf[:n]
	if offset > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	return string(buf)
}

// buildFailureEvent turns a failed run into an exception event: the last
// Python or Node traceback in the output when there is one, otherwise
// ExitStatus with the last non-empty output line.
func buildFailureEvent(output, command, series string, exitCode int, runErr error, endTime float64) *lib.Event {
	var ev *lib.Event
	if parsed, ok := lib.ParseTraceback(output); ok {
		ev = lib.NewExceptionEvent(parsed.Type, parsed.Value, parsed.Platform, parsed.Frames, Version)
	} else {
		value := lastNonEmptyLine(output)
		if value == "" && runErr != nil {
			value = runErr.Error()
		}
		if value == "" {
			value = fmt.Sprintf("exit status %d", exitCode)
		}
		ev = lib.NewExceptionEvent("ExitStatus", value, "other", nil, Version)
	}

	host := effectiveHostname()
	sec := int64(endTime)
	ev.Timestamp = lib.FormatIngestTime(time.Unix(sec, int64((endTime-float64(sec))*1e9)))
	ev.ServerName = host
	ev.Environment = viper.GetString(varEnv)
	ev.Tags = map[string]string{
		"cronitor.monitor": monitorCode,
		"cronitor.series":  series,
		"exit_code":        strconv.Itoa(exitCode),
		"command":          lib.TruncateChars(command, maxTagValue),
		"host":             lib.TruncateChars(host, maxTagValue),
	}
	if tail := outputTail(output); tail != "" {
		ev.Extra = map[string]interface{}{"output_tail": tail}
	}
	return ev
}

func outputTail(output string) string {
	output = strings.TrimRight(output, "\r\n")
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) > captureTailLines {
		lines = lines[len(lines)-captureTailLines:]
	}
	return lib.TailBytes(strings.Join(lines, "\n"), captureTailBytes)
}

func lastNonEmptyLine(output string) string {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

func sendFailureEvent(ev *lib.Event, apiKey string, wg *sync.WaitGroup) {
	defer wg.Done()
	env, err := lib.NewEventEnvelope(ev)
	if err != nil {
		log(err.Error())
		return
	}
	log(fmt.Sprintf("Sending exception event %s to %s/api/envelope", ev.EventID, ingestHost()))
	if err := sendEnvelope(env, apiKey); err != nil {
		log(err.Error())
	}
}

// logShipper is an io.Writer that splits command output into lines and
// ships them as log records in batches of logBatchLines, or every
// logBatchInterval. Write never blocks on the network: batches queue for a
// single sender goroutine and are dropped if the queue is full.
type logShipper struct {
	mu        sync.Mutex
	partial   []byte
	pending   []lib.LogRecord
	lineNo    int
	lastStamp time.Time // every record gets a later timestamp than the one before, so cursor paging never skips a line
	attrs     map[string]lib.LogAttribute
	closed    bool
	dropped   int
	failed    int
	batches   chan []lib.LogRecord
	stopTimer chan struct{}
	done      chan struct{}
	send      func([]lib.LogRecord) error
}

func newLogShipper(send func([]lib.LogRecord) error, attrs map[string]lib.LogAttribute) *logShipper {
	s := &logShipper{
		attrs:     attrs,
		batches:   make(chan []lib.LogRecord, maxQueuedLogBatches),
		stopTimer: make(chan struct{}),
		done:      make(chan struct{}),
		send:      send,
	}
	go s.flushEveryInterval()
	go s.sendBatches()
	return s
}

// newExecLogShipper ships to the ingest host, bound to the current run.
func newExecLogShipper(apiKey, series string) *logShipper {
	attrs := map[string]lib.LogAttribute{
		"cronitor.monitor": lib.StringAttribute(monitorCode),
		"cronitor.series":  lib.StringAttribute(series),
		"server.address":   lib.StringAttribute(effectiveHostname()),
	}
	if env := viper.GetString(varEnv); env != "" {
		attrs["sentry.environment"] = lib.StringAttribute(env)
	}
	return newLogShipper(func(batch []lib.LogRecord) error {
		env, err := lib.NewLogEnvelope(batch, Version)
		if err != nil {
			return err
		}
		return sendEnvelope(env, apiKey)
	}, attrs)
}

func (s *logShipper) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return len(p), nil
	}
	now := time.Now()
	s.partial = append(s.partial, p...)
	for {
		i := bytes.IndexByte(s.partial, '\n')
		if i < 0 {
			break
		}
		s.addLineLocked(string(s.partial[:i]), now)
		s.partial = s.partial[i+1:]
	}
	if len(s.partial) > maxLogLineBytes {
		s.addLineLocked(string(s.partial), now)
		s.partial = nil
	}
	return len(p), nil
}

// addLineLocked numbers every line, including blank ones, so the "line"
// attribute matches the output; blank lines are not shipped.
func (s *logShipper) addLineLocked(line string, now time.Time) {
	line = strings.TrimRight(line, "\r")
	s.lineNo++
	if strings.TrimSpace(line) == "" {
		return
	}
	if !now.After(s.lastStamp) {
		now = s.lastStamp.Add(time.Microsecond)
	}
	s.lastStamp = now
	attrs := make(map[string]lib.LogAttribute, len(s.attrs)+1)
	for k, v := range s.attrs {
		attrs[k] = v
	}
	attrs["line"] = lib.IntAttribute(s.lineNo)
	s.pending = append(s.pending, lib.NewLogRecord(line, lib.LogLevelForLine(line), now, Version, attrs))
	if len(s.pending) >= logBatchLines {
		s.flushLocked()
	}
}

func (s *logShipper) flushLocked() {
	if len(s.pending) == 0 {
		return
	}
	batch := s.pending
	s.pending = nil
	select {
	case s.batches <- batch:
	default:
		s.dropped += len(batch)
	}
}

func (s *logShipper) flushEveryInterval() {
	ticker := time.NewTicker(logBatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			if !s.closed {
				s.flushLocked()
			}
			s.mu.Unlock()
		case <-s.stopTimer:
			return
		}
	}
}

func (s *logShipper) sendBatches() {
	defer close(s.done)
	for batch := range s.batches {
		if err := s.send(batch); err != nil {
			s.mu.Lock()
			s.failed++
			first := s.failed == 1
			s.mu.Unlock()
			if first {
				log("Could not ship logs: " + err.Error())
			}
		}
	}
}

// Close flushes the trailing partial line and pending records, then waits
// up to logDrainTimeout for queued batches to send.
func (s *logShipper) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if len(s.partial) > 0 {
		s.addLineLocked(string(s.partial), time.Now())
		s.partial = nil
	}
	s.flushLocked()
	s.closed = true
	close(s.stopTimer)
	close(s.batches)
	s.mu.Unlock()

	select {
	case <-s.done:
	case <-time.After(logDrainTimeout):
		log("Stopped waiting for log batches to send")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dropped > 0 {
		log(fmt.Sprintf("Dropped %d log lines because the send queue was full", s.dropped))
	}
	if s.failed > 1 {
		log(fmt.Sprintf("%d log batches could not be sent", s.failed))
	}
}
