package lib_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/internal/testutil"
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// --- API Client Unit Tests ---

func newTestClient(serverURL string) *lib.APIClient {
	return &lib.APIClient{
		BaseURL:   serverURL,
		ApiKey:    "test-api-key-1234567890",
		UserAgent: "CronitorCLI/test",
		IsDev:     false,
		Logger:    nil,
	}
}

func TestAPIClient_GET(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	fixture := testutil.LoadFixture("monitors_list.json")
	mock.On("GET", "/monitors", 200, fixture)

	client := newTestClient(mock.Server.URL)
	resp, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	req := mock.LastRequest()
	if req.Method != "GET" {
		t.Errorf("expected GET, got %s", req.Method)
	}
	if req.Path != "/monitors" {
		t.Errorf("expected /monitors, got %s", req.Path)
	}
}

func TestAPIClient_GET_WithQueryParams(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{"monitors":[]}`)

	client := newTestClient(mock.Server.URL)
	params := map[string]string{
		"page":   "2",
		"type":   "job",
		"env":    "production",
		"search": "backup",
	}
	_, err := client.GET("/monitors", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	for key, expected := range params {
		got := req.QueryParams.Get(key)
		if got != expected {
			t.Errorf("query param %s: expected %q, got %q", key, expected, got)
		}
	}
}

func TestAPIClient_GET_EmptyQueryParamsOmitted(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{"monitors":[]}`)

	client := newTestClient(mock.Server.URL)
	params := map[string]string{
		"page": "1",
		"type": "",
		"env":  "",
	}
	_, err := client.GET("/monitors", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	if req.QueryParams.Get("page") != "1" {
		t.Error("expected page=1")
	}
	// Empty values should not be sent
	if req.QueryParams.Get("type") != "" {
		t.Error("expected empty type param to be omitted")
	}
	if req.QueryParams.Get("env") != "" {
		t.Error("expected empty env param to be omitted")
	}
}

