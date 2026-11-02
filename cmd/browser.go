package cmd

import (
	"fmt"
	"os/exec"
	"runtime"
)

// browserCommand returns the program and arguments that open url in the
// default browser. The URL is always a single argument and is passed to
// exec.Command directly, with no shell in between.
//
// Windows must not use cmd /c start. cmd.exe reparses that command line and
// treats an unquoted '&' as a command separator, which truncates an OAuth
// URL at the first query separator. Wrapping the URL in quotes is not
// reliable either: Go's EscapeArg does not quote '&' (only spaces, tabs,
// quotes, and backslashes), cmd /c strips quotes when the line contains
// &<>()@^|, '%' is expanded even inside quotes, and start takes the first
// quoted string as a window title. rundll32 url.dll,FileProtocolHandler is
// not a shell. CreateProcess hands it the URL as one argument, and '&',
// '=', and '%' reach the browser unchanged.
func browserCommand(goos, url string) (string, []string, error) {
	switch goos {
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "darwin":
		return "open", []string{url}, nil
	default:
		return "", nil, fmt.Errorf("unsupported platform")
	}
}

func openBrowser(url string) {
	name, args, err := browserCommand(runtime.GOOS, url)
	if err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
		return
	}
	if err := exec.Command(name, args...).Start(); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}
}
