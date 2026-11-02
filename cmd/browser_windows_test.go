//go:build windows

package cmd

import (
	"syscall"
	"testing"
)

// EscapeArg is the quoting os/exec applies before CreateProcess. The OAuth
// URL must come back unchanged, or rundll32 would not receive it as one argument.
func TestWindowsEscapeArgLeavesOAuthURLUnquoted(t *testing.T) {
	if got := syscall.EscapeArg(oauthBrowserURL); got != oauthBrowserURL {
		t.Fatalf("EscapeArg(%q) = %q, want the URL unchanged", oauthBrowserURL, got)
	}
}
