package lib

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedError is an exception recognized in command output.
type ParsedError struct {
	Type     string
	Value    string
	Platform string  // "python" or "node"
	Frames   []Frame // oldest first, crash site last
}

var (
	pyHeaderPattern    = regexp.MustCompile(`Traceback \(most recent call last\):\s*$`)
	pyFramePattern     = regexp.MustCompile(`^\s+File "([^"]+)", line (\d+)(?:, in (.+?))?\s*$`)
	pyRepeatedPattern  = regexp.MustCompile(`^\s+\[Previous line repeated \d+ more times?\]\s*$`)
	pyExceptionPattern = regexp.MustCompile(`^([A-Za-z_][\w.]*)(?::\s?(.*))?$`)
	pyLibraryPattern   = regexp.MustCompile(`(?i)(site-packages|dist-packages|^<frozen |[/\\]lib[/\\]python\d+(\.\d+)?[/\\]|[/\\]python\d+[^/\\]*[/\\]lib[/\\])`)

	nodeFramePattern     = regexp.MustCompile(`^\s+at (?:async )?(.+?) \((.+):(\d+):(\d+)\)\s*\{?\s*$`)
	nodeBareFramePattern = regexp.MustCompile(`^\s+at (?:async )?(.+):(\d+):(\d+)\s*\{?\s*$`)
	nodeAnyFramePattern  = regexp.MustCompile(`^\s+at \S`)
	nodeHeaderPattern    = regexp.MustCompile(`^(?:Uncaught\s+)?([A-Za-z_$][\w$.]*(?: \[[A-Z0-9_]+\])?): (.*)$`)
)

// nodeHeaderLookback is how many lines above the first "at" line the
// "<Type>: <message>" line may be, to allow multi-line messages.
const nodeHeaderLookback = 5

// ParseTraceback finds the last Python traceback or Node.js stack trace in
// output. It returns ok=false when neither is recognized.
func ParseTraceback(output string) (*ParsedError, bool) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\r")
	}

	py, pyEnd := parsePythonTraceback(lines)
	node, nodeEnd := parseNodeStack(lines)
	switch {
	case py != nil && (node == nil || pyEnd > nodeEnd):
		return py, true
	case node != nil:
		return node, true
	}
	return nil, false
}

// parsePythonTraceback returns the last complete traceback block and the
// index of its exception line.
//
// The last one is taken because the uncaught exception that ended the
// process is printed as the interpreter exits, after any traceback the
// program logged and survived. It need not be the end of the output:
// stdout to a pipe or file is block-buffered and flushes at exit, after the
// traceback went to unbuffered stderr, so the search runs back from the end
// rather than looking only at the tail. The same buffering means a
// traceback the program printed to stdout itself can land after the fatal
// one and be taken instead; there is no telling them apart from the text.
func parsePythonTraceback(lines []string) (*ParsedError, int) {
	for h := len(lines) - 1; h >= 0; h-- {
		if !pyHeaderPattern.MatchString(lines[h]) {
			continue
		}
		if parsed, end, ok := parsePythonBlock(lines, h); ok {
			return parsed, end
		}
	}
	return nil, -1
}

