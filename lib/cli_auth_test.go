package lib_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
)

func parseForm(body string) (url.Values, error) {
	return url.ParseQuery(body)
}

const (
	testDeviceCode  = "DEVICE_POLL_CODE_SECRET_do_not_print"
	testUserCode    = "WD-TEST-42"
	testAccessToken = "WORKOS_ACCESS_TOKEN_SECRET_do_not_print"
	testRefreshTok  = "WORKOS_REFRESH_TOKEN_SECRET_do_not_print"
	testMachineKey  = "cronitor_machine_key_SECRET_do_not_print"
	testClientID    = "client_test_cronitor_cli"
)

func TestRequestDeviceAuthorization_FormAndResponse(t *testing.T) {
	var gotContentType, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		gotBody = string(body)
		if r.URL.Path != "/oauth2/device_authorization" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"device_code":               testDeviceCode,
			"user_code":                 testUserCode,
			"verification_uri":          "https://login.example/device",
			"verification_uri_complete": "https://login.example/device?user_code=" + testUserCode,
			"expires_in":                300,
			"interval":                  5,
		})
	}))
	defer server.Close()

	auth, err := lib.RequestDeviceAuthorization(server.URL, testClientID, "openid")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("content-type: %s", gotContentType)
	}
	if !strings.Contains(gotBody, "client_id="+testClientID) || !strings.Contains(gotBody, "scope=openid") {
		t.Errorf("form body: %s", gotBody)
	}
	if auth.DeviceCode != testDeviceCode || auth.UserCode != testUserCode {
		t.Errorf("codes: %+v", auth)
	}
	if auth.VerificationURI == "" {
		t.Fatal("missing verification_uri")
	}
}

func TestRequestDeviceAuthorization_DefaultsIntervalAndExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"device_code":      testDeviceCode,
			"user_code":        testUserCode,
			"verification_uri": "https://login.example/device",
		})
	}))
	defer server.Close()

	auth, err := lib.RequestDeviceAuthorization(server.URL, testClientID, "")
	if err != nil {
		t.Fatal(err)
	}
	if auth.Interval != 5 {
		t.Errorf("interval default: %d", auth.Interval)
	}
	if auth.ExpiresIn != 300 {
		t.Errorf("expires_in default: %d", auth.ExpiresIn)
	}
}

func TestExchangeDeviceToken_PendingSlowDownDeniedExpired(t *testing.T) {
	cases := []struct {
		errorCode string
	}{
		{"authorization_pending"},
		{"slow_down"},
		{"access_denied"},
		{"expired_token"},
	}
	for _, tc := range cases {
		t.Run(tc.errorCode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/oauth2/token" {
					t.Errorf("path: %s", r.URL.Path)
				}
				body, _ := io.ReadAll(r.Body)
				form, err := parseForm(string(body))
				if err != nil {
					t.Fatal(err)
				}
				if form.Get("grant_type") != lib.DeviceCodeGrantType {
					t.Errorf("grant: %s", body)
				}
				if form.Get("device_code") != testDeviceCode {
					t.Errorf("device_code missing")
				}
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": tc.errorCode})
			}))
			defer server.Close()

			token, oauthErr, err := lib.ExchangeDeviceToken(server.URL, testClientID, testDeviceCode)
			if err != nil {
				t.Fatal(err)
			}
			if token != nil {
				t.Fatal("expected no token")
			}
			if oauthErr != tc.errorCode {
				t.Errorf("got %q", oauthErr)
			}
		})
	}
}

func TestExchangeDeviceToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  testAccessToken,
			"refresh_token": testRefreshTok,
			"token_type":    "Bearer",
			"expires_in":    300,
		})
	}))
	defer server.Close()

	token, oauthErr, err := lib.ExchangeDeviceToken(server.URL, testClientID, testDeviceCode)
	if err != nil || oauthErr != "" {
		t.Fatalf("err=%v oauth=%s", err, oauthErr)
	}
	if token.AccessToken != testAccessToken {
		t.Fatal("missing access token")
	}
	token.Discard()
	if token.AccessToken != "" || token.RefreshToken != "" || token.IDToken != "" {
		t.Fatal("Discard left token fields populated")
	}
}

