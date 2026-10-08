package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
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
	// Create/Update flags
	siteName        string
	siteWebVitals   bool
	siteErrors      bool
	siteSampling    int
	siteFilterLocal bool
	siteFilterBots  bool
	// Query flags
	siteQueryType            string
	siteQuerySite            string
	siteQueryTime            string
	siteQueryStart           string
	siteQueryEnd             string
	siteQueryMetrics         string
	siteQueryAggregate       string
	siteQueryGroupBy         string
	siteQueryFilters         string
	siteQueryOrderBy         string
	siteQueryTimezone        string
	siteQueryBucket          string
	siteQueryCompare         bool
	siteQueryKind            string
	siteQueryEnvironment     string
	siteQueryFiltersBehavior string
	siteQuerySearch          string
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
  cronitor site query --site my-site --type breakdown --metric web_vital_lcp_p50 --group-by country_code
  cronitor site query --site my-site --type timeseries --metric session_count --bucket hour

For full API documentation:
  Humans: https://cronitor.io/docs/sites-api
  Agents: https://cronitor.io/docs/sites-api.md`,
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

The POST /api/sites/query body uses the Sites API names: kind, aggregate
(a list), and group_by (one string). --type is an alias for kind and
--metric is an alias for aggregate. This command does not send type,
metrics, or dimensions.

Query kinds:
  aggregation     Aggregate metrics over the time range
  breakdown       Group metrics by one dimension (--group-by is required)
  timeseries      Metrics over time (--bucket is time_bucket)
  error_groups    Grouped JavaScript errors. Rows are the group key plus requested aggregates; with no --metric only group keys are returned
  search_options  Find dimension values. Pass --metric and --search (an empty --search is valid)

Aggregate names are whatever the server currently accepts. Names that are
valid on the API today include session_count, pageview_count, and
web_vital_lcp_p50. Short names such as lcp_p50 are not server fields.
The CLI forwards any aggregate you pass; it does not keep its own enum.

group_by / filter dimensions include country_code, path, browser, and
device_type. Pass exactly one --group-by value.
Filter operators: eq, ne, gt, gte, lt, lte, startsWith, endsWith, contains.

Time ranges: 1h, 24h, today, 3d, 7d, 14d, 30d, 180d, 4w, mtd, 3m, 6m, 12m, 24m, ytd, custom
  custom also requires --start and --end (ISO 8601). Timezone is IANA; the server defaults to UTC.

Timeseries buckets allowed for a time range:
  1h                         minute
  24h, today                 hour
  3d, 7d, 14d                hour, day
  30d, 4w, mtd               hour, day, week
  180d, 3m, 6m, 12m, 24m, ytd  day, week, month
  custom                     hour, day, week, month

Examples:
  cronitor site query --site my-site --type aggregation --metric session_count,web_vital_lcp_p50
  cronitor site query --site my-site --type breakdown --metric session_count --group-by country_code
  cronitor site query --site my-site --type timeseries --metric pageview_count --bucket hour --time 7d
  cronitor site query --site my-site --type breakdown --metric web_vital_lcp_p50 --group-by browser --filter "device_type:eq:desktop"
  cronitor site query --site my-site --type error_groups --metric message,error_type,error_count,last_seen --time 24h`,
	Run: func(cmd *cobra.Command, args []string) {
		kind, err := siteQueryKindFromFlags(cmd)
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		metrics, err := siteQueryMetricsFromFlags(cmd)
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}

		payload, err := buildSiteQueryPayload(siteQueryInput{
			Site:            siteQuerySite,
			Kind:            kind,
			Metrics:         metrics,
			GroupBy:         siteQueryGroupBy,
			Time:            siteQueryTime,
			Start:           siteQueryStart,
			End:             siteQueryEnd,
			Timezone:        siteQueryTimezone,
			Bucket:          siteQueryBucket,
			Filters:         siteQueryFilters,
			OrderBy:         siteQueryOrderBy,
			Environment:     siteQueryEnvironment,
			FiltersBehavior: siteQueryFiltersBehavior,
			Search:          siteQuerySearch,
			SearchSet:       cmd.Flags().Changed("search"),
			Compare:         siteQueryCompare,
			Page:            sitePage,
			PageSize:        sitePageSize,
		})
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}

		body, err := json.Marshal(payload)
		if err != nil {
			Error(fmt.Sprintf("Failed to encode query: %s", err))
			os.Exit(1)
		}
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

		// JSON is the default because the response shape depends on kind.
		if siteFormat == "table" {
			aggregates, _ := payload["aggregate"].([]string)
			groupBy, _ := payload["group_by"].(string)
			renderQueryTable(resp.Body, kind, aggregates, groupBy)
		} else {
			siteOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- ERRORS (parent command) ---
var siteErrorsCmd = &cobra.Command{
	Use:     "error",
	Aliases: []string{"errors"},
	Short:   "Manage JavaScript errors",
	Long: `Manage JavaScript errors collected from RUM sites.

