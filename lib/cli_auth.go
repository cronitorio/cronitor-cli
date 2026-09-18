package lib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Public WorkOS Connect application defaults.
const (
	DefaultWorkOSClientID   = "client_01M2S35Z13KFG88QDXA7W8A41K"
	DefaultWorkOSAuthKitURL = "https://auth.cronitor.io"

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

// AuthorizationToken is the successful authorization-code grant response. Tokens are
// discarded after the machine-credential bootstrap; they are never persisted.
type AuthorizationToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// Discard zeros token fields so a later log or dump cannot print them.
func (t *AuthorizationToken) Discard() {
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

// WorkOSAuthKitURL is the public AuthKit Connect issuer (no trailing slash).
// The production default is the AuthKit custom domain https://auth.cronitor.io.
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

// WorkOSScope defaults to identity scopes; no refresh token is needed.
func WorkOSScope() string {
	if WorkOSScopeOverride != "" {
		return WorkOSScopeOverride
	}
	if scope := strings.TrimSpace(os.Getenv(envWorkOSScope)); scope != "" {
		return scope
	}
	return "openid profile email"
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
// Only a 2xx response is confirmed revocation. 401 means the key is already
// invalid. 403 (refused) and 404 (missing route or unknown) do not prove delete.
func DeleteCurrentMachineCredential(apiBase, apiKey string) error {
	status, body, err := doCLIAuthBasic(http.MethodDelete, machineCredentialsCurrentURL(apiBase), apiKey, nil)
	if err != nil {
		return err
	}
	if status >= 200 && status < 300 {
		return nil
	}
	if status == http.StatusUnauthorized {
		return &CredentialGoneError{Status: status}
	}
	return &CredentialRevokeUnconfirmedError{Status: status, Detail: sanitizeAPIError(body)}
}

// CredentialGoneError means the stored key is no longer valid remotely.
type CredentialGoneError struct {
	Status int
}

func (e *CredentialGoneError) Error() string {
	return "machine credential is no longer valid"
}

// CredentialRevokeUnconfirmedError means DELETE did not confirm revocation.
// 403 can be a refusal; 404 can mean the route is missing during rollout.
type CredentialRevokeUnconfirmedError struct {
	Status int
	Detail string
}

func (e *CredentialRevokeUnconfirmedError) Error() string {
	if e == nil {
		return "could not revoke machine credential"
	}
	if e.Detail != "" && e.Detail != "request failed" {
		return fmt.Sprintf("could not revoke machine credential (%d): %s", e.Status, e.Detail)
	}
	return fmt.Sprintf("could not revoke machine credential (%d)", e.Status)
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