func TestCreateMachineCredential_FrozenContract(t *testing.T) {
	var gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotAuth = r.Header.Get("Authorization")
		gotBody = string(body)
		if r.Method != "POST" || r.URL.Path != "/api/cli/machine-credentials" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":          testMachineKey,
			"name":         "cli-testhost",
			"kind":         "machine",
			"scopes":       []string{"monitors:read", "telemetry:write"},
			"organization": "Acme",
		})
	}))
	defer server.Close()

	cred, err := lib.CreateMachineCredential(server.URL+"/api", testAccessToken, "testhost")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+testAccessToken {
		t.Errorf("authorization: %s", gotAuth)
	}
	var payload map[string]string
	if json.Unmarshal([]byte(gotBody), &payload) != nil {
		t.Fatalf("body: %s", gotBody)
	}
	if payload["hostname"] != "testhost" || len(payload) != 1 {
		t.Errorf("frozen body must be hostname only: %#v", payload)
	}
	if cred.Key != testMachineKey || cred.Name != "cli-testhost" || cred.Kind != "machine" || cred.Organization != "Acme" {
		t.Errorf("cred: %+v", cred)
	}
	if len(cred.Scopes) != 2 {
		t.Errorf("scopes: %#v", cred.Scopes)
	}
}

func TestGetCurrentMachineCredential_NeverReturnsKey(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.Method != "GET" || r.URL.Path != "/api/cli/machine-credentials/current" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":          testMachineKey,
			"name":         "cli-testhost",
			"kind":         "machine",
			"scopes":       []string{"telemetry:write"},
			"organization": map[string]string{"name": "Acme", "id": "org_1"},
		})
	}))
	defer server.Close()

	cred, err := lib.GetCurrentMachineCredential(server.URL+"/api", testMachineKey)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth, got %s", gotAuth)
	}
	if cred.Key != "" {
		t.Fatal("GET current must never surface the key")
	}
	if cred.Organization != "Acme" {
		t.Errorf("organization: %s", cred.Organization)
	}
}

func TestDeleteCurrentMachineCredential_204(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(204)
	}))
	defer server.Close()

	if err := lib.DeleteCurrentMachineCredential(server.URL+"/api", testMachineKey); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/cli/machine-credentials/current" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
}

func TestDeleteCurrentMachineCredential_401AlreadyInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"error":"unauthorized"}`)
	}))
	defer server.Close()

	err := lib.DeleteCurrentMachineCredential(server.URL+"/api", testMachineKey)
	if _, ok := err.(*lib.CredentialGoneError); !ok {
		t.Fatalf("expected CredentialGoneError, got %v", err)
	}
}

func TestDeleteCurrentMachineCredential_403Unconfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"error":"forbidden"}`)
	}))
	defer server.Close()

	err := lib.DeleteCurrentMachineCredential(server.URL+"/api", testMachineKey)
	unconf, ok := err.(*lib.CredentialRevokeUnconfirmedError)
	if !ok {
		t.Fatalf("expected CredentialRevokeUnconfirmedError, got %v", err)
	}
	if unconf.Status != 403 {
		t.Errorf("status %d", unconf.Status)
	}
}

func TestDeleteCurrentMachineCredential_404Unconfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		io.WriteString(w, `{"error":"not found"}`)
	}))
	defer server.Close()

	err := lib.DeleteCurrentMachineCredential(server.URL+"/api", testMachineKey)
	unconf, ok := err.(*lib.CredentialRevokeUnconfirmedError)
	if !ok {
		t.Fatalf("expected CredentialRevokeUnconfirmedError, got %v", err)
	}
	if unconf.Status != 404 {
		t.Errorf("status %d", unconf.Status)
	}
}

