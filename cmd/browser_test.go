package cmd

import "testing"

// oauthBrowserURL is an authorize URL with several query separators. The
// Windows bug dropped everything after the first '&'.
const oauthBrowserURL = "https://auth.example/oauth2/authorize?response_type=code&client_id=abc&redirect_uri=http%3A%2F%2F127.0.0.1%3A8319%2Fcallback&state=xyz&code_challenge=qq&code_challenge_method=S256&scope=openid+profile"

func TestBrowserCommand(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", oauthBrowserURL}},
		{"linux", "xdg-open", []string{oauthBrowserURL}},
		{"freebsd", "xdg-open", []string{oauthBrowserURL}},
		{"openbsd", "xdg-open", []string{oauthBrowserURL}},
		{"netbsd", "xdg-open", []string{oauthBrowserURL}},
		{"darwin", "open", []string{oauthBrowserURL}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, err := browserCommand(tt.goos, oauthBrowserURL)
			if err != nil {
				t.Fatalf("browserCommand: %v", err)
			}
			if name != tt.name || len(args) != len(tt.args) {
				t.Fatalf("got %q %#v, want %q %#v", name, args, tt.name, tt.args)
			}
			for i := range args {
				if args[i] != tt.args[i] {
					t.Fatalf("arg %d = %q, want %q", i, args[i], tt.args[i])
				}
			}
		})
	}

	if _, _, err := browserCommand("plan9", oauthBrowserURL); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}