For grouped error analytics, use: cronitor site query --site my-site --type error_groups --metric message,error_type,error_count,last_seen

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

	// Get flags
	siteGetCmd.Flags().BoolVar(&siteWithSnippet, "with-snippet", false, "Include JavaScript installation snippet")

	// Create flags
	siteCreateCmd.Flags().StringVarP(&siteData, "data", "d", "", "JSON payload")

	// Update flags
	siteUpdateCmd.Flags().StringVarP(&siteData, "data", "d", "", "JSON payload")

	// Query flags. --type/--metric stay as the flags agents already pass.
	// --kind/--aggregate are the Sites API names for the same values.
	siteQueryCmd.Flags().StringVar(&siteQuerySite, "site", "", "Site key (required)")
	siteQueryCmd.Flags().StringVar(&siteQueryType, "type", "", "Query kind: aggregation, breakdown, timeseries, error_groups, search_options")
	siteQueryCmd.Flags().StringVar(&siteQueryKind, "kind", "", "Alias for --type (Sites API field name)")
	siteQueryCmd.Flags().StringVar(&siteQueryTime, "time", "24h", "Time range: 1h, 24h, today, 3d, 7d, 14d, 30d, 180d, 4w, mtd, 3m, 6m, 12m, 24m, ytd, custom")
	siteQueryCmd.Flags().StringVar(&siteQueryStart, "start", "", "Custom range start (ISO 8601, with --time custom)")
	siteQueryCmd.Flags().StringVar(&siteQueryEnd, "end", "", "Custom range end (ISO 8601, with --time custom)")
	siteQueryCmd.Flags().StringVar(&siteQueryMetrics, "metric", "", "Aggregate metrics, comma-separated (sent as aggregate)")
	siteQueryCmd.Flags().StringVar(&siteQueryAggregate, "aggregate", "", "Alias for --metric (Sites API field name)")
	siteQueryCmd.Flags().StringVar(&siteQueryGroupBy, "group-by", "", "Single group_by dimension (required for breakdown)")
	siteQueryCmd.Flags().StringVar(&siteQueryFilters, "filter", "", "Filters: dimension:operator:value (comma-separated)")
	siteQueryCmd.Flags().StringVar(&siteQueryOrderBy, "order-by", "", "Sort fields, comma-separated (prefix - for desc)")
	siteQueryCmd.Flags().StringVar(&siteQueryTimezone, "timezone", "", "IANA timezone (server default UTC)")
	siteQueryCmd.Flags().StringVar(&siteQueryBucket, "bucket", "", "Timeseries time_bucket: minute, hour, day, week, month")
	siteQueryCmd.Flags().BoolVar(&siteQueryCompare, "compare", false, "Set compare to previous_time_range")
	siteQueryCmd.Flags().StringVar(&siteQueryEnvironment, "environment", "", "Environment key")
	siteQueryCmd.Flags().StringVar(&siteQueryFiltersBehavior, "filters-behavior", "", "Combine filters: and, or")
	siteQueryCmd.Flags().StringVar(&siteQuerySearch, "search", "", "Search term (search_options)")

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