func TestGetCurrentMachineCredential_Gone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		io.WriteString(w, `{"error":"invalid"}`)
	}))
	defer server.Close()

	_, err := lib.GetCurrentMachineCredential(server.URL+"/api", testMachineKey)
	if _, ok := err.(*lib.CredentialGoneError); !ok {
		t.Fatalf("expected CredentialGoneError, got %v", err)
	}
}

func withClearedAuthKitURL(t *testing.T) {
	t.Helper()
	old := lib.WorkOSAuthKitURLOverride
	lib.WorkOSAuthKitURLOverride = ""
	t.Cleanup(func() { lib.WorkOSAuthKitURLOverride = old })
	t.Setenv("CRONITOR_WORKOS_AUTHKIT_URL", "")
}

func TestWorkOSAuthKitURL_DefaultIsAuthCronitor(t *testing.T) {
	withClearedAuthKitURL(t)

	const want = "https://auth.cronitor.io"
	if lib.DefaultWorkOSAuthKitURL != want {
		t.Errorf("DefaultWorkOSAuthKitURL = %q, want %q", lib.DefaultWorkOSAuthKitURL, want)
	}
	if got := lib.WorkOSAuthKitURL(); got != want {
		t.Errorf("WorkOSAuthKitURL() = %q, want %q", got, want)
	}
}

func TestWorkOSClientID_ProductionDefaultAndOverrides(t *testing.T) {
	old := lib.WorkOSClientIDOverride
	t.Cleanup(func() { lib.WorkOSClientIDOverride = old })
	lib.WorkOSClientIDOverride = ""

	const productionClientID = "client_01M2S35Z13KFG88QDXA7W8A41K"
	for _, env := range []string{"", "   ", "\t"} {
		t.Setenv("CRONITOR_WORKOS_CLIENT_ID", env)
		if got := lib.WorkOSClientID(); got != productionClientID {
			t.Errorf("env %q: WorkOSClientID() = %q, want %q", env, got, productionClientID)
		}
	}

	t.Setenv("CRONITOR_WORKOS_CLIENT_ID", " client_staging ")
	if got := lib.WorkOSClientID(); got != "client_staging" {
		t.Errorf("environment override: got %q", got)
	}
	lib.WorkOSClientIDOverride = "client_build_override"
	if got := lib.WorkOSClientID(); got != "client_build_override" {
		t.Errorf("build override should win over environment: got %q", got)
	}
}

func TestWorkOSAuthKitURL_EmptyAndWhitespaceEnvUseDefault(t *testing.T) {
	withClearedAuthKitURL(t)

	for _, env := range []string{"", "   ", "\t"} {
		t.Setenv("CRONITOR_WORKOS_AUTHKIT_URL", env)
		if got := lib.WorkOSAuthKitURL(); got != lib.DefaultWorkOSAuthKitURL {
			t.Errorf("env %q: WorkOSAuthKitURL() = %q, want default %q", env, got, lib.DefaultWorkOSAuthKitURL)
		}
	}
}

func TestWorkOSAuthKitURL_OverrideAndEnvPrecedence(t *testing.T) {
	old := lib.WorkOSAuthKitURLOverride
	t.Cleanup(func() { lib.WorkOSAuthKitURLOverride = old })

	t.Setenv("CRONITOR_WORKOS_AUTHKIT_URL", "https://env.example/authkit/")
	lib.WorkOSAuthKitURLOverride = ""
	if got := lib.WorkOSAuthKitURL(); got != "https://env.example/authkit" {
		t.Errorf("env override: got %q", got)
	}

	lib.WorkOSAuthKitURLOverride = "https://override.example/authkit/"
	if got := lib.WorkOSAuthKitURL(); got != "https://override.example/authkit" {
		t.Errorf("test override should win over env: got %q", got)
	}

	lib.WorkOSAuthKitURLOverride = ""
	t.Setenv("CRONITOR_WORKOS_AUTHKIT_URL", "")
	if got := lib.WorkOSAuthKitURL(); got != "https://auth.cronitor.io" {
		t.Errorf("cleared override/env must fall back to AuthKit default: got %q", got)
	}
}
