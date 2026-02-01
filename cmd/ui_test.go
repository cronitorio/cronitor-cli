package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// MergePagedJSON
// ---------------------------------------------------------------------------

func TestMergePagedJSON_TwoPages(t *testing.T) {
	pages := [][]byte{
		[]byte(`{"items":[{"id":1}]}`),
		[]byte(`{"items":[{"id":2}]}`),
	}

	result := MergePagedJSON(pages, "items")

	var items []map[string]interface{}
	if err := json.Unmarshal(result, &items); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	if items[0]["id"].(float64) != 1 {
		t.Errorf("expected first item id=1, got %v", items[0]["id"])
	}
	if items[1]["id"].(float64) != 2 {
		t.Errorf("expected second item id=2, got %v", items[1]["id"])
	}
}

func TestMergePagedJSON_EmptyPages(t *testing.T) {
	result := MergePagedJSON([][]byte{}, "items")

	// json.MarshalIndent of nil slice produces "null"
	if string(result) != "null" {
		t.Errorf("expected null for empty pages, got %s", string(result))
	}
}

func TestMergePagedJSON_SinglePage(t *testing.T) {
	pages := [][]byte{
		[]byte(`{"items":[{"id":1},{"id":2}]}`),
	}

	result := MergePagedJSON(pages, "items")

	var items []map[string]interface{}
	if err := json.Unmarshal(result, &items); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestMergePagedJSON_MismatchedKey(t *testing.T) {
	pages := [][]byte{
		[]byte(`{"monitors":[{"id":1}]}`),
		[]byte(`{"monitors":[{"id":2}]}`),
	}

	result := MergePagedJSON(pages, "items")

	// Key "items" does not exist, so no items are collected → nil slice → "null"
	if string(result) != "null" {
		t.Errorf("expected null for mismatched key, got %s", string(result))
	}
}

// ---------------------------------------------------------------------------
// FetchAllPages
// ---------------------------------------------------------------------------

func TestFetchAllPages_StopsOnEmptyItems(t *testing.T) {
	viper.Set("CRONITOR_API_KEY", "test-key")
	defer viper.Set("CRONITOR_API_KEY", "")

	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		page := r.URL.Query().Get("page")
		var body string
		switch page {
		case "", "1":
			body = `{"items":[{"id":1}]}`
		case "2":
			body = `{"items":[{"id":2}]}`
		default:
			body = `{"items":[]}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(body))
	}))
	defer server.Close()

	client := &lib.APIClient{
		BaseURL:   server.URL,
		ApiKey:    "test-key",
		UserAgent: "test",
	}

	pages, err := FetchAllPages(client, "/monitors", nil, "items")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Pages 1, 2, and 3 are fetched; page 3 is the empty one that stops iteration.
	// The empty page body is still included in the returned slice.
	if len(pages) != 3 {
		t.Fatalf("expected 3 pages, got %d", len(pages))
	}

	// Verify that request count matches: pages 1, 2, 3
	if int(requestCount.Load()) != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount.Load())
	}
}

func TestFetchAllPages_PageQueryParamsIncrement(t *testing.T) {
	viper.Set("CRONITOR_API_KEY", "test-key")
	defer viper.Set("CRONITOR_API_KEY", "")

	var receivedPages []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		receivedPages = append(receivedPages, page)

		var body string
		switch page {
		case "", "1":
			body = `{"items":[{"id":1}]}`
		case "2":
			body = `{"items":[{"id":2}]}`
		default:
			body = `{"items":[]}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(body))
	}))
	defer server.Close()

	client := &lib.APIClient{
		BaseURL:   server.URL,
		ApiKey:    "test-key",
		UserAgent: "test",
	}

	_, err := FetchAllPages(client, "/monitors", nil, "items")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect page params: "1", "2", "3"
	if len(receivedPages) != 3 {
		t.Fatalf("expected 3 page requests, got %d: %v", len(receivedPages), receivedPages)
	}

	for i, expected := range []string{"1", "2", "3"} {
		if receivedPages[i] != expected {
			t.Errorf("request %d: expected page=%s, got page=%s", i, expected, receivedPages[i])
		}
	}
}

func TestFetchAllPages_SafetyLimitAt200(t *testing.T) {
	viper.Set("CRONITOR_API_KEY", "test-key")
	defer viper.Set("CRONITOR_API_KEY", "")

	var requestCount atomic.Int32

	// Server that always returns non-empty items
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		page := r.URL.Query().Get("page")
		body := fmt.Sprintf(`{"items":[{"id":%s}]}`, page)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(body))
	}))
	defer server.Close()

	client := &lib.APIClient{
		BaseURL:   server.URL,
		ApiKey:    "test-key",
		UserAgent: "test",
	}

	pages, err := FetchAllPages(client, "/monitors", nil, "items")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(pages) != 200 {
		t.Errorf("expected safety limit of 200 pages, got %d", len(pages))
	}

	if int(requestCount.Load()) != 200 {
		t.Errorf("expected 200 requests, got %d", requestCount.Load())
	}
}
