package cmd

import (
	"strings"
	"testing"
)

// oauthBrowserURL is an authorize URL with several query separators. The
// Windows bug dropped everything after the first '&'.
const oauthBrowserURL = "https://auth.example/oauth2/authorize?response_type=code&client_id=abc&redirect_uri=http%3A%2F%2F127.0.0.1%3A8319%2Fcallback&state=xyz&code_challenge=qq&code_challenge_method=S256&scope=openid+profile"

func TestBrowserCommandWindowsPassesQueryIntact(t *testing.T) {
	name, args, err := browserCommand("windows", oauthBrowserURL)
	if err != nil {
		t.Fatalf("browserCommand: %v", err)
	}
	if name != "rundll32" {
		t.Fatalf("command = %q, want rundll32", name)
	}
	if len(args) != 2 {
		t.Fatalf("args = %#v, want exactly two arguments", args)
	}
	if args[0] != "url.dll,FileProtocolHandler" {
		t.Fatalf("entry = %q", args[0])
	}
	if args[1] != oauthBrowserURL {
		t.Fatalf("url argument =\n%s\nwant\n%s", args[1], oauthBrowserURL)
	}
	if strings.Count(args[1], "&") < 2 || strings.Count(args[1], "=") < 2 {
		t.Fatalf("url argument lost query separators: %q", args[1])
	}
	for _, arg := range append([]string{name}, args...) {
		if arg == "cmd" || arg == "/c" || arg == "start" || strings.Contains(arg, "^&") {
			t.Fatalf("windows opener fell back to cmd quoting/escaping: %q %#v", name, args)
		}
	}
}

func TestBrowserCommandDoesNotSplitQueryAcrossArguments(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			name, args, err := browserCommand(goos, oauthBrowserURL)
			if err != nil {
				t.Fatalf("browserCommand: %v", err)
			}
			if name == "" || len(args) == 0 {
				t.Fatalf("empty command: %q %#v", name, args)
			}
			if got := args[len(args)-1]; got != oauthBrowserURL {
				t.Fatalf("last arg = %q, want the full URL as one argument", got)
			}
			for _, arg := range args[:len(args)-1] {
				if strings.Contains(arg, "&") || strings.Contains(arg, oauthBrowserURL) {
					t.Fatalf("query leaked out of the URL argument: %#v", args)
				}
			}
		})
	}
}

func TestBrowserCommandOtherPlatforms(t *testing.T) {
	tests := []struct {
		goos string
		name string
	}{
		{goos: "linux", name: "xdg-open"},
		{goos: "darwin", name: "open"},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := browserCommand(tt.goos, "http://localhost:9000")
			if err != nil {
				t.Fatalf("browserCommand: %v", err)
			}
			if name != tt.name || len(args) != 1 || args[0] != "http://localhost:9000" {
				t.Fatalf("got %q %#v, want %q [url]", name, args, tt.name)
			}
		})
	}

	if _, _, err := browserCommand("plan9", oauthBrowserURL); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}
