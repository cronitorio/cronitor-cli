package cmd

import (
	"syscall"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

// EscapeArg is the quoting os/exec applies before CreateProcess. If it
// wrapped the URL in quotes, rundll32 would still receive one argument, but
// those quote characters would pass through to FileProtocolHandler.
func TestWindowsEscapeArgLeavesOAuthURLUnquoted(t *testing.T) {
	if got := syscall.EscapeArg(oauthBrowserURL); got != oauthBrowserURL {
		t.Fatalf("EscapeArg(%q) = %q, want the URL unchanged", oauthBrowserURL, got)
	}

	auth, err := lib.NewBrowserAuthorization(lib.DefaultWorkOSAuthKitURL, lib.DefaultWorkOSClientID, lib.WorkOSScope())
	if err != nil {
		t.Fatalf("NewBrowserAuthorization: %v", err)
	}
	if got := syscall.EscapeArg(auth.URL); got != auth.URL {
		t.Fatalf("EscapeArg(%q) = %q, want the authorize URL unchanged", auth.URL, got)
	}
}
