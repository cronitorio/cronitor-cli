package lib_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
)

func TestPKCEAuthorizationAndExchange(t *testing.T) {
	var authorization *lib.BrowserAuthorization
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		r.ParseForm()
		q, _ := url.Parse(authorization.URL)
		digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if q.Query().Get("code_challenge") != base64.RawURLEncoding.EncodeToString(digest[:]) {
			t.Error("PKCE mismatch")
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != lib.CLIAuthRedirectURI || r.Form.Get("code") != "secret-code" || r.Form.Get("client_id") != "client_test" {
			t.Error("incorrect exchange")
		}
		if r.Form.Has("client_secret") || r.Form.Has("resource") {
			t.Error("unexpected secret/resource")
		}
		json.NewEncoder(w).Encode(map[string]string{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer"})
	}))
	defer server.Close()
	var err error
	authorization, err = lib.NewBrowserAuthorization(server.URL, "client_test", "openid profile email")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(authorization.URL)
	q := u.Query()
	if u.Path != "/oauth2/authorize" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("scope") != "openid profile email" || q.Get("redirect_uri") != lib.CLIAuthRedirectURI || len(q.Get("state")) < 43 || q.Has("resource") {
		t.Fatalf("invalid authorization URL")
	}
	next, _ := lib.NewBrowserAuthorization(server.URL, "client_test", "openid profile email")
	if next.URL == authorization.URL {
		t.Fatal("reused randomness")
	}
	code, err := authorization.CallbackCode(lib.CLIAuthRedirectURI + "?code=secret-code&state=" + q.Get("state"))
	if err != nil {
		t.Fatal(err)
	}
	token, err := authorization.Exchange(context.Background(), code)
	if err != nil || token.AccessToken != "access" {
		t.Fatalf("exchange: %v", err)
	}
	token.Discard()
	if token.AccessToken != "" || token.RefreshToken != "" {
		t.Fatal("tokens retained")
	}
}

func TestPKCERejectsInvalidCallbacks(t *testing.T) {
	a, _ := lib.NewBrowserAuthorization("https://auth.cronitor.io", "client_test", "openid")
	u, _ := url.Parse(a.URL)
	valid := lib.CLIAuthRedirectURI + "?code=secret-code&state=" + u.Query().Get("state")
	for _, raw := range []string{
		strings.Replace(valid, "127.0.0.1", "localhost", 1), strings.Replace(valid, ":8319", ":8320", 1), strings.Replace(valid, "http:", "https:", 1), strings.Replace(valid, "/callback", "/other", 1), valid + "#fragment", valid + "&code=other", valid + "&state=other", valid + "&error=access_denied", valid + "&iss=https://other.example", strings.Replace(valid, "code=secret-code", "code=", 1), strings.Replace(valid, "state=", "state=wrong", 1), strings.Replace(valid, "127.0.0.1", "user@127.0.0.1", 1), valid + "&iss=https://auth.cronitor.io&iss=https://auth.cronitor.io",
	} {
		if _, err := a.CallbackCode(raw); err == nil {
			t.Errorf("accepted invalid callback")
		} else if strings.Contains(err.Error(), "secret-code") {
			t.Error("leaked code")
		}
	}
	if _, err := a.CallbackCode(valid + "&iss=https%3A%2F%2Fauth.cronitor.io"); err != nil {
		t.Fatal(err)
	}
}

func TestPKCEExchangeTimeoutAndErrorRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"secret-code","error_description":"secret-code"}`))
	}))
	defer server.Close()
	a, _ := lib.NewBrowserAuthorization(server.URL, "client", "openid")
	_, err := a.Exchange(context.Background(), "secret-code")
	if err == nil || strings.Contains(err.Error(), "secret-code") {
		t.Fatal("unsafe token error")
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := a.Exchange(ctx, "secret-code"); err == nil {
		t.Fatal("expired context accepted")
	}
}

func TestPKCEExchangeCancelsInFlightRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	a, _ := lib.NewBrowserAuthorization(server.URL, "client", "openid")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := a.Exchange(ctx, "secret"); err == nil {
		t.Fatal("timeout accepted")
	}
	if time.Since(start) > 300*time.Millisecond {
		t.Fatal("token exchange ignored deadline")
	}
}

func TestPKCEExchangeNeverFollowsRedirect(t *testing.T) {
	var forwarded bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded = true }))
	defer destination.Close()
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer issuer.Close()
	a, _ := lib.NewBrowserAuthorization(issuer.URL, "client", "openid")
	if _, err := a.Exchange(context.Background(), "secret"); err == nil {
		t.Fatal("redirect accepted")
	}
	if forwarded {
		t.Fatal("forwarded code/verifier to redirect destination")
	}
}
