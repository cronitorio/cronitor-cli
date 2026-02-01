package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	sitePage        int
	sitePageSize    int
	siteFormat      string
	siteOutput      string
	siteWithSnippet bool
	siteData        string
	siteFetchAll    bool
	// Create/Update flags
	siteName        string
	siteWebVitals   bool
	siteErrors      bool
	siteSampling    int
	siteFilterLocal bool
	siteFilterBots  bool
	// Query flags
	siteQueryType     string
	siteQuerySite     string
	siteQueryTime     string
	siteQueryStart    string
	siteQueryEnd      string
	siteQueryMetrics  string
	siteQueryDims     string
	siteQueryGroupBy  string
	siteQueryFilters  string
	siteQueryOrderBy  string
	siteQueryTimezone string
	siteQueryBucket   string
	siteQueryCompare  bool
)

var siteCmd = &cobra.Command{
	Use:   "site",
	Short: "Manage RUM sites",
	Long: `Manage Real User Monitoring (RUM) sites.

Sites collect web performance metrics, Core Web Vitals, and JavaScript errors
from your web applications.

Examples:
  cronitor site list
  cronitor site get my-site
  cronitor site get my-site --with-snippet
  cronitor site create "My Website"
  cronitor site update my-site --sampling 50
  cronitor site delete my-site

  cronitor site errors --site my-site
  cronitor site query --site my-site --type aggregation --metric session_count
  cronitor site query --site my-site --type breakdown --metric lcp_p50 --group-by country_code
  cronitor site query --site my-site --type timeseries --metric session_count --bucket hour

For full API documentation, see https://cronitor.io/docs/sites-api.md`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	RootCmd.AddCommand(siteCmd)
	siteCmd.PersistentFlags().IntVar(&sitePage, "page", 1, "Page number")
	siteCmd.PersistentFlags().IntVar(&sitePageSize, "page-size", 0, "Results per page")
	siteCmd.PersistentFlags().StringVar(&siteFormat, "format", "", "Output format: json, table")
	siteCmd.PersistentFlags().StringVarP(&siteOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var siteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all RUM sites",
	Long: `List all Real User Monitoring sites.

Examples:
  cronitor site list
  cronitor site list --page-size 100`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if sitePage > 1 {
			params["page"] = fmt.Sprintf("%d", sitePage)
		}
		if sitePageSize > 0 {
			params["pageSize"] = fmt.Sprintf("%d", sitePageSize)
		}

		if siteFetchAll {
			bodies, err := FetchAllPages(client, "/sites", params, "data")
			if err != nil {
				Error(fmt.Sprintf("Failed to list sites: %s", err))
				os.Exit(1)
			}
			if siteFormat == "json" || siteFormat == "" {
				siteOutputToTarget(FormatJSON(MergePagedJSON(bodies, "data")))
				return
			}
			// Table: accumulate rows from all pages
			table := &UITable{
				Headers: []string{"NAME", "KEY", "WEB VITALS", "ERRORS", "SAMPLING"},
			}
			for _, body := range bodies {
				var result struct {
					Sites []struct {
						Key              string `json:"key"`
						Name             string `json:"name"`
						ClientKey        string `json:"client_key"`
						WebVitalsEnabled bool   `json:"webvitals_enabled"`
						ErrorsEnabled    bool   `json:"errors_enabled"`
						Sampling         int    `json:"sampling"`
					} `json:"data"`
				}
				json.Unmarshal(body, &result)
				for _, s := range result.Sites {
					webVitals := "off"
					if s.WebVitalsEnabled {
						webVitals = successStyle.Render("on")
					}
					errors := "off"
					if s.ErrorsEnabled {
						errors = successStyle.Render("on")
					}
					sampling := fmt.Sprintf("%d%%", s.Sampling)
					table.Rows = append(table.Rows, []string{s.Name, s.Key, webVitals, errors, sampling})
				}
			}
			siteOutputToTarget(table.Render())
			return
		}

		resp, err := client.GET("/sites", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list sites: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if siteFormat == "json" {
			siteOutputToTarget(FormatJSON(resp.Body))
			return
		}

		var result struct {
			Sites []struct {
				Key              string `json:"key"`
				Name             string `json:"name"`
				ClientKey        string `json:"client_key"`
				WebVitalsEnabled bool   `json:"webvitals_enabled"`
				ErrorsEnabled    bool   `json:"errors_enabled"`
				Sampling         int    `json:"sampling"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		if len(result.Sites) == 0 {
			siteOutputToTarget(mutedStyle.Render("No sites found"))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "WEB VITALS", "ERRORS", "SAMPLING"},
		}

		for _, s := range result.Sites {
			webVitals := "off"
			if s.WebVitalsEnabled {
				webVitals = successStyle.Render("on")
			}
			errors := "off"
			if s.ErrorsEnabled {
				errors = successStyle.Render("on")
			}
			sampling := fmt.Sprintf("%d%%", s.Sampling)
			table.Rows = append(table.Rows, []string{s.Name, s.Key, webVitals, errors, sampling})
		}

		siteOutputToTarget(table.Render())
	},
}

// --- GET ---
var siteGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a RUM site",
	Long: `Get details of a specific RUM site.

Examples:
  cronitor site get my-site
  cronitor site get my-site --with-snippet`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if siteWithSnippet {
			params["withSnippet"] = "true"
		}

		resp, err := client.GET(fmt.Sprintf("/sites/%s", key), params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get site: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Site '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		siteOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var siteCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a RUM site",
	Long: `Create a new Real User Monitoring site.

Examples:
  cronitor site create --data '{"name":"My Website"}'
  cronitor site create --data '{"name":"My App","sampling":50}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if siteData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(siteData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/sites", []byte(siteData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create site: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			Key       string `json:"key"`
			Name      string `json:"name"`
			ClientKey string `json:"client_key"`
		}
		if err := json.Unmarshal(resp.Body, &result); err == nil {
			Success(fmt.Sprintf("Created site: %s (key: %s)", result.Name, result.Key))
			Info(fmt.Sprintf("Client key for browser: %s", result.ClientKey))
		} else {
			Success("Site created")
		}

		if siteFormat == "json" {
			siteOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- UPDATE ---
var siteUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a RUM site",
	Long: `Update settings for a RUM site.

Examples:
  cronitor site update my-site --data '{"name":"New Name"}'
  cronitor site update my-site --data '{"sampling":50}'
  cronitor site update my-site --data '{"webvitals_enabled":false}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if siteData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(siteData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/sites/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update site: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Site '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Site '%s' updated", key))
		if siteFormat == "json" {
			siteOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var siteDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a RUM site",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/sites/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete site: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Site '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Site '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

// --- QUERY ---
var siteQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query RUM analytics data",
	Long: `Query Real User Monitoring analytics data.

Query types:
  aggregation   - Aggregate metrics over time range
  breakdown     - Group metrics by dimension
  timeseries    - Metrics over time with buckets
  error_groups  - Grouped JavaScript error patterns

Available metrics:
  session_count, pageview_count, bounce_rate
  page_load_p50, page_load_p75, page_load_p90, page_load_p99
  lcp_p50, lcp_p75, lcp_p90, lcp_p99 (Largest Contentful Paint)
  fid_p50, fid_p75, fid_p90, fid_p99 (First Input Delay)
  cls_p50, cls_p75, cls_p90, cls_p99 (Cumulative Layout Shift)
  ttfb_p50, ttfb_p75, ttfb_p90, ttfb_p99 (Time to First Byte)

Dimensions for breakdown/filtering:
  country_code, city_name, path, hostname, device_type
  browser, operating_system, referrer_hostname
  utm_source, utm_medium, utm_campaign, connection_type

Time ranges: 1h, 6h, 12h, 24h, 3d, 7d, 14d, 30d, 90d
Time buckets: minute, hour, day, week, month

Examples:
  cronitor site query --site my-site --type aggregation --metric session_count,lcp_p50
  cronitor site query --site my-site --type breakdown --metric session_count --group-by country_code
  cronitor site query --site my-site --type timeseries --metric pageview_count --bucket hour --time 7d
  cronitor site query --site my-site --type breakdown --metric lcp_p50 --group-by browser --filter "device_type:eq:desktop"
  cronitor site query --site my-site --type error_groups --time 24h`,
	Run: func(cmd *cobra.Command, args []string) {
		if siteQuerySite == "" {
			Error("--site is required")
			os.Exit(1)
		}
		if siteQueryType == "" {
			Error("--type is required (aggregation, breakdown, timeseries, error_groups)")
			os.Exit(1)
		}

		payload := map[string]interface{}{
			"site": siteQuerySite,
			"type": siteQueryType,
		}

		// Time range
		if siteQueryTime != "" {
			payload["time"] = siteQueryTime
		} else {
			payload["time"] = "24h"
		}
		if siteQueryStart != "" {
			payload["start"] = siteQueryStart
		}
		if siteQueryEnd != "" {
			payload["end"] = siteQueryEnd
		}
		if siteQueryTimezone != "" {
			payload["timezone"] = siteQueryTimezone
		}

		// Metrics
		if siteQueryMetrics != "" {
			payload["metrics"] = splitAndTrimSite(siteQueryMetrics)
		}

		// Dimensions (for breakdown)
		if siteQueryGroupBy != "" {
			payload["dimensions"] = splitAndTrimSite(siteQueryGroupBy)
		}

		// Time bucket (for timeseries)
		if siteQueryBucket != "" {
			payload["time_bucket"] = siteQueryBucket
		}

		// Filters
		if siteQueryFilters != "" {
			filters := parseFilters(siteQueryFilters)
			if len(filters) > 0 {
				payload["filters"] = filters
			}
		}

		// Order by
		if siteQueryOrderBy != "" {
			payload["order_by"] = splitAndTrimSite(siteQueryOrderBy)
		}

		// Compare
		if siteQueryCompare {
			payload["compare"] = "previous_time_range"
		}

		// Pagination
		if sitePage > 1 {
			payload["page"] = sitePage
		}
		if sitePageSize > 0 {
			payload["page_size"] = sitePageSize
		}

		body, _ := json.Marshal(payload)
		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/sites/query", body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to query site: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		// For query results, JSON is the default since structure varies by query type
		if siteFormat == "table" {
			renderQueryTable(resp.Body, siteQueryType)
		} else {
			siteOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- ERRORS (parent command) ---
var siteErrorsCmd = &cobra.Command{
	Use:   "error",
	Aliases: []string{"errors"},
	Short: "Manage JavaScript errors",
	Long: `Manage JavaScript errors collected from RUM sites.

For grouped error analytics, use: cronitor site query --type error_groups

Examples:
  cronitor site error list --site my-site
  cronitor site error get <error-key>`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// --- ERROR LIST ---
var siteErrorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List JavaScript errors",
	Long: `List JavaScript errors collected from RUM sites.

Examples:
  cronitor site error list --site my-site
  cronitor site error list --site my-site --page-size 100`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if sitePage > 1 {
			params["page"] = fmt.Sprintf("%d", sitePage)
		}
		if sitePageSize > 0 {
			params["pageSize"] = fmt.Sprintf("%d", sitePageSize)
		}

		siteKey, _ := cmd.Flags().GetString("site")
		if siteKey != "" {
			params["site"] = siteKey
		}

		resp, err := client.GET("/site_errors", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list errors: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if siteFormat == "json" {
			siteOutputToTarget(FormatJSON(resp.Body))
			return
		}

		var result struct {
			Errors []struct {
				Key       string `json:"key"`
				Message   string `json:"message"`
				ErrorType string `json:"error_type"`
				Filename  string `json:"filename"`
				Count     int    `json:"count"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		if len(result.Errors) == 0 {
			siteOutputToTarget(mutedStyle.Render("No errors found"))
			return
		}

		table := &UITable{
			Headers: []string{"KEY", "TYPE", "MESSAGE", "FILE", "COUNT"},
		}

		for _, e := range result.Errors {
			msg := e.Message
			if len(msg) > 40 {
				msg = msg[:37] + "..."
			}
			filename := e.Filename
			if len(filename) > 30 {
				filename = "..." + filename[len(filename)-27:]
			}
			table.Rows = append(table.Rows, []string{e.Key, e.ErrorType, msg, filename, fmt.Sprintf("%d", e.Count)})
		}

		siteOutputToTarget(table.Render())
	},
}

// --- ERROR GET ---
var siteErrorGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get error details",
	Long: `Get detailed information about a specific JavaScript error.

Examples:
  cronitor site error get abc123`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/site_errors/%s", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to get error: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Error '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		siteOutputToTarget(FormatJSON(resp.Body))
	},
}

func init() {
	siteCmd.AddCommand(siteListCmd)
	siteCmd.AddCommand(siteGetCmd)
	siteCmd.AddCommand(siteCreateCmd)
	siteCmd.AddCommand(siteUpdateCmd)
	siteCmd.AddCommand(siteDeleteCmd)
	siteCmd.AddCommand(siteQueryCmd)
	siteCmd.AddCommand(siteErrorsCmd)

	// List flags
	siteListCmd.Flags().BoolVar(&siteFetchAll, "all", false, "Fetch all pages of results")

	// Get flags
	siteGetCmd.Flags().BoolVar(&siteWithSnippet, "with-snippet", false, "Include JavaScript installation snippet")

	// Create flags
	siteCreateCmd.Flags().StringVarP(&siteData, "data", "d", "", "JSON payload")

	// Update flags
	siteUpdateCmd.Flags().StringVarP(&siteData, "data", "d", "", "JSON payload")

	// Query flags
	siteQueryCmd.Flags().StringVar(&siteQuerySite, "site", "", "Site key (required)")
	siteQueryCmd.Flags().StringVar(&siteQueryType, "type", "", "Query type: aggregation, breakdown, timeseries, error_groups")
	siteQueryCmd.Flags().StringVar(&siteQueryTime, "time", "24h", "Time range: 1h, 6h, 12h, 24h, 3d, 7d, 14d, 30d, 90d")
	siteQueryCmd.Flags().StringVar(&siteQueryStart, "start", "", "Custom start time (ISO 8601)")
	siteQueryCmd.Flags().StringVar(&siteQueryEnd, "end", "", "Custom end time (ISO 8601)")
	siteQueryCmd.Flags().StringVar(&siteQueryMetrics, "metric", "", "Metrics to return (comma-separated)")
	siteQueryCmd.Flags().StringVar(&siteQueryGroupBy, "group-by", "", "Dimensions to group by (comma-separated)")
	siteQueryCmd.Flags().StringVar(&siteQueryFilters, "filter", "", "Filters: dim:op:value (comma-separated)")
	siteQueryCmd.Flags().StringVar(&siteQueryOrderBy, "order-by", "", "Sort fields (prefix - for desc)")
	siteQueryCmd.Flags().StringVar(&siteQueryTimezone, "timezone", "", "Timezone (IANA format)")
	siteQueryCmd.Flags().StringVar(&siteQueryBucket, "bucket", "", "Time bucket: minute, hour, day, week, month")
	siteQueryCmd.Flags().BoolVar(&siteQueryCompare, "compare", false, "Compare with previous time range")

	// Error subcommands
	siteErrorsCmd.AddCommand(siteErrorListCmd)
	siteErrorsCmd.AddCommand(siteErrorGetCmd)

	// Error list flags
	siteErrorListCmd.Flags().String("site", "", "Filter by site key")
}

func siteOutputToTarget(content string) {
	if siteOutput != "" {
		if err := os.WriteFile(siteOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", siteOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", siteOutput))
	} else {
		fmt.Println(content)
	}
}

