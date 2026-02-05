package testutil

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// RecordedRequest captures details of an incoming HTTP request for assertion.
type RecordedRequest struct {
	Method      string
	Path        string
	QueryParams url.Values
	Headers     http.Header
	Body        string
}

// MockAPI is a test HTTP server that records requests and returns configurable responses.
type MockAPI struct {
	Server        *httptest.Server
	mu            sync.Mutex
	Requests      []RecordedRequest
	routes        map[string]mockResponse
	defaultStatus int
	defaultBody   string
}

type mockResponse struct {
	status  int
	body    string
	headers map[string]string
}

// NewMockAPI creates a new mock API server.
func NewMockAPI() *MockAPI {
	m := &MockAPI{
		routes:        make(map[string]mockResponse),
		defaultStatus: 200,
		defaultBody:   `{}`,
	}

	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		m.mu.Lock()
		m.Requests = append(m.Requests, RecordedRequest{
			Method:      r.Method,
			Path:        r.URL.Path,
			QueryParams: r.URL.Query(),
			Headers:     r.Header.Clone(),
			Body:        string(bodyBytes),
		})
		m.mu.Unlock()

		// Find matching route
		key := r.Method + " " + r.URL.Path
		if resp, ok := m.routes[key]; ok {
			for k, v := range resp.headers {
				w.Header().Set(k, v)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.status)
			w.Write([]byte(resp.body))
			return
		}

		// Try wildcard match (METHOD *)
		wildcardKey := r.Method + " *"
		if resp, ok := m.routes[wildcardKey]; ok {
			for k, v := range resp.headers {
				w.Header().Set(k, v)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.status)
			w.Write([]byte(resp.body))
			return
		}

		// Default response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(m.defaultStatus)
		w.Write([]byte(m.defaultBody))
	}))

	return m
}

// On registers a response for a specific method + path.
func (m *MockAPI) On(method, path string, status int, body string) {
	m.routes[method+" "+path] = mockResponse{status: status, body: body}
}

// OnWithHeaders registers a response with custom headers.
func (m *MockAPI) OnWithHeaders(method, path string, status int, body string, headers map[string]string) {
	m.routes[method+" "+path] = mockResponse{status: status, body: body, headers: headers}
}

// SetDefault sets the default response for unmatched routes.
func (m *MockAPI) SetDefault(status int, body string) {
	m.defaultStatus = status
	m.defaultBody = body
}

// LastRequest returns the most recent recorded request.
func (m *MockAPI) LastRequest() RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Requests) == 0 {
		return RecordedRequest{}
	}
	return m.Requests[len(m.Requests)-1]
}

// RequestCount returns the number of recorded requests.
func (m *MockAPI) RequestCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Requests)
}

// Reset clears all recorded requests.
func (m *MockAPI) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = nil
}

// Close shuts down the mock server.
func (m *MockAPI) Close() {
	m.Server.Close()
}

// TestdataDir returns the path to the testdata directory at the project root.
func TestdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "testdata")
}

// LoadFixture reads a JSON fixture file from testdata/.
func LoadFixture(name string) string {
	data, err := os.ReadFile(filepath.Join(TestdataDir(), name))
	if err != nil {
		panic("failed to load fixture " + name + ": " + err.Error())
	}
	return string(data)
}