// siteQueryInput is the resolved CLI query. buildSiteQueryPayload maps it
// onto RUMQuerySerializer field names. Metric and dimension values are not
// checked against a client-side enum; the server accepts new aggregates
// (for example those added after this CLI release) without a CLI change.
type siteQueryInput struct {
	Site            string
	Kind            string
	Metrics         string
	GroupBy         string
	Time            string
	Start           string
	End             string
	Timezone        string
	Bucket          string
	Filters         string
	OrderBy         string
	Environment     string
	FiltersBehavior string
	Search          string
	SearchSet       bool
	Compare         bool
	Page            int
	PageSize        int
}

func siteQueryKindFromFlags(cmd *cobra.Command) (string, error) {
	typeSet := cmd.Flags().Changed("type")
	kindSet := cmd.Flags().Changed("kind")
	switch {
	case typeSet && kindSet && siteQueryType != siteQueryKind:
		return "", fmt.Errorf("--type %q and --kind %q disagree; pass only one", siteQueryType, siteQueryKind)
	case kindSet:
		return siteQueryKind, nil
	case typeSet:
		return siteQueryType, nil
	default:
		return "", fmt.Errorf("--type is required (aggregation, breakdown, timeseries, error_groups, search_options); --kind is an alias")
	}
}

func siteQueryMetricsFromFlags(cmd *cobra.Command) (string, error) {
	metricSet := cmd.Flags().Changed("metric")
	aggSet := cmd.Flags().Changed("aggregate")
	switch {
	case metricSet && aggSet && siteQueryMetrics != siteQueryAggregate:
		return "", fmt.Errorf("--metric %q and --aggregate %q disagree; pass only one", siteQueryMetrics, siteQueryAggregate)
	case aggSet:
		return siteQueryAggregate, nil
	default:
		return siteQueryMetrics, nil
	}
}

func buildSiteQueryPayload(in siteQueryInput) (map[string]interface{}, error) {
	site := strings.TrimSpace(in.Site)
	if site == "" {
		return nil, fmt.Errorf("--site is required")
	}
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		return nil, fmt.Errorf("--type is required (aggregation, breakdown, timeseries, error_groups, search_options); --kind is an alias")
	}

	groupBy, err := singleSiteGroupBy(in.GroupBy)
	if err != nil {
		return nil, err
	}
	if kind == "breakdown" && groupBy == "" {
		return nil, fmt.Errorf("--group-by is required when kind is breakdown (one dimension, for example country_code)")
	}

	// aggregate is required by the serializer. An omitted --metric is an empty
	// list rather than a missing field. Names are forwarded as given; this
	// client does not keep a closed list of aggregate values.
	aggregate := []string{}
	if strings.TrimSpace(in.Metrics) != "" {
		aggregate = splitAndTrimSite(in.Metrics)
	}
	if kind == "search_options" {
		// RUMSearchOptionsQuery accepts search="" (the flag must be present).
		// It still requires at least one search field in aggregate.
		if !in.SearchSet {
			return nil, fmt.Errorf("--search is required when kind is search_options")
		}
		if len(aggregate) == 0 {
			return nil, fmt.Errorf("--metric is required when kind is search_options (one or more search fields)")
		}
	}

	payload := map[string]interface{}{
		"site":      site,
		"kind":      kind,
		"aggregate": aggregate,
	}
	timeRange := strings.TrimSpace(in.Time)
	if timeRange == "custom" && (strings.TrimSpace(in.Start) == "" || strings.TrimSpace(in.End) == "") {
		return nil, fmt.Errorf("--start and --end are required when --time is custom")
	}
	if timeRange != "" {
		payload["time"] = timeRange
	}
	if v := strings.TrimSpace(in.Start); v != "" {
		payload["start"] = v
	}
	if v := strings.TrimSpace(in.End); v != "" {
		payload["end"] = v
	}
	if v := strings.TrimSpace(in.Timezone); v != "" {
		payload["timezone"] = v
	}
	if v := strings.TrimSpace(in.Bucket); v != "" {
		payload["time_bucket"] = v
	}
	if groupBy != "" {
		payload["group_by"] = groupBy
	}
	if strings.TrimSpace(in.Filters) != "" {
		filters, err := parseFilters(in.Filters)
		if err != nil {
			return nil, err
		}
		if len(filters) > 0 {
			payload["filters"] = filters
		}
	}
	if strings.TrimSpace(in.OrderBy) != "" {
		payload["order_by"] = splitAndTrimSite(in.OrderBy)
	}
	if in.Compare {
		payload["compare"] = "previous_time_range"
	}
	if v := strings.TrimSpace(in.Environment); v != "" {
		payload["environment"] = v
	}
	if v := strings.TrimSpace(in.FiltersBehavior); v != "" {
		payload["filters_behavior"] = v
	}
	if in.SearchSet {
		// Include "" when the flag was passed. The server treats that as a
		// search, distinct from omitting the field.
		payload["search"] = strings.TrimSpace(in.Search)
	}
	if in.Page > 1 {
		payload["page"] = in.Page
	}
	if in.PageSize > 0 {
		payload["page_size"] = in.PageSize
	}
	return payload, nil
}