func splitAndTrimSite(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseFilters parses filter strings in format "dimension:operator:value"
// e.g., "device_type:eq:desktop,country_code:eq:US"
func parseFilters(filterStr string) []map[string]string {
	filters := []map[string]string{}
	for _, f := range splitAndTrimSite(filterStr) {
		parts := strings.SplitN(f, ":", 3)
		if len(parts) == 3 {
			filters = append(filters, map[string]string{
				"dimension": parts[0],
				"operator":  parts[1],
				"value":     parts[2],
			})
		}
	}
	return filters
}

// renderQueryTable renders query results as a table based on query type
func renderQueryTable(body []byte, queryType string) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		siteOutputToTarget(FormatJSON(body))
		return
	}

	switch queryType {
	case "aggregation":
		renderAggregationTable(result)
	case "breakdown":
		renderBreakdownTable(result)
	case "timeseries":
		renderTimeseriesTable(result)
	case "error_groups":
		renderErrorGroupsTable(result)
	default:
		siteOutputToTarget(FormatJSON(body))
	}
}

func renderAggregationTable(result map[string]interface{}) {
	data, ok := result["data"].(map[string]interface{})
	if !ok {
		fmt.Println("No data")
		return
	}

	table := &UITable{
		Headers: []string{"METRIC", "VALUE"},
	}

	for k, v := range data {
		table.Rows = append(table.Rows, []string{k, formatSiteValue(v)})
	}

	siteOutputToTarget(table.Render())
}

