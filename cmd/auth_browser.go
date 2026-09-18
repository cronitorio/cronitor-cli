package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"golang.org/x/term"
)

var readCallbackFn = readCallbackFromTerminal

func authorizeBrowser(issuer, clientID string, timeout time.Duration) (*lib.AuthorizationToken, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	a, err := lib.NewBrowserAuthorization(issuer, clientID, lib.WorkOSScope())
	if err != nil {
		return nil, err
	}
	var callback string
	if authNoBrowser {
		fmt.Println("Open this URL in any browser and complete sign-in:")
		fmt.Println()
		fmt.Println(a.URL)
		fmt.Println()
		fmt.Println("After sign-in, the browser may show a localhost connection error. Copy the entire URL from its address bar and paste it here. Keep this terminal open.")
		callback, err = readCallbackFn(ctx, a.URL)
	} else {
		listener, listenErr := net.Listen("tcp4", "127.0.0.1:8319")
		if listenErr != nil {
			return nil, fmt.Errorf("cannot listen on 127.0.0.1:8319; close other login attempts or use --no-browser for callback paste-back")
		}
		callbacks := make(chan string, 1)
		server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10}
		server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			if r.Method != http.MethodGet || r.Host != "127.0.0.1:8319" || len(r.RequestURI) > 16<<10 {
				http.Error(w, "Invalid callback", 400)
				return
			}
			raw := "http://" + r.Host + r.URL.RequestURI()
			if _, err := a.CallbackCode(raw); err != nil && !errors.Is(err, lib.ErrAuthorizationDenied) {
				http.Error(w, "Invalid or declined authorization. Return to the terminal or retry login.", 400)
				return
			}
			select {
			case callbacks <- raw:
				io.WriteString(w, "Authorization received. Return to your terminal to check that login completed.")
			default:
				http.Error(w, "Authorization already received", 409)
			}
		})
		defer server.Close()
		go server.Serve(listener)
		fmt.Println("Open this URL in a browser on this computer:")
		fmt.Println()
		fmt.Println(a.URL)
		fmt.Println()
		fmt.Println("For a browser on another computer, cancel and rerun with --no-browser.")
		openBrowserFn(a.URL)
		select {
		case callback = <-callbacks:
		case <-ctx.Done():
			return nil, fmt.Errorf("login cancelled or timed out; start a new login")
		}
	}
	if err != nil {
		return nil, err
	}
	code, err := a.CallbackCode(callback)
	if err != nil {
		return nil, err
	}
	rememberSecret(code)
	return a.Exchange(ctx, code)
}

// Terminal input is hidden. On timeout/cancellation restore echo before returning.
// Redirected stdin is supported, but callbacks must not be put in shell history,
// agent conversations, or recorded tool arguments.
func readCallbackFromTerminal(ctx context.Context, _ string) (string, error) {
	fmt.Print("Paste callback URL (input hidden): ")
	input := os.Stdin
	fd := int(input.Fd())
	terminal := term.IsTerminal(fd)
	if terminal {
		state, err := term.GetState(fd)
		if err != nil {
			return "", fmt.Errorf("could not read terminal state")
		}
		defer term.Restore(fd, state)
	}
	type result struct {
		value string
		err   error
	}
	results := make(chan result, 1)
	go func() {
		var value string
		var err error
		if terminal {
			var b []byte
			b, err = term.ReadPassword(fd)
			value = string(b)
		} else {
			value, err = bufio.NewReader(io.LimitReader(input, 16<<10)).ReadString('\n')
		}
		results <- result{strings.TrimSpace(value), err}
	}()
	defer fmt.Println()
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("login cancelled or timed out; start a new login")
	case r := <-results:
		if r.err != nil && !(r.err == io.EOF && r.value != "") {
			return "", fmt.Errorf("could not read callback URL")
		}
		if len(r.value) >= 16<<10 {
			return "", fmt.Errorf("callback URL is too long")
		}
		return r.value, nil
	}
}