func singleSiteGroupBy(raw string) (string, error) {
	parts := splitAndTrimSite(raw)
	if len(parts) > 1 {
		return "", fmt.Errorf("--group-by accepts a single dimension, got %s; the Sites API group_by field is one string, not a list", strings.Join(parts, ", "))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], nil
}

// parseFilters parses filter strings in format "dimension:operator:value"
// e.g., "device_type:eq:desktop,country_code:eq:US"
func parseFilters(filterStr string) ([]map[string]string, error) {
	filters := []map[string]string{}
	for _, f := range splitAndTrimSite(filterStr) {
		parts := strings.SplitN(f, ":", 3)
		if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid --filter %q; expected dimension:operator:value", f)
		}
		filters = append(filters, map[string]string{
			"dimension": strings.TrimSpace(parts[0]),
			"operator":  strings.TrimSpace(parts[1]),
			"value":     parts[2],
		})
	}
	return filters, nil
}

// renderQueryTable renders query results as a table based on query kind.
// aggregates and groupBy are the request fields, in the order the user asked
// for them, so columns follow that order instead of Go map iteration.
func renderQueryTable(body []byte, queryType string, aggregates []string, groupBy string) {
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		siteOutputToTarget(FormatJSON(body))
		return
	}

	switch queryType {
	case "aggregation":
		renderAggregationTable(result)
	case "breakdown":
		renderBreakdownTable(result, groupBy, aggregates)
	case "timeseries":
		renderTimeseriesTable(result)
	case "error_groups":
		renderErrorGroupsTable(result, aggregates)
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

	keys := make([]string, 0, len(data))
	showPrevious := false
	for k, v := range data {
		keys = append(keys, k)
		if m, ok := v.(map[string]interface{}); ok {
			if _, has := m["previous_time_range_value"]; has {
				showPrevious = true
			}
		}
	}
	sort.Strings(keys)

	headers := []string{"METRIC", "VALUE"}
	if showPrevious {
		headers = append(headers, "PREVIOUS", "CHANGE")
	}
	table := &UITable{Headers: headers}
	for _, k := range keys {
		value, previous, change := formatAggregateCell(data[k])
		row := []string{k, value}
		if showPrevious {
			row = append(row, previous, change)
		}
		table.Rows = append(table.Rows, row)
	}

	siteOutputToTarget(table.Render())
}

