package lib

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Error events and log records are written by whoever holds an account's
// telemetry key, which is not secret: it is in every ping URL. Their text
// is stripped of terminal controls before the CLI prints it or the MCP
// server hands it to an agent.

// StripControl removes terminal escape sequences (CSI, OSC, DCS and the
// other ESC- and C1-introduced sequences) and every other C0 and C1
// control character from s, keeping tabs. Invalid UTF-8 becomes U+FFFD.
func StripControl(s string) string {
	return stripControl(s, false)
}

// StripControlMultiline is StripControl that keeps line feeds, for text
// that may span lines. A CRLF becomes a LF.
func StripControlMultiline(s string) string {
	return stripControl(s, true)
}

func stripControl(s string, keepNewlines bool) string {
	if isClean(s, keepNewlines) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		switch {
		case r == '\x1b':
			i += escapeLen(s[i:])
		case r == '\u009b': // CSI
			i += csiLen(s[i:])
		case r == '\u0090', r == '\u0098', r == '\u009d', r == '\u009e', r == '\u009f': // DCS, SOS, OSC, PM, APC
			i += controlStringLen(s[i:])
		case r == '\t', r == '\n' && keepNewlines:
			b.WriteRune(r)
		case unicode.IsControl(r):
		case r == utf8.RuneError && size == 1:
			b.WriteRune(utf8.RuneError)
		default:
			b.WriteString(s[i-size : i])
		}
	}
	return b.String()
}

func isClean(s string, keepNewlines bool) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= utf8.RuneSelf {
			// Non-ASCII: C1 controls and invalid UTF-8 need the slow path.
			return !strings.ContainsFunc(s[i:], func(r rune) bool {
				return r == utf8.RuneError || unicode.IsControl(r) && r != '\t' && !(r == '\n' && keepNewlines)
			})
		}
		if (c < 0x20 || c == 0x7f) && c != '\t' && !(c == '\n' && keepNewlines) {
			return false
		}
	}
	return true
}

// escapeLen returns the length of the escape sequence that follows an ESC
// at the start of s. A lone ESC has length 0.
func escapeLen(s string) int {
	if s == "" {
		return 0
	}
	switch c := s[0]; {
	case c == '[':
		return 1 + csiLen(s[1:])
	case c == ']', c == 'P', c == 'X', c == '^', c == '_': // OSC, DCS, SOS, PM, APC
		return 1 + controlStringLen(s[1:])
	case c >= 0x20 && c <= 0x2f: // intermediate bytes, then a final byte
		n := 1
		for n < len(s) && s[n] >= 0x20 && s[n] <= 0x2f {
			n++
		}
		if n < len(s) && s[n] >= 0x30 && s[n] <= 0x7e {
			n++
		}
		return n
	case c >= 0x30 && c <= 0x7e:
		return 1
	}
	return 0
}

// csiLen returns the length of a control sequence's parameter and
// intermediate bytes and its final byte.
func csiLen(s string) int {
	n := 0
	for n < len(s) && s[n] >= 0x30 && s[n] <= 0x3f {
		n++
	}
	for n < len(s) && s[n] >= 0x20 && s[n] <= 0x2f {
		n++
	}
	if n < len(s) && s[n] >= 0x40 && s[n] <= 0x7e {
		n++
	}
	return n
}

// controlStringLen returns the length of a control string's content and
// its terminator: BEL, ESC \ or ST. An ESC that does not start ST ends the
// string without being part of it. An unterminated string runs to the end.
func controlStringLen(s string) int {
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\a':
			return i + 1
		case s[i] == '\x1b':
			if i+1 < len(s) && s[i+1] == '\\' {
				return i + 2
			}
			return i
		case strings.HasPrefix(s[i:], "\u009c"): // ST
			return i + len("\u009c")
		}
	}
	return len(s)
}

// StripControlJSON applies StripControlMultiline to the strings in a JSON
// document: to every string and object key, or, when keys are given, only
// to the string values of objects' fields with those names. It returns
// body itself when nothing needed stripping or body is not JSON.
func StripControlJSON(body []byte, keys ...string) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v interface{}
	if dec.Decode(&v) != nil {
		return body
	}
	var only map[string]bool
	if len(keys) > 0 {
		only = make(map[string]bool, len(keys))
		for _, k := range keys {
			only[k] = true
		}
	}
	v, changed := stripJSONStrings(v, only == nil, only)
	if !changed {
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(v) != nil {
		return body
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// stripJSONStrings strips v when it is a string and strip is set, and
// walks into arrays and objects. only == nil means every string.
func stripJSONStrings(v interface{}, strip bool, only map[string]bool) (interface{}, bool) {
	switch node := v.(type) {
	case string:
		if !strip {
			return node, false
		}
		s := StripControlMultiline(node)
		return s, s != node
	case []interface{}:
		changed := false
		for i, child := range node {
			c, ch := stripJSONStrings(child, only == nil, only)
			node[i] = c
			changed = changed || ch
		}
		return node, changed
	case map[string]interface{}:
		changed := false
		out := make(map[string]interface{}, len(node))
		for k, child := range node {
			key := k
			if only == nil {
				key = StripControl(k)
			}
			c, ch := stripJSONStrings(child, only == nil || only[k], only)
			out[key] = c
			changed = changed || ch || key != k
		}
		return out, changed
	}
	return v, false
}
