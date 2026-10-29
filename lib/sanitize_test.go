package lib_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

func TestStripControl(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text and tabs are kept", "ValueError:\tboom — naïve 日本", "ValueError:\tboom — naïve 日本"},
		{"ANSI color", "\x1b[1;31mred\x1b[0m text", "red text"},
		{"OSC 52 clipboard write ended by BEL", "before\x1b]52;c;Y3VybCBldmlsLnNoIHwgc2g=\x07after", "beforeafter"},
		{"OSC 52 clipboard write ended by ST", "before\x1b]52;c;Y3VybA==\x1b\\after", "beforeafter"},
		{"OSC hyperlink", "\x1b]8;;https://evil.example\x1b\\click\x1b]8;;\x1b\\", "click"},
		{"unterminated OSC runs to the end", "keep\x1b]52;c;Y3VybA==", "keep"},
		{"CSI cursor moves and erase", "real\x1b[2A\x1b[10;5Hfake\x1b[2K", "realfake"},
		{"CSI with private parameters", "\x1b[?1049hscreen", "screen"},
		{"two-character escapes", "\x1bcreset\x1b(Bcharset\x1b7saved", "resetcharsetsaved"},
		{"lone ESC at the end", "end\x1b", "end"},
		{"ESC before a control character", "a\x1b\x07b", "ab"},
		{"C1 CSI", "\u009b31mred\u009b0m", "red"},
		{"C1 OSC ended by C1 ST", "a\u009d52;c;Y3VybA==\u009cb", "ab"},
		{"other C1 controls", "a\u0085b\u008dc", "abc"},
		{"carriage return, backspace, bell, NUL and DEL", "safe\rspoof\b\b\x07\x00x\x7f", "safespoofx"},
		{"newlines are stripped", "line one\nline two\r\n", "line oneline two"},
		{"invalid UTF-8 is replaced", "a\x9bb", "a�b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lib.StripControl(tc.in); got != tc.want {
				t.Errorf("StripControl(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStripControlMultiline(t *testing.T) {
	in := "Traceback:\r\n  \x1b[31mFile\x1b[0m \"x\"\n\x1b]52;c;Y3VybA==\x07ValueError:\tboom\rfake"
	want := "Traceback:\n  File \"x\"\nValueError:\tboomfake"
	if got := lib.StripControlMultiline(in); got != want {
		t.Errorf("StripControlMultiline = %q, want %q", got, want)
	}
}

func TestStripControlJSON(t *testing.T) {
	body := []byte(`{"title": "\u001b[31mboom\u001b[0m", "count": 12345678901234567890,
		"tags": {"k\u001b[2J": "v\u009b2J"}, "frames": [{"context_line": "x\u0007"}],
		"body": "line\nnext\r\n"}`)

	var all map[string]interface{}
	if err := json.Unmarshal(lib.StripControlJSON(body), &all); err != nil {
		t.Fatal(err)
	}
	if all["title"] != "boom" || all["body"] != "line\nnext\n" {
		t.Errorf("strings: %v", all)
	}
	if tags := all["tags"].(map[string]interface{}); tags["k"] != "v" || len(tags) != 1 {
		t.Errorf("object keys and values are stripped too: %v", tags)
	}
	if out := string(lib.StripControlJSON(body)); !strings.Contains(out, "12345678901234567890") {
		t.Errorf("numbers must keep every digit: %s", out)
	}

	var some map[string]interface{}
	if err := json.Unmarshal(lib.StripControlJSON(body, "title", "context_line"), &some); err != nil {
		t.Fatal(err)
	}
	if some["title"] != "boom" || some["frames"].([]interface{})[0].(map[string]interface{})["context_line"] != "x" {
		t.Errorf("named keys are stripped at any depth: %v", some)
	}
	if some["body"] != "line\nnext\r\n" {
		t.Errorf("keys not named are left alone: %q", some["body"])
	}

	clean := []byte(`{"title": "fine", "n": 1.50}`)
	if got := lib.StripControlJSON(clean); string(got) != string(clean) {
		t.Errorf("clean JSON is returned as is, got %s", got)
	}
	if got := lib.StripControlJSON([]byte("not json \x1b[2J")); string(got) != "not json \x1b[2J" {
		t.Errorf("non-JSON is returned as is, got %q", got)
	}
}
