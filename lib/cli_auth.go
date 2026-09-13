package lib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Public WorkOS Connect device-authorization endpoints (RFC 8628).
const (
	DeviceCodeGrantType     = "urn:ietf:params:oauth:grant-type:device_code"
	DefaultDeviceInterval   = 5 * time.Second
	SlowDownIncrement       = 5 * time.Second
	DefaultDeviceExpiry     = 300 * time.Second
	DefaultWorkOSClientID   = "client_cronitor_cli"
	DefaultWorkOSAuthKitURL = "https://login.cronitor.io"

	envWorkOSClientID   = "CRONITOR_WORKOS_CLIENT_ID"
	envWorkOSAuthKitURL = "CRONITOR_WORKOS_AUTHKIT_URL"
	envWorkOSScope      = "CRONITOR_WORKOS_SCOPE"
)

// Test overrides. Production reads baked-in defaults, then env.
var (
	WorkOSAuthKitURLOverride string
	WorkOSClientIDOverride   string
	WorkOSScopeOverride      string
	CLIAuthHTTPClient        *http.Client
)

// DeviceAuthorization is the WorkOS/RFC 8628 device authorization response.
// DeviceCode is for token polling only and must never be shown to the user.
type DeviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceToken is the successful device-code grant response. Tokens are
// discarded after the machine-credential bootstrap; they are never persisted.
type DeviceToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// Discard zeros token fields so a later log or dump cannot print them.
func (t *DeviceToken) Discard() {
	if t == nil {
		return
	}
	t.AccessToken = ""
	t.RefreshToken = ""
	t.IDToken = ""
}

// MachineCredential is the frozen Cronitor CLI machine-credential document.
// GET /current never includes Key; POST create does, once.
type MachineCredential struct {
	Key          string   `json:"key,omitempty"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Scopes       []string `json:"scopes"`
	Organization string   `json:"organization"`
}

type machineCredentialWire struct {
	Key          string          `json:"key"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Scopes       json.RawMessage `json:"scopes"`
	Organization json.RawMessage `json:"organization"`
}

func (w machineCredentialWire) credential() MachineCredential {
	return MachineCredential{
		Key:          w.Key,
		Name:         w.Name,
		Kind:         w.Kind,
		Scopes:       parseStringList(w.Scopes),
		Organization: parseOrganization(w.Organization),
	}
}

func parseStringList(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		return list
	}
	var s string
	if json.Unmarshal(raw, &s) == nil && s != "" {
		return strings.Fields(s)
	}
	return nil
}