func renderBreakdownTable(result map[string]interface{}) {
	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Println("No data")
		return
	}

	// Get headers from first row
	firstRow, ok := data[0].(map[string]interface{})
	if !ok {
		fmt.Println("Invalid data format")
		return
	}

	headers := []string{}
	for k := range firstRow {
		headers = append(headers, strings.ToUpper(k))
	}

	table := &UITable{
		Headers: headers,
	}

	for _, item := range data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values := []string{}
		for _, h := range headers {
			key := strings.ToLower(h)
			values = append(values, formatSiteValue(row[key]))
		}
		table.Rows = append(table.Rows, values)
	}

	siteOutputToTarget(table.Render())
}

func renderTimeseriesTable(result map[string]interface{}) {
	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Println("No data")
		return
	}

	// Build headers from first row
	firstRow, ok := data[0].(map[string]interface{})
	if !ok {
		fmt.Println("Invalid data format")
		return
	}

	headers := []string{"TIMESTAMP"}
	for k := range firstRow {
		if k != "timestamp" && k != "time" {
			headers = append(headers, strings.ToUpper(k))
		}
	}

	table := &UITable{
		Headers: headers,
	}

	for _, item := range data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values := []string{}
		if ts, ok := row["timestamp"]; ok {
			values = append(values, formatSiteValue(ts))
		} else if ts, ok := row["time"]; ok {
			values = append(values, formatSiteValue(ts))
		} else {
			values = append(values, "-")
		}
		for _, h := range headers[1:] {
			key := strings.ToLower(h)
			values = append(values, formatSiteValue(row[key]))
		}
		table.Rows = append(table.Rows, values)
	}

	siteOutputToTarget(table.Render())
}

func renderErrorGroupsTable(result map[string]interface{}) {
	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Println("No error groups found")
		return
	}

	table := &UITable{
		Headers: []string{"MESSAGE", "TYPE", "COUNT", "FIRST SEEN", "LAST SEEN"},
	}

	for _, item := range data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		msg := formatSiteValue(row["message"])
		if len(msg) > 50 {
			msg = msg[:47] + "..."
		}
		table.Rows = append(table.Rows, []string{
			msg,
			formatSiteValue(row["error_type"]),
			formatSiteValue(row["count"]),
			formatSiteValue(row["first_seen"]),
			formatSiteValue(row["last_seen"]),
		})
	}

	siteOutputToTarget(table.Render())
}

func formatSiteValue(v interface{}) string {
	if v == nil {
		return "-"
	}
	switch val := v.(type) {
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%.2f", val)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}