func parsePythonBlock(lines []string, header int) (*ParsedError, int, bool) {
	// Text before "Traceback" on the header line is either noise on that
	// line only (a "^C") or a prefix on every line (a timestamp or a
	// container name). The first frame line decides which.
	strip := 0
	if prefix := pyHeaderPattern.FindStringIndex(lines[header])[0]; prefix > 0 && header+1 < len(lines) {
		next := lines[header+1]
		if !pyFramePattern.MatchString(next) && len(next) > prefix && pyFramePattern.MatchString(next[prefix:]) {
			strip = prefix
		}
	}

	var frames []Frame
	codePending := false
	for j := header + 1; j < len(lines); j++ {
		line := lines[j]
		if strip > 0 {
			if len(line) <= strip {
				return nil, j, false
			}
			line = line[strip:]
		}

		if m := pyFramePattern.FindStringSubmatch(line); m != nil {
			lineno, _ := strconv.Atoi(m[2])
			frames = append(frames, Frame{
				Filename: m[1],
				Function: m[3],
				Lineno:   lineno,
				InApp:    !pyLibraryPattern.MatchString(m[1]),
			})
			codePending = true
			continue
		}
		if pyRepeatedPattern.MatchString(line) {
			codePending = false
			continue
		}
		if isIndented(line) && strings.TrimSpace(line) != "" {
			// The first indented line after a frame is its source line;
			// later ones are caret markers or continuations.
			if codePending {
				frames[len(frames)-1].ContextLine = strings.TrimSpace(line)
				codePending = false
			}
			continue
		}
		if m := pyExceptionPattern.FindStringSubmatch(strings.TrimSpace(line)); m != nil && len(frames) > 0 && !isIndented(line) {
			return &ParsedError{
				Type:     m[1],
				Value:    strings.TrimSpace(m[2]),
				Platform: "python",
				Frames:   frames,
			}, j, true
		}
		return nil, j, false
	}
	return nil, len(lines), false
}

// parseNodeStack returns the last stack trace and the index of its last
// "at" line.
func parseNodeStack(lines []string) (*ParsedError, int) {
	end := len(lines) - 1
	for end >= 0 {
		if !nodeAnyFramePattern.MatchString(lines[end]) {
			end--
			continue
		}
		start := end
		for start > 0 && nodeAnyFramePattern.MatchString(lines[start-1]) {
			start--
		}
		if parsed, ok := parseNodeBlock(lines, start, end); ok {
			return parsed, end
		}
		end = start - 1
	}
	return nil, -1
}

func parseNodeBlock(lines []string, start, end int) (*ParsedError, bool) {
	excType, value := "", ""
	found := false
	for h := start - 1; h >= 0 && h >= start-nodeHeaderLookback; h-- {
		if nodeAnyFramePattern.MatchString(lines[h]) {
			break
		}
		if m := nodeHeaderPattern.FindStringSubmatch(lines[h]); m != nil {
			excType = m[1]
			parts := append([]string{m[2]}, lines[h+1:start]...)
			value = strings.TrimSpace(strings.Join(parts, "\n"))
			found = true
			break
		}
	}
	if !found {
		return nil, false
	}

	// Node prints the crash site first; Sentry wants it last.
	var frames []Frame
	for i := end; i >= start; i-- {
		var frame Frame
		if m := nodeFramePattern.FindStringSubmatch(lines[i]); m != nil {
			frame = Frame{Function: m[1], Filename: m[2], Lineno: atoi(m[3]), Colno: atoi(m[4])}
		} else if m := nodeBareFramePattern.FindStringSubmatch(lines[i]); m != nil {
			frame = Frame{Filename: m[1], Lineno: atoi(m[2]), Colno: atoi(m[3])}
		} else {
			continue // e.g. "at Array.forEach (<anonymous>)"
		}
		if !isNodeFramePath(frame.Filename) {
			continue // e.g. "at 2026-10-29 10:30:45" in a log line
		}
		frame.InApp = isNodeAppFile(frame.Filename)
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		return nil, false // a header needs at least one real frame
	}
	return &ParsedError{Type: excType, Value: value, Platform: "node", Frames: frames}, true
}

// isNodeFramePath reports whether a frame's file looks like what Node
// prints: a path, a node: or file: URL, or <anonymous>.
func isNodeFramePath(filename string) bool {
	return strings.ContainsAny(filename, `/\`) ||
		strings.HasPrefix(filename, "node:") ||
		strings.HasPrefix(filename, "file:") ||
		strings.HasPrefix(filename, "<anonymous>")
}

func isNodeAppFile(filename string) bool {
	return !strings.HasPrefix(filename, "node:") &&
		!strings.HasPrefix(filename, "internal/") &&
		!strings.Contains(filename, "node_modules")
}

func isIndented(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