func TestAPIClient_POST(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("POST", "/monitors", 201, `{"key":"new-mon","name":"New Monitor"}`)

	client := newTestClient(mock.Server.URL)
	body := []byte(`{"key":"new-mon","name":"New Monitor","type":"job"}`)
	resp, err := client.POST("/monitors", body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 201 {
		t.Errorf("expected status 201, got %d", resp.StatusCode)
	}

	req := mock.LastRequest()
	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.Body != string(body) {
		t.Errorf("expected body %q, got %q", string(body), req.Body)
	}
}

func TestAPIClient_PUT(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("PUT", "/groups/prod", 200, `{"key":"prod","name":"Updated"}`)

	client := newTestClient(mock.Server.URL)
	body := []byte(`{"name":"Updated"}`)
	resp, err := client.PUT("/groups/prod", body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	req := mock.LastRequest()
	if req.Method != "PUT" {
		t.Errorf("expected PUT, got %s", req.Method)
	}
	if req.Path != "/groups/prod" {
		t.Errorf("expected /groups/prod, got %s", req.Path)
	}
}

func TestAPIClient_DELETE(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("DELETE", "/monitors/abc123", 204, "")

	client := newTestClient(mock.Server.URL)
	resp, err := client.DELETE("/monitors/abc123", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 204 {
		t.Errorf("expected status 204, got %d", resp.StatusCode)
	}

	req := mock.LastRequest()
	if req.Method != "DELETE" {
		t.Errorf("expected DELETE, got %s", req.Method)
	}
}

func TestAPIClient_DELETE_WithBody(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("DELETE", "/monitors", 200, `{"deleted_count":3}`)

	client := newTestClient(mock.Server.URL)
	body := []byte(`{"monitors":["a","b","c"]}`)
	_, err := client.DELETE("/monitors", body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	if req.Body != string(body) {
		t.Errorf("expected body %q, got %q", string(body), req.Body)
	}
}

func TestAPIClient_Authentication(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{}`)

	client := newTestClient(mock.Server.URL)
	client.ApiKey = "my-secret-key"

	// Need to bypass viper for this test - set the key directly
	// The Request method reads from viper first, then falls back to client.ApiKey
	_, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	authHeader := req.Headers.Get("Authorization")
	if authHeader == "" {
		t.Error("expected Authorization header to be set")
	}
	if !strings.HasPrefix(authHeader, "Basic ") {
		t.Errorf("expected Basic auth, got %q", authHeader)
	}
}

func TestAPIClient_Headers(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{}`)

	// Ensure no version is configured
	viper.Set("CRONITOR_API_VERSION", "")
	defer viper.Set("CRONITOR_API_VERSION", "")

	client := newTestClient(mock.Server.URL)
	_, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()

	// Content-Type
	if ct := req.Headers.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	// User-Agent
	if ua := req.Headers.Get("User-Agent"); ua != "CronitorCLI/test" {
		t.Errorf("expected User-Agent CronitorCLI/test, got %q", ua)
	}

	// Cronitor-Version should NOT be sent when no version configured
	if cv := req.Headers.Get("Cronitor-Version"); cv != "" {
		t.Errorf("expected no Cronitor-Version header when unset, got %q", cv)
	}
}

func TestAPIClient_URLConstruction(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors/abc123", 200, `{}`)

	client := newTestClient(mock.Server.URL)
	_, err := client.GET("/monitors/abc123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	if req.Path != "/monitors/abc123" {
		t.Errorf("expected /monitors/abc123, got %s", req.Path)
	}
}

// --- Error Response Tests ---

func TestAPIClient_400_BadRequest(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	errBody := testutil.LoadFixture("error_responses/400.json")
	mock.On("POST", "/monitors", 400, errBody)

	client := newTestClient(mock.Server.URL)
	resp, err := client.POST("/monitors", []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	if resp.IsSuccess() {
		t.Error("expected IsSuccess() to be false")
	}

	parsed := resp.ParseError()
	if !strings.Contains(parsed, "name is required") {
		t.Errorf("expected error message to contain 'name is required', got %q", parsed)
	}
}

func TestAPIClient_403_Forbidden(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	errBody := testutil.LoadFixture("error_responses/403.json")
	mock.On("GET", "/monitors", 403, errBody)

	client := newTestClient(mock.Server.URL)
	resp, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 403 {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	parsed := resp.ParseError()
	if !strings.Contains(parsed, "Invalid API key") {
		t.Errorf("expected 'Invalid API key', got %q", parsed)
	}
}

func TestAPIClient_404_NotFound(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	errBody := testutil.LoadFixture("error_responses/404.json")
	mock.On("GET", "/monitors/nonexistent", 404, errBody)

	client := newTestClient(mock.Server.URL)
	resp, err := client.GET("/monitors/nonexistent", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !resp.IsNotFound() {
		t.Error("expected IsNotFound() to be true")
	}
}

func TestAPIClient_429_RateLimit(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	errBody := testutil.LoadFixture("error_responses/429.json")
	mock.OnWithHeaders("GET", "/monitors", 429, errBody, map[string]string{
		"Retry-After": "30",
	})

	client := newTestClient(mock.Server.URL)
	resp, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 429 {
		t.Errorf("expected 429, got %d", resp.StatusCode)
	}
	if resp.Headers.Get("Retry-After") != "30" {
		t.Errorf("expected Retry-After: 30, got %q", resp.Headers.Get("Retry-After"))
	}
}

func TestAPIClient_500_ServerError(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	errBody := testutil.LoadFixture("error_responses/500.json")
	mock.On("GET", "/monitors", 500, errBody)

	client := newTestClient(mock.Server.URL)
	resp, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != 500 {
		t.Errorf("expected 500, got %d", resp.StatusCode)
	}
	if resp.IsSuccess() {
		t.Error("expected IsSuccess() to be false")
	}

	parsed := resp.ParseError()
	if !strings.Contains(parsed, "Internal server error") {
		t.Errorf("expected 'Internal server error', got %q", parsed)
	}
}

func TestAPIClient_NetworkError(t *testing.T) {
	// Create a server and immediately close it to simulate connection refused
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client := newTestClient(serverURL)
	_, err := client.GET("/monitors", nil)
	if err == nil {
		t.Error("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("expected 'request failed' in error, got %q", err.Error())
	}
}

func TestAPIClient_MalformedJSON(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{not valid json`)

	client := newTestClient(mock.Server.URL)
	resp, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The client should return the raw body, not crash
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// FormatJSON should handle this gracefully
	formatted := resp.FormatJSON()
	if formatted != `{not valid json` {
		t.Errorf("expected raw body on invalid JSON, got %q", formatted)
	}
}

// --- Response Helper Tests ---

func TestAPIResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		code   int
		expect bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{300, false},
		{400, false},
		{404, false},
		{500, false},
	}

	for _, tt := range tests {
		resp := &lib.APIResponse{StatusCode: tt.code}
		if resp.IsSuccess() != tt.expect {
			t.Errorf("IsSuccess() for %d: expected %v", tt.code, tt.expect)
		}
	}
}

func TestAPIResponse_IsNotFound(t *testing.T) {
	tests := []struct {
		code   int
		expect bool
	}{
		{404, true},
		{200, false},
		{403, false},
		{500, false},
	}

	for _, tt := range tests {
		resp := &lib.APIResponse{StatusCode: tt.code}
		if resp.IsNotFound() != tt.expect {
			t.Errorf("IsNotFound() for %d: expected %v", tt.code, tt.expect)
		}
	}
}

func TestAPIResponse_FormatJSON(t *testing.T) {
	resp := &lib.APIResponse{
		Body: []byte(`{"key":"abc","name":"Test"}`),
	}
	formatted := resp.FormatJSON()
	if !strings.Contains(formatted, "  ") {
		t.Error("expected pretty-printed JSON with indentation")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(formatted), &parsed); err != nil {
		t.Errorf("formatted JSON should be valid: %v", err)
	}
}

func TestAPIResponse_ParseError_ErrorField(t *testing.T) {
	resp := &lib.APIResponse{
		Body: []byte(`{"error":"Invalid API key"}`),
	}
	if msg := resp.ParseError(); msg != "Invalid API key" {
		t.Errorf("expected 'Invalid API key', got %q", msg)
	}
}

func TestAPIResponse_ParseError_MessageField(t *testing.T) {
	resp := &lib.APIResponse{
		Body: []byte(`{"message":"Rate limit exceeded"}`),
	}
	if msg := resp.ParseError(); msg != "Rate limit exceeded" {
		t.Errorf("expected 'Rate limit exceeded', got %q", msg)
	}
}

func TestAPIResponse_ParseError_ErrorsArray(t *testing.T) {
	resp := &lib.APIResponse{
		Body: []byte(`{"errors":[{"message":"name is required"},{"message":"type is invalid"}]}`),
	}
	msg := resp.ParseError()
	if !strings.Contains(msg, "name is required") {
		t.Errorf("expected 'name is required' in %q", msg)
	}
	if !strings.Contains(msg, "type is invalid") {
		t.Errorf("expected 'type is invalid' in %q", msg)
	}
}

func TestAPIResponse_ParseError_RawFallback(t *testing.T) {
	resp := &lib.APIResponse{
		Body: []byte(`not json at all`),
	}
	if msg := resp.ParseError(); msg != "not json at all" {
		t.Errorf("expected raw body as fallback, got %q", msg)
	}
}

// --- BaseURLOverride Tests ---

func TestNewAPIClient_DefaultURL(t *testing.T) {
	old := lib.BaseURLOverride
	lib.BaseURLOverride = ""
	defer func() { lib.BaseURLOverride = old }()

	client := lib.NewAPIClient(false, nil)
	if client.BaseURL != "https://cronitor.io/api" {
		t.Errorf("expected default URL, got %s", client.BaseURL)
	}
}

func TestNewAPIClient_DevURL(t *testing.T) {
	old := lib.BaseURLOverride
	lib.BaseURLOverride = ""
	defer func() { lib.BaseURLOverride = old }()

	client := lib.NewAPIClient(true, nil)
	if client.BaseURL != "http://dev.cronitor.io/api" {
		t.Errorf("expected dev URL, got %s", client.BaseURL)
	}
}

func TestNewAPIClient_OverrideURL(t *testing.T) {
	old := lib.BaseURLOverride
	lib.BaseURLOverride = "http://localhost:9999/api"
	defer func() { lib.BaseURLOverride = old }()

	// Override should take priority over both dev and prod
	client := lib.NewAPIClient(false, nil)
	if client.BaseURL != "http://localhost:9999/api" {
		t.Errorf("expected override URL, got %s", client.BaseURL)
	}

	clientDev := lib.NewAPIClient(true, nil)
	if clientDev.BaseURL != "http://localhost:9999/api" {
		t.Errorf("expected override URL even with isDev=true, got %s", clientDev.BaseURL)
	}
}

// --- PaginatedResponse Tests ---

func TestPaginatedResponse_Parse(t *testing.T) {
	body := `{"page":2,"page_size":50,"total_count":150,"data":[{"key":"abc"}]}`
	var paginated lib.PaginatedResponse
	if err := json.Unmarshal([]byte(body), &paginated); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if paginated.Page != 2 {
		t.Errorf("expected page 2, got %d", paginated.Page)
	}
	if paginated.PageSize != 50 {
		t.Errorf("expected page_size 50, got %d", paginated.PageSize)
	}
	if paginated.TotalCount != 150 {
		t.Errorf("expected total_count 150, got %d", paginated.TotalCount)
	}
}

// --- Integration-style tests: full request/response cycle per resource endpoint ---

func TestAPIClient_MonitorEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name     string
		do       func() (*lib.APIResponse, error)
		method   string
		path     string
		wantBody string
	}{
		{
			name:   "list monitors",
			do:     func() (*lib.APIResponse, error) { return client.GET("/monitors", nil) },
			method: "GET",
			path:   "/monitors",
		},
		{
			name:   "get monitor",
			do:     func() (*lib.APIResponse, error) { return client.GET("/monitors/abc123", nil) },
			method: "GET",
			path:   "/monitors/abc123",
		},
		{
			name: "create monitor",
			do: func() (*lib.APIResponse, error) {
				return client.POST("/monitors", []byte(`{"key":"new","type":"job"}`), nil)
			},
			method:   "POST",
			path:     "/monitors",
			wantBody: `{"key":"new","type":"job"}`,
		},
		{
			name: "update monitor (PUT batch)",
			do: func() (*lib.APIResponse, error) {
				return client.PUT("/monitors", []byte(`[{"key":"abc","name":"Updated"}]`), nil)
			},
			method:   "PUT",
			path:     "/monitors",
			wantBody: `[{"key":"abc","name":"Updated"}]`,
		},
		{
			name:   "delete monitor",
			do:     func() (*lib.APIResponse, error) { return client.DELETE("/monitors/abc123", nil, nil) },
			method: "DELETE",
			path:   "/monitors/abc123",
		},
		{
			name: "clone monitor",
			do: func() (*lib.APIResponse, error) {
				return client.POST("/monitors/clone", []byte(`{"key":"abc123"}`), nil)
			},
			method:   "POST",
			path:     "/monitors/clone",
			wantBody: `{"key":"abc123"}`,
		},
		{
			name:   "pause monitor",
			do:     func() (*lib.APIResponse, error) { return client.GET("/monitors/abc123/pause", nil) },
			method: "GET",
			path:   "/monitors/abc123/pause",
		},
		{
			name:   "pause monitor with hours",
			do:     func() (*lib.APIResponse, error) { return client.GET("/monitors/abc123/pause/4", nil) },
			method: "GET",
			path:   "/monitors/abc123/pause/4",
		},
		{
			name:   "unpause monitor",
			do:     func() (*lib.APIResponse, error) { return client.GET("/monitors/abc123/pause/0", nil) },
			method: "GET",
			path:   "/monitors/abc123/pause/0",
		},
		{
			name: "search monitors",
			do: func() (*lib.APIResponse, error) {
				return client.GET("/search", map[string]string{"query": "backup"})
			},
			method: "GET",
			path:   "/search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected method %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected path %s, got %s", tt.path, req.Path)
			}
			if tt.wantBody != "" && req.Body != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, req.Body)
			}
		})
	}
}

func TestAPIClient_GroupEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name   string
		do     func() (*lib.APIResponse, error)
		method string
		path   string
	}{
		{"list groups", func() (*lib.APIResponse, error) { return client.GET("/groups", nil) }, "GET", "/groups"},
		{"get group", func() (*lib.APIResponse, error) { return client.GET("/groups/prod", nil) }, "GET", "/groups/prod"},
		{"create group", func() (*lib.APIResponse, error) {
			return client.POST("/groups", []byte(`{"name":"New"}`), nil)
		}, "POST", "/groups"},
		{"update group", func() (*lib.APIResponse, error) {
			return client.PUT("/groups/prod", []byte(`{"name":"Updated"}`), nil)
		}, "PUT", "/groups/prod"},
		{"delete group", func() (*lib.APIResponse, error) { return client.DELETE("/groups/prod", nil, nil) }, "DELETE", "/groups/prod"},
		{"pause group", func() (*lib.APIResponse, error) { return client.GET("/groups/prod/pause/4", nil) }, "GET", "/groups/prod/pause/4"},
		{"resume group", func() (*lib.APIResponse, error) { return client.GET("/groups/prod/pause/0", nil) }, "GET", "/groups/prod/pause/0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
		})
	}
}

func TestAPIClient_EnvironmentEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name   string
		do     func() (*lib.APIResponse, error)
		method string
		path   string
	}{
		{"list", func() (*lib.APIResponse, error) { return client.GET("/environments", nil) }, "GET", "/environments"},
		{"get", func() (*lib.APIResponse, error) { return client.GET("/environments/prod", nil) }, "GET", "/environments/prod"},
		{"create", func() (*lib.APIResponse, error) {
			return client.POST("/environments", []byte(`{"key":"staging"}`), nil)
		}, "POST", "/environments"},
		{"update", func() (*lib.APIResponse, error) {
			return client.PUT("/environments/staging", []byte(`{"name":"QA"}`), nil)
		}, "PUT", "/environments/staging"},
		{"delete", func() (*lib.APIResponse, error) { return client.DELETE("/environments/old", nil, nil) }, "DELETE", "/environments/old"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
		})
	}
}

func TestAPIClient_NotificationEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name   string
		do     func() (*lib.APIResponse, error)
		method string
		path   string
	}{
		{"list", func() (*lib.APIResponse, error) { return client.GET("/notifications", nil) }, "GET", "/notifications"},
		{"get", func() (*lib.APIResponse, error) { return client.GET("/notifications/default", nil) }, "GET", "/notifications/default"},
		{"create", func() (*lib.APIResponse, error) {
			return client.POST("/notifications", []byte(`{"name":"DevOps"}`), nil)
		}, "POST", "/notifications"},
		{"update", func() (*lib.APIResponse, error) {
			return client.PUT("/notifications/devops", []byte(`{"name":"Updated"}`), nil)
		}, "PUT", "/notifications/devops"},
		{"delete", func() (*lib.APIResponse, error) { return client.DELETE("/notifications/old", nil, nil) }, "DELETE", "/notifications/old"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
		})
	}
}

func TestAPIClient_IssueEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name     string
		do       func() (*lib.APIResponse, error)
		method   string
		path     string
		wantBody string
	}{
		{"list", func() (*lib.APIResponse, error) { return client.GET("/issues", nil) }, "GET", "/issues", ""},
		{"get", func() (*lib.APIResponse, error) { return client.GET("/issues/issue-001", nil) }, "GET", "/issues/issue-001", ""},
		{"create", func() (*lib.APIResponse, error) {
			return client.POST("/issues", []byte(`{"name":"DB issue","severity":"outage"}`), nil)
		}, "POST", "/issues", `{"name":"DB issue","severity":"outage"}`},
		{"update", func() (*lib.APIResponse, error) {
			return client.PUT("/issues/issue-001", []byte(`{"state":"investigating"}`), nil)
		}, "PUT", "/issues/issue-001", `{"state":"investigating"}`},
		{"resolve", func() (*lib.APIResponse, error) {
			return client.PUT("/issues/issue-001", []byte(`{"state":"resolved"}`), nil)
		}, "PUT", "/issues/issue-001", `{"state":"resolved"}`},
		{"delete", func() (*lib.APIResponse, error) { return client.DELETE("/issues/issue-001", nil, nil) }, "DELETE", "/issues/issue-001", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
			if tt.wantBody != "" && req.Body != tt.wantBody {
				t.Errorf("expected body %q, got %q", tt.wantBody, req.Body)
			}
		})
	}
}

func TestAPIClient_MaintenanceEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name   string
		do     func() (*lib.APIResponse, error)
		method string
		path   string
	}{
		{"list", func() (*lib.APIResponse, error) { return client.GET("/maintenance_windows", nil) }, "GET", "/maintenance_windows"},
		{"get", func() (*lib.APIResponse, error) { return client.GET("/maintenance_windows/maint-001", nil) }, "GET", "/maintenance_windows/maint-001"},
		{"create", func() (*lib.APIResponse, error) {
			return client.POST("/maintenance_windows", []byte(`{"name":"Deploy"}`), nil)
		}, "POST", "/maintenance_windows"},
		{"update", func() (*lib.APIResponse, error) {
			return client.PUT("/maintenance_windows/maint-001", []byte(`{"name":"Updated"}`), nil)
		}, "PUT", "/maintenance_windows/maint-001"},
		{"delete", func() (*lib.APIResponse, error) {
			return client.DELETE("/maintenance_windows/maint-001", nil, nil)
		}, "DELETE", "/maintenance_windows/maint-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
		})
	}
}

func TestAPIClient_StatuspageEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name   string
		do     func() (*lib.APIResponse, error)
		method string
		path   string
	}{
		{"list statuspages", func() (*lib.APIResponse, error) { return client.GET("/statuspages", nil) }, "GET", "/statuspages"},
		{"get statuspage", func() (*lib.APIResponse, error) { return client.GET("/statuspages/main", nil) }, "GET", "/statuspages/main"},
		{"create statuspage", func() (*lib.APIResponse, error) {
			return client.POST("/statuspages", []byte(`{"name":"Main"}`), nil)
		}, "POST", "/statuspages"},
		{"update statuspage", func() (*lib.APIResponse, error) {
			return client.PUT("/statuspages/main", []byte(`{"name":"Updated"}`), nil)
		}, "PUT", "/statuspages/main"},
		{"delete statuspage", func() (*lib.APIResponse, error) { return client.DELETE("/statuspages/main", nil, nil) }, "DELETE", "/statuspages/main"},
		{"list components", func() (*lib.APIResponse, error) { return client.GET("/statuspage_components", nil) }, "GET", "/statuspage_components"},
		{"create component", func() (*lib.APIResponse, error) {
			return client.POST("/statuspage_components", []byte(`{"statuspage":"main"}`), nil)
		}, "POST", "/statuspage_components"},
		{"delete component", func() (*lib.APIResponse, error) {
			return client.DELETE("/statuspage_components/comp-001", nil, nil)
		}, "DELETE", "/statuspage_components/comp-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
		})
	}
}

func TestAPIClient_MetricEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	t.Run("get metrics with params", func(t *testing.T) {
		mock.Reset()
		params := map[string]string{
			"monitor":  "abc123",
			"field":    "duration_p50,success_rate",
			"time":     "7d",
			"env":      "production",
			"region":   "us-east-1",
			"withNulls": "true",
		}
		_, err := client.GET("/metrics", params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		req := mock.LastRequest()
		if req.Path != "/metrics" {
			t.Errorf("expected /metrics, got %s", req.Path)
		}
		for k, v := range params {
			if req.QueryParams.Get(k) != v {
				t.Errorf("param %s: expected %q, got %q", k, v, req.QueryParams.Get(k))
			}
		}
	})

	t.Run("get aggregates", func(t *testing.T) {
		mock.Reset()
		_, err := client.GET("/aggregates", map[string]string{"monitor": "abc123"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		req := mock.LastRequest()
		if req.Path != "/aggregates" {
			t.Errorf("expected /aggregates, got %s", req.Path)
		}
	})
}

func TestAPIClient_SiteEndpoints(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	tests := []struct {
		name   string
		do     func() (*lib.APIResponse, error)
		method string
		path   string
	}{
		{"list sites", func() (*lib.APIResponse, error) { return client.GET("/sites", nil) }, "GET", "/sites"},
		{"get site", func() (*lib.APIResponse, error) { return client.GET("/sites/my-site", nil) }, "GET", "/sites/my-site"},
		{"create site", func() (*lib.APIResponse, error) {
			return client.POST("/sites", []byte(`{"name":"My Site"}`), nil)
		}, "POST", "/sites"},
		{"update site", func() (*lib.APIResponse, error) {
			return client.PUT("/sites/my-site", []byte(`{"name":"Updated"}`), nil)
		}, "PUT", "/sites/my-site"},
		{"delete site", func() (*lib.APIResponse, error) { return client.DELETE("/sites/my-site", nil, nil) }, "DELETE", "/sites/my-site"},
		{"query site", func() (*lib.APIResponse, error) {
			return client.POST("/sites/query", []byte(`{"site":"my-site","type":"aggregation"}`), nil)
		}, "POST", "/sites/query"},
		{"list site errors", func() (*lib.APIResponse, error) {
			return client.GET("/site_errors", map[string]string{"site": "my-site"})
		}, "GET", "/site_errors"},
		{"get site error", func() (*lib.APIResponse, error) { return client.GET("/site_errors/err-001", nil) }, "GET", "/site_errors/err-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.Reset()
			_, err := tt.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := mock.LastRequest()
			if req.Method != tt.method {
				t.Errorf("expected %s, got %s", tt.method, req.Method)
			}
			if req.Path != tt.path {
				t.Errorf("expected %s, got %s", tt.path, req.Path)
			}
		})
	}
}

// --- Filter/Query Param Tests ---

func TestAPIClient_MonitorListFilters(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	params := map[string]string{
		"type":    "job,check",
		"group":   "production",
		"tag":     "critical,database",
		"state":   "failing",
		"search":  "backup",
		"sort":    "-created",
		"env":     "production",
		"page":    "2",
		"pageSize": "100",
	}

	_, err := client.GET("/monitors", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	for k, v := range params {
		if req.QueryParams.Get(k) != v {
			t.Errorf("param %s: expected %q, got %q", k, v, req.QueryParams.Get(k))
		}
	}
}

func TestAPIClient_IssueListFilters(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	params := map[string]string{
		"state":    "unresolved",
		"severity": "outage",
		"job":      "my-job",
		"group":    "production",
		"tag":      "critical",
		"env":      "production",
		"search":   "database",
		"time":     "24h",
		"orderBy":  "-started",
	}

	_, err := client.GET("/issues", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	for k, v := range params {
		if req.QueryParams.Get(k) != v {
			t.Errorf("param %s: expected %q, got %q", k, v, req.QueryParams.Get(k))
		}
	}
}

func TestAPIClient_MaintenanceListFilters(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	params := map[string]string{
		"past":                     "true",
		"ongoing":                  "true",
		"upcoming":                 "true",
		"statuspage":               "main",
		"env":                      "production",
		"withAllAffectedMonitors": "true",
	}

	_, err := client.GET("/maintenance_windows", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	for k, v := range params {
		if req.QueryParams.Get(k) != v {
			t.Errorf("param %s: expected %q, got %q", k, v, req.QueryParams.Get(k))
		}
	}
}

func TestAPIClient_StatuspageListFilters(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()
	client := newTestClient(mock.Server.URL)

	params := map[string]string{
		"withStatus":     "true",
		"withComponents": "true",
	}

	_, err := client.GET("/statuspages", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	for k, v := range params {
		if req.QueryParams.Get(k) != v {
			t.Errorf("param %s: expected %q, got %q", k, v, req.QueryParams.Get(k))
		}
	}
}

// --- Phase 5f: Configuration & Version Header Tests ---

func TestVersionHeader_NotSentWhenUnset(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{}`)

	viper.Set("CRONITOR_API_VERSION", "")
	defer viper.Set("CRONITOR_API_VERSION", "")

	client := newTestClient(mock.Server.URL)
	_, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	if cv := req.Headers.Get("Cronitor-Version"); cv != "" {
		t.Errorf("expected no Cronitor-Version header when unset, got %q", cv)
	}
}

func TestVersionHeader_SentWhenConfigured(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{}`)

	viper.Set("CRONITOR_API_VERSION", "2025-11-28")
	defer viper.Set("CRONITOR_API_VERSION", "")

	client := newTestClient(mock.Server.URL)
	_, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	if cv := req.Headers.Get("Cronitor-Version"); cv != "2025-11-28" {
		t.Errorf("expected Cronitor-Version 2025-11-28, got %q", cv)
	}
}

func TestVersionHeader_DifferentVersionValues(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{}`)

	versions := []string{"2020-10-01", "2025-11-28", "2026-01-01"}
	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			mock.Reset()
			viper.Set("CRONITOR_API_VERSION", version)
			defer viper.Set("CRONITOR_API_VERSION", "")

			client := newTestClient(mock.Server.URL)
			_, err := client.GET("/monitors", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := mock.LastRequest()
			if cv := req.Headers.Get("Cronitor-Version"); cv != version {
				t.Errorf("expected Cronitor-Version %q, got %q", version, cv)
			}
		})
	}
}

func TestVersionHeader_AppliesAcrossAllMethods(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.SetDefault(200, `{}`)

	viper.Set("CRONITOR_API_VERSION", "2025-11-28")
	defer viper.Set("CRONITOR_API_VERSION", "")

	client := newTestClient(mock.Server.URL)

	methods := []struct {
		name string
		do   func() (*lib.APIResponse, error)
	}{
		{"GET", func() (*lib.APIResponse, error) { return client.GET("/test", nil) }},
		{"POST", func() (*lib.APIResponse, error) { return client.POST("/test", []byte(`{}`), nil) }},
		{"PUT", func() (*lib.APIResponse, error) { return client.PUT("/test", []byte(`{}`), nil) }},
		{"DELETE", func() (*lib.APIResponse, error) { return client.DELETE("/test", nil, nil) }},
		{"PATCH", func() (*lib.APIResponse, error) { return client.PATCH("/test", []byte(`{}`), nil) }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			mock.Reset()
			_, err := m.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := mock.LastRequest()
			if cv := req.Headers.Get("Cronitor-Version"); cv != "2025-11-28" {
				t.Errorf("%s: expected Cronitor-Version 2025-11-28, got %q", m.name, cv)
			}
		})
	}
}

func TestVersionHeader_NotSentAcrossAllMethods(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.SetDefault(200, `{}`)

	viper.Set("CRONITOR_API_VERSION", "")
	defer viper.Set("CRONITOR_API_VERSION", "")

	client := newTestClient(mock.Server.URL)

	methods := []struct {
		name string
		do   func() (*lib.APIResponse, error)
	}{
		{"GET", func() (*lib.APIResponse, error) { return client.GET("/test", nil) }},
		{"POST", func() (*lib.APIResponse, error) { return client.POST("/test", []byte(`{}`), nil) }},
		{"PUT", func() (*lib.APIResponse, error) { return client.PUT("/test", []byte(`{}`), nil) }},
		{"DELETE", func() (*lib.APIResponse, error) { return client.DELETE("/test", nil, nil) }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			mock.Reset()
			_, err := m.do()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			req := mock.LastRequest()
			if cv := req.Headers.Get("Cronitor-Version"); cv != "" {
				t.Errorf("%s: expected no Cronitor-Version header, got %q", m.name, cv)
			}
		})
	}
}

func TestVersionHeader_ViperPriority_EnvOverridesConfig(t *testing.T) {
	mock := testutil.NewMockAPI()
	defer mock.Close()

	mock.On("GET", "/monitors", 200, `{}`)

	// Simulate config file value
	viper.Set("CRONITOR_API_VERSION", "2020-10-01")
	defer viper.Set("CRONITOR_API_VERSION", "")

	// Env var should override (viper.AutomaticEnv handles this in production;
	// in tests we simulate by setting the viper key directly)
	viper.Set("CRONITOR_API_VERSION", "2025-11-28")

	client := newTestClient(mock.Server.URL)
	_, err := client.GET("/monitors", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := mock.LastRequest()
	if cv := req.Headers.Get("Cronitor-Version"); cv != "2025-11-28" {
		t.Errorf("expected override version 2025-11-28, got %q", cv)
	}
}
