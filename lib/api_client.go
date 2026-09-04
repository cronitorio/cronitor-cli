package lib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// BaseURLOverride allows tests to point NewAPIClient at a mock server.
// When non-empty, NewAPIClient uses this instead of the default base URL.
var BaseURLOverride string

// APIClient provides a generic interface for Cronitor API operations
type APIClient struct {
	BaseURL   string
	ApiKey    string
	UserAgent string
	IsDev     bool
	Logger    func(string)
}

// APIResponse wraps the raw response with metadata
type APIResponse struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// PaginatedResponse represents a paginated API response
type PaginatedResponse struct {
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalCount int             `json:"total_count"`
	Data       json.RawMessage `json:"data"`
}

// NewAPIClient creates a new API client with the given configuration
func NewAPIClient(isDev bool, logger func(string)) *APIClient {
	baseURL := "https://cronitor.io/api"
	if BaseURLOverride != "" {
		baseURL = BaseURLOverride
	} else if isDev {
		baseURL = "http://dev.cronitor.io/api"
	}

	return &APIClient{
		BaseURL:   baseURL,
		ApiKey:    viper.GetString("CRONITOR_API_KEY"),
		UserAgent: "CronitorCLI",
		IsDev:     isDev,
		Logger:    logger,
	}
}

// Request makes a generic API request
func (c *APIClient) Request(method, endpoint string, body []byte, queryParams map[string]string) (*APIResponse, error) {
	// Build URL with query parameters
	reqURL := fmt.Sprintf("%s%s", c.BaseURL, endpoint)
	if len(queryParams) > 0 {
		params := url.Values{}
		for k, v := range queryParams {
			if v != "" {
				params.Add(k, v)
			}
		}
		if encoded := params.Encode(); encoded != "" {
			reqURL = fmt.Sprintf("%s?%s", reqURL, encoded)
		}
	}

	c.log(fmt.Sprintf("API Request: %s %s", method, reqURL))

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
		c.log(fmt.Sprintf("Request Body: %s", string(body)))
	}

	req, err := http.NewRequest(method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set authentication
	apiKey := viper.GetString("CRONITOR_API_KEY")
	if apiKey == "" {
		apiKey = c.ApiKey
	}
	req.SetBasicAuth(apiKey, "")

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.UserAgent)
	if apiVersion := viper.GetString("CRONITOR_API_VERSION"); apiVersion != "" {
		req.Header.Set("Cronitor-Version", apiVersion)
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.log(fmt.Sprintf("Response Status: %d", resp.StatusCode))
	c.log(fmt.Sprintf("Response Body: %s", string(respBody)))

	return &APIResponse{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header,
	}, nil
}

// GET makes a GET request
func (c *APIClient) GET(endpoint string, queryParams map[string]string) (*APIResponse, error) {
	return c.Request("GET", endpoint, nil, queryParams)
}

// POST makes a POST request
func (c *APIClient) POST(endpoint string, body []byte, queryParams map[string]string) (*APIResponse, error) {
	return c.Request("POST", endpoint, body, queryParams)
}

// PUT makes a PUT request
func (c *APIClient) PUT(endpoint string, body []byte, queryParams map[string]string) (*APIResponse, error) {
	return c.Request("PUT", endpoint, body, queryParams)
}

// DELETE makes a DELETE request
func (c *APIClient) DELETE(endpoint string, body []byte, queryParams map[string]string) (*APIResponse, error) {
	return c.Request("DELETE", endpoint, body, queryParams)
}

// PATCH makes a PATCH request
func (c *APIClient) PATCH(endpoint string, body []byte, queryParams map[string]string) (*APIResponse, error) {
	return c.Request("PATCH", endpoint, body, queryParams)
}

func (c *APIClient) log(msg string) {
	if c.Logger != nil {
		c.Logger(msg)
	}
}

// IsSuccess returns true if the status code indicates success
func (r *APIResponse) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// IsNotFound returns true if the status code is 404
func (r *APIResponse) IsNotFound() bool {
	return r.StatusCode == 404
}

// FormatJSON pretty-prints the response body as JSON
func (r *APIResponse) FormatJSON() string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, r.Body, "", "  "); err != nil {
		return string(r.Body)
	}
	return buf.String()
}

// ParseError attempts to extract an error message from the response
func (r *APIResponse) ParseError() string {
	// Try to parse as JSON error
	var errResp struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.Unmarshal(r.Body, &errResp); err == nil {
		if errResp.Error != "" {
			// {"error":"invalid_fields","fields":["url"]}: name the fields.
			if fields := stringListField(r.Body, "fields"); len(fields) > 0 {
				return errResp.Error + ": " + strings.Join(fields, ", ")
			}
			return errResp.Error
		}
		if errResp.Message != "" {
			return errResp.Message
		}
		if len(errResp.Errors) > 0 {
			var messages []string
			for _, e := range errResp.Errors {
				messages = append(messages, e.Message)
			}
			return strings.Join(messages, "; ")
		}
	}

	// Django REST Framework validation errors: {"name":["name must be unique"]}
	if msg := parseFieldErrorMap(r.Body); msg != "" {
		return msg
	}

	// Fall back to raw body
	return string(r.Body)
}

// stringListField returns body[key] when it is a JSON array of strings.
func stringListField(body []byte, key string) []string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil {
		return nil
	}
	var list []string
	if json.Unmarshal(obj[key], &list) != nil {
		return nil
	}
	return list
}

// parseFieldErrorMap renders a {field: [messages]} validation body as
// "field: message; field2: message". Keys are sorted for stable output.
func parseFieldErrorMap(body []byte) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || len(obj) == 0 {
		return ""
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		var messages []string
		if json.Unmarshal(obj[k], &messages) == nil && len(messages) > 0 {
			parts = append(parts, k+": "+strings.Join(messages, ", "))
			continue
		}
		var single string
		if json.Unmarshal(obj[k], &single) == nil && single != "" {
			parts = append(parts, k+": "+single)
		}
	}
	return strings.Join(parts, "; ")
}