func parseOrganization(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Name string `json:"name"`
		ID   string `json:"id"`
		Key  string `json:"key"`
		Slug string `json:"slug"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		for _, v := range []string{obj.Name, obj.Key, obj.Slug, obj.ID} {
			if v != "" {
				return v
			}
		}
	}
	return ""
}

// WorkOSAuthKitURL is the public Connect issuer (no trailing slash).
func WorkOSAuthKitURL() string {
	if WorkOSAuthKitURLOverride != "" {
		return strings.TrimRight(WorkOSAuthKitURLOverride, "/")
	}
	if v := strings.TrimSpace(os.Getenv(envWorkOSAuthKitURL)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return strings.TrimRight(DefaultWorkOSAuthKitURL, "/")
}

// WorkOSClientID is the public Connect application id.
func WorkOSClientID() string {
	if WorkOSClientIDOverride != "" {
		return WorkOSClientIDOverride
	}
	if v := strings.TrimSpace(os.Getenv(envWorkOSClientID)); v != "" {
		return v
	}
	return DefaultWorkOSClientID
}

// WorkOSScope is optional; empty means omit the scope parameter.
func WorkOSScope() string {
	if WorkOSScopeOverride != "" {
		return WorkOSScopeOverride
	}
	return strings.TrimSpace(os.Getenv(envWorkOSScope))
}

func cliAuthClient() *http.Client {
	if CLIAuthHTTPClient != nil {
		return CLIAuthHTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func readResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// RequestDeviceAuthorization starts the WorkOS Connect device flow.
func RequestDeviceAuthorization(authKitURL, clientID, scope string) (*DeviceAuthorization, error) {
	if authKitURL == "" {
		return nil, fmt.Errorf("WorkOS AuthKit URL is not configured")
	}
	if clientID == "" {
		return nil, fmt.Errorf("WorkOS Connect client_id is not configured")
	}

	form := url.Values{}
	form.Set("client_id", clientID)
	if scope != "" {
		form.Set("scope", scope)
	}

	endpoint := strings.TrimRight(authKitURL, "/") + "/oauth2/device_authorization"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create device authorization request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CronitorCLI")

	resp, err := cliAuthClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("device authorization request failed: %w", err)
	}
	body, err := readResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to read device authorization response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("device authorization failed (%d): %s", resp.StatusCode, sanitizeOAuthError(body))
	}

	var auth DeviceAuthorization
	if err := json.Unmarshal(body, &auth); err != nil {
		return nil, fmt.Errorf("failed to parse device authorization response")
	}
	if auth.DeviceCode == "" || auth.UserCode == "" || auth.VerificationURI == "" {
		return nil, fmt.Errorf("device authorization response was missing user_code or verification_uri")
	}
	if auth.ExpiresIn <= 0 {
		auth.ExpiresIn = int(DefaultDeviceExpiry / time.Second)
	}
	if auth.Interval <= 0 {
		auth.Interval = int(DefaultDeviceInterval / time.Second)
	}
	return &auth, nil
}

// ExchangeDeviceToken polls the token endpoint once. oauthError is an RFC 8628
// error code (authorization_pending, slow_down, access_denied, expired_token)
// when the grant is not yet complete. err is a transport or unexpected failure.
func ExchangeDeviceToken(authKitURL, clientID, deviceCode string) (token *DeviceToken, oauthError string, err error) {
	form := url.Values{}
	form.Set("grant_type", DeviceCodeGrantType)
	form.Set("device_code", deviceCode)
	form.Set("client_id", clientID)

	endpoint := strings.TrimRight(authKitURL, "/") + "/oauth2/token"
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CronitorCLI")

	resp, err := cliAuthClient().Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("token request failed: %w", err)
	}
	body, err := readResponse(resp)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read token response: %w", err)
	}

	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		IDToken          string `json:"id_token"`
		TokenType        string `json:"token_type"`
		ExpiresIn        int    `json:"expires_in"`
	}
	_ = json.Unmarshal(body, &payload)

	if payload.Error != "" {
		return nil, payload.Error, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("token request failed (%d): %s", resp.StatusCode, sanitizeOAuthError(body))
	}
	if payload.AccessToken == "" {
		return nil, "", fmt.Errorf("token response did not include an access token")
	}
	return &DeviceToken{
		AccessToken:  payload.AccessToken,
		RefreshToken: payload.RefreshToken,
		IDToken:      payload.IDToken,
		TokenType:    payload.TokenType,
		ExpiresIn:    payload.ExpiresIn,
	}, "", nil
}

func sanitizeOAuthError(body []byte) string {
	var payload struct {
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
		Message          string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if payload.Error != "" && payload.ErrorDescription != "" {
			return payload.Error + ": " + payload.ErrorDescription
		}
		if payload.Error != "" {
			return payload.Error
		}
		if payload.Message != "" {
			return payload.Message
		}
	}
	return "request failed"
}

// CreateMachineCredential calls POST /api/cli/machine-credentials with the
// WorkOS access token. The body is exactly {"hostname": ...} per the frozen
// contract. Success is 201 and includes the key once.
func CreateMachineCredential(apiBase, accessToken, hostname string) (*MachineCredential, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("missing access token")
	}
	payload, err := json.Marshal(map[string]string{"hostname": hostname})
	if err != nil {
		return nil, err
	}
	status, body, err := doCLIAuthJSON(http.MethodPost, machineCredentialsURL(apiBase), "Bearer "+accessToken, payload)
	if err != nil {
		return nil, err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return nil, fmt.Errorf("failed to create machine credential (%d): %s", status, sanitizeAPIError(body))
	}
	cred, err := decodeMachineCredential(body)
	if err != nil {
		return nil, err
	}
	if cred.Key == "" {
		return nil, fmt.Errorf("machine credential response did not include a key")
	}
	return cred, nil
}

// GetCurrentMachineCredential calls GET /api/cli/machine-credentials/current
// with Basic key authentication. The key is never returned.
func GetCurrentMachineCredential(apiBase, apiKey string) (*MachineCredential, error) {
	status, body, err := doCLIAuthBasic(http.MethodGet, machineCredentialsCurrentURL(apiBase), apiKey, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound {
		return nil, &CredentialGoneError{Status: status}
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("failed to read machine credential (%d): %s", status, sanitizeAPIError(body))
	}
	cred, err := decodeMachineCredential(body)
	if err != nil {
		return nil, err
	}
	// Contract: metadata only. Drop a key if a buggy server echoes one.
	cred.Key = ""
	return cred, nil
}

// DeleteCurrentMachineCredential calls DELETE /api/cli/machine-credentials/current.
// 204 is the frozen success status; 404 means already gone.
func DeleteCurrentMachineCredential(apiBase, apiKey string) error {
	status, body, err := doCLIAuthBasic(http.MethodDelete, machineCredentialsCurrentURL(apiBase), apiKey, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNoContent || status == http.StatusOK || status == http.StatusNotFound {
		return nil
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return nil
	}
	return fmt.Errorf("failed to revoke machine credential (%d): %s", status, sanitizeAPIError(body))
}

// CredentialGoneError means the stored key is no longer valid remotely.
type CredentialGoneError struct {
	Status int
}

func (e *CredentialGoneError) Error() string {
	return "machine credential is no longer valid"
}

func machineCredentialsURL(apiBase string) string {
	return strings.TrimRight(apiBase, "/") + "/cli/machine-credentials"
}

func machineCredentialsCurrentURL(apiBase string) string {
	return machineCredentialsURL(apiBase) + "/current"
}

func decodeMachineCredential(body []byte) (*MachineCredential, error) {
	var wire machineCredentialWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("failed to parse machine credential response")
	}
	cred := wire.credential()
	return &cred, nil
}

func sanitizeAPIError(body []byte) string {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if payload.Error != "" {
			return payload.Error
		}
		if payload.Message != "" {
			return payload.Message
		}
	}
	return "request failed"
}

func doCLIAuthJSON(method, endpoint, authorization string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CronitorCLI")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := cliAuthClient().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	b, err := readResponse(resp)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response: %w", err)
	}
	return resp.StatusCode, b, nil
}

func doCLIAuthBasic(method, endpoint, apiKey string, body []byte) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CronitorCLI")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.SetBasicAuth(apiKey, "")
	resp, err := cliAuthClient().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	b, err := readResponse(resp)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read response: %w", err)
	}
	return resp.StatusCode, b, nil
}

// PollInterval converts the device-authorization interval (seconds) to a duration.
func PollInterval(seconds int) time.Duration {
	if seconds <= 0 {
		return DefaultDeviceInterval
	}
	return time.Duration(seconds) * time.Second
}
