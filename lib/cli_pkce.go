package lib

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// CLIAuthRedirectURI must be registered on the public Cronitor CLI Connect app.
const CLIAuthRedirectURI = "http://127.0.0.1:8319/callback"

var ErrAuthorizationDenied = errors.New("authorization was denied or failed; start a new login")

// BrowserAuthorization binds one browser login to its initiating CLI process.
// The verifier is private and never included in the browser URL or persisted.
type BrowserAuthorization struct {
	URL                               string
	issuer, clientID, state, verifier string
}

func NewBrowserAuthorization(issuer, clientID, scope string) (*BrowserAuthorization, error) {
	u, err := url.Parse(issuer)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
		return nil, fmt.Errorf("invalid WorkOS issuer URL")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, fmt.Errorf("WorkOS Connect client_id is not configured")
	}
	random := make([]byte, 64)
	if _, err := rand.Read(random); err != nil {
		return nil, fmt.Errorf("could not initialize secure login")
	}
	a := &BrowserAuthorization{issuer: strings.TrimRight(issuer, "/"), clientID: clientID, state: base64.RawURLEncoding.EncodeToString(random[:32]), verifier: base64.RawURLEncoding.EncodeToString(random[32:])}
	digest := sha256.Sum256([]byte(a.verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {CLIAuthRedirectURI}, "state": {a.state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])}, "scope": {scope}}
	a.URL = a.issuer + "/oauth2/authorize?" + q.Encode()
	return a, nil
}

// CallbackCode accepts only this attempt's exact registered callback and state.
// Errors never include provider-supplied text or callback secrets.
func (a *BrowserAuthorization) CallbackCode(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" || u.Host != "127.0.0.1:8319" || u.User != nil || u.EscapedPath() != "/callback" || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("invalid callback URL; paste the full localhost callback URL")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", fmt.Errorf("invalid callback parameters")
	}
	for _, values := range q {
		if len(values) != 1 {
			return "", fmt.Errorf("duplicate callback parameters")
		}
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(a.state)) != 1 {
		return "", fmt.Errorf("callback does not match this login attempt")
	}
	if issuer, ok := q["iss"]; ok && issuer[0] != a.issuer {
		return "", fmt.Errorf("callback issuer does not match")
	}
	if q.Has("error") {
		return "", ErrAuthorizationDenied
	}
	if strings.TrimSpace(q.Get("code")) == "" {
		return "", fmt.Errorf("callback is missing an authorization code")
	}
	return q.Get("code"), nil
}

func (a *BrowserAuthorization) Exchange(ctx context.Context, code string) (*AuthorizationToken, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {a.clientID}, "code": {code}, "redirect_uri": {CLIAuthRedirectURI}, "code_verifier": {a.verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.issuer+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("could not create token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CronitorCLI")
	// Never forward the code and verifier to a redirect destination.
	client := *cliAuthClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed or timed out; start a new login")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("could not read token response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange failed (HTTP %d); start a new login", resp.StatusCode)
	}
	var token AuthorizationToken
	if json.Unmarshal(body, &token) != nil || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") {
		return nil, fmt.Errorf("invalid token response")
	}
	return &token, nil
}