// formatAggregateCell reads an aggregation metric. The API returns
// {"session_count":{"value":0}} and, with compare, previous_time_range_*.
func formatAggregateCell(v interface{}) (value, previous, change string) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return formatSiteValue(v), "-", "-"
	}
	if _, has := m["value"]; !has {
		return formatSiteValue(v), "-", "-"
	}
	value = formatSiteValue(m["value"])
	if prev, has := m["previous_time_range_value"]; has {
		previous = formatSiteValue(prev)
	} else {
		previous = "-"
	}
	if rate, has := m["previous_time_range_change_rate"]; has {
		change = formatSiteValue(rate)
	} else {
		change = "-"
	}
	return value, previous, change
}

func renderBreakdownTable(result map[string]interface{}, groupBy string, aggregates []string) {
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

	headers := breakdownHeaders(firstRow, groupBy, aggregates)

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
			values = append(values, formatQueryCell(row[key]))
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

	metricHeaders := []string{}
	for k := range firstRow {
		if k != "timestamp" && k != "ts" && k != "time" {
			metricHeaders = append(metricHeaders, strings.ToUpper(k))
		}
	}
	sort.Strings(metricHeaders)
	headers := append([]string{"TIMESTAMP"}, metricHeaders...)

	table := &UITable{
		Headers: headers,
	}

	for _, item := range data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values := []string{timeseriesStamp(row)}
		for _, h := range headers[1:] {
			key := strings.ToLower(h)
			values = append(values, formatQueryCell(row[key]))
		}
		table.Rows = append(table.Rows, values)
	}

	siteOutputToTarget(table.Render())
}

func breakdownHeaders(firstRow map[string]interface{}, groupBy string, aggregates []string) []string {
	seen := map[string]bool{}
	keys := make([]string, 0, len(firstRow))
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		keys = append(keys, name)
	}
	// group_by, then the aggregates in the order they were requested.
	add(groupBy)
	for _, name := range aggregates {
		add(name)
	}
	rest := make([]string, 0, len(firstRow))
	for key := range firstRow {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	keys = append(keys, rest...)

	headers := make([]string, len(keys))
	for i, key := range keys {
		headers[i] = strings.ToUpper(key)
	}
	return headers
}

func renderErrorGroupsTable(result map[string]interface{}, aggregates []string) {
	data, ok := result["data"].([]interface{})
	if !ok || len(data) == 0 {
		fmt.Println("No error groups found")
		return
	}

	// Rows are {key, <requested aggregates>}. key is the error group, not an
	// aggregate the caller lists in --metric.
	columns := []string{"key"}
	seen := map[string]bool{"key": true}
	for _, name := range aggregates {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		columns = append(columns, name)
	}
	headers := make([]string, len(columns))
	for i, column := range columns {
		headers[i] = strings.ToUpper(column)
	}

	table := &UITable{Headers: headers}
	for _, item := range data {
		row, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		values := make([]string, len(columns))
		for i, column := range columns {
			values[i] = formatQueryCell(row[column])
		}
		table.Rows = append(table.Rows, values)
	}

	siteOutputToTarget(table.Render())
}

func timeseriesStamp(row map[string]interface{}) string {
	for _, key := range []string{"timestamp", "ts", "time"} {
		if ts, ok := row[key]; ok {
			return formatQueryCell(ts)
		}
	}
	return "-"
}

func formatQueryCell(v interface{}) string {
	if m, ok := v.(map[string]interface{}); ok {
		if val, has := m["value"]; has {
			return formatSiteValue(val)
		}
	}
	return formatSiteValue(v)
}

func formatSiteValue(v interface{}) string {
	if v == nil {
		return "-"
	}
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return strconv.FormatFloat(val, 'f', 0, 64)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case string:
		return val
	default:
		return fmt.Sprintf("%v", val)
	}
}
