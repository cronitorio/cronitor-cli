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
	metricFormat    string
	metricOutput    string
	metricMonitors  string
	metricGroups    string
	metricTags      string
	metricTypes     string
	metricTime      string
	metricStart     int64
	metricEnd       int64
	metricEnv       string
	metricRegions   string
	metricFields    string
	metricWithNulls bool
)

var metricCmd = &cobra.Command{
	Use:     "metric",
	Aliases: []string{"metrics"},
	Short:   "Query monitor metrics and aggregates",
	Long: `Query performance metrics and aggregated data for monitors.

Metrics provides time-series data points while aggregates provide summarized statistics.

Time ranges:
  1h, 6h, 12h, 24h, 3d, 7d, 14d, 30d, 90d, 180d, 365d

Available fields:
  Performance: duration_p10, duration_p50, duration_p90, duration_p99, duration_mean, success_rate
  Counts: run_count, complete_count, fail_count, tick_count, alert_count
  Checks: checks_healthy_count, checks_triggered_count, checks_failed_count

Examples:
  cronitor metric get --monitor my-job --field duration_p50,success_rate
  cronitor metric get --group production --time 7d --field run_count,fail_count
  cronitor metric aggregate --monitor my-job --time 30d
  cronitor metric aggregate --tag critical --env production

For full API documentation:
  Humans: https://cronitor.io/docs/metrics-api
  Agents: https://cronitor.io/docs/metrics-api.md`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	RootCmd.AddCommand(metricCmd)
	metricCmd.PersistentFlags().StringVar(&metricFormat, "format", "", "Output format: json, table")
	metricCmd.PersistentFlags().StringVarP(&metricOutput, "output", "o", "", "Write output to file")
}

// --- GET (time-series metrics) ---
var metricGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get time-series metrics",
	Long: `Get time-series metrics data for monitors.

You must specify at least one --field parameter.

Examples:
  cronitor metric get --monitor my-job --field duration_p50
  cronitor metric get --monitor my-job --field duration_p50,duration_p90,success_rate --time 7d
  cronitor metric get --group production --field run_count,fail_count
  cronitor metric get --tag critical --time 30d --field success_rate`,
	Run: func(cmd *cobra.Command, args []string) {
		if metricFields == "" {
			Error("At least one --field is required")
			Info("Available fields: duration_p10, duration_p50, duration_p90, duration_p99, duration_mean, success_rate, run_count, complete_count, fail_count, tick_count, alert_count")
			os.Exit(1)
		}

		if metricMonitors == "" && metricGroups == "" && metricTags == "" {
			Error("At least one of --monitor, --group, or --tag is required")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		params := buildMetricParams()

		// Add fields
		params["field"] = metricFields

		resp, err := client.GET("/metrics", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get metrics: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if metricFormat == "json" || metricFormat == "" {
			metricOutputToTarget(FormatJSON(resp.Body))
			return
		}

		// Parse and display as table
		var result struct {
			Monitors map[string]map[string][]map[string]interface{} `json:"monitors"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		if len(result.Monitors) == 0 {
			metricOutputToTarget(mutedStyle.Render("No metrics found"))
			return
		}

		// Build table with dynamic columns based on fields
		fields := splitAndTrimMetric(metricFields)
		headers := []string{"MONITOR", "ENV", "TIMESTAMP"}
		headers = append(headers, fields...)

		table := &UITable{
			Headers: headers,
		}

		for monitorKey, envData := range result.Monitors {
			for envKey, dataPoints := range envData {
				for _, dp := range dataPoints {
					row := []string{monitorKey, envKey}
					if stamp, ok := dp["stamp"].(float64); ok {
						row = append(row, fmt.Sprintf("%.0f", stamp))
					} else {
						row = append(row, "-")
					}
					for _, f := range fields {
						if val, ok := dp[f]; ok {
							row = append(row, formatMetricValue(val))
						} else {
							row = append(row, "-")
						}
					}
					table.Rows = append(table.Rows, row)
				}
			}
		}

		metricOutputToTarget(table.Render())
	},
}

// --- AGGREGATE ---
var metricAggregateCmd = &cobra.Command{
	Use:     "aggregate",
	Aliases: []string{"agg"},
	Short:   "Get aggregated metrics",
	Long: `Get aggregated statistics for monitors.

Returns summarized metrics like mean duration, success rate, total runs, and uptime.

Examples:
  cronitor metric aggregate --monitor my-job
  cronitor metric aggregate --monitor my-job --time 30d
  cronitor metric aggregate --group production --env production
  cronitor metric aggregate --tag critical`,
	Run: func(cmd *cobra.Command, args []string) {
		if metricMonitors == "" && metricGroups == "" && metricTags == "" {
			Error("At least one of --monitor, --group, or --tag is required")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		params := buildMetricParams()

		resp, err := client.GET("/aggregates", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get aggregates: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if metricFormat == "json" || metricFormat == "" {
			metricOutputToTarget(FormatJSON(resp.Body))
			return
		}

		// Parse and display as table
		var result struct {
			Monitors map[string]map[string]map[string]interface{} `json:"monitors"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		if len(result.Monitors) == 0 {
			metricOutputToTarget(mutedStyle.Render("No aggregates found"))
			return
		}

		table := &UITable{
			Headers: []string{"MONITOR", "ENV", "SUCCESS RATE", "MEAN DURATION", "P50", "P90", "RUNS", "FAILURES"},
		}

		for monitorKey, envData := range result.Monitors {
			for envKey, agg := range envData {
				row := []string{monitorKey, envKey}

				if sr, ok := agg["success_rate"]; ok {
					row = append(row, formatMetricValue(sr)+"%")
				} else {
					row = append(row, "-")
				}
				if dm, ok := agg["duration_mean"]; ok {
					row = append(row, formatMetricValue(dm)+"ms")
				} else {
					row = append(row, "-")
				}
				if p50, ok := agg["duration_p50"]; ok {
					row = append(row, formatMetricValue(p50)+"ms")
				} else {
					row = append(row, "-")
				}
				if p90, ok := agg["duration_p90"]; ok {
					row = append(row, formatMetricValue(p90)+"ms")
				} else {
					row = append(row, "-")
				}
				if runs, ok := agg["total_runs"]; ok {
					row = append(row, formatMetricValue(runs))
				} else {
					row = append(row, "-")
				}
				if fails, ok := agg["total_failures"]; ok {
					row = append(row, formatMetricValue(fails))
				} else {
					row = append(row, "-")
				}

				table.Rows = append(table.Rows, row)
			}
		}

		metricOutputToTarget(table.Render())
	},
}

func init() {
	metricCmd.AddCommand(metricGetCmd)
	metricCmd.AddCommand(metricAggregateCmd)

	// Shared flags for both commands
	for _, cmd := range []*cobra.Command{metricGetCmd, metricAggregateCmd} {
		cmd.Flags().StringVar(&metricMonitors, "monitor", "", "Monitor keys (comma-separated)")
		cmd.Flags().StringVar(&metricGroups, "group", "", "Group keys (comma-separated)")
		cmd.Flags().StringVar(&metricTags, "tag", "", "Tag names (comma-separated)")
		cmd.Flags().StringVar(&metricTypes, "type", "", "Monitor types: job, check, event, heartbeat (comma-separated)")
		cmd.Flags().StringVar(&metricTime, "time", "24h", "Time range: 1h, 6h, 12h, 24h, 3d, 7d, 14d, 30d, 90d, 180d, 365d")
		cmd.Flags().Int64Var(&metricStart, "start", 0, "Custom start time (Unix timestamp)")
		cmd.Flags().Int64Var(&metricEnd, "end", 0, "Custom end time (Unix timestamp)")
		cmd.Flags().StringVar(&metricEnv, "env", "", "Environment key")
		cmd.Flags().StringVar(&metricRegions, "region", "", "Regions (comma-separated)")
		cmd.Flags().BoolVar(&metricWithNulls, "with-nulls", false, "Include null values for missing data points")
	}

	// Field flag only for get command
	metricGetCmd.Flags().StringVar(&metricFields, "field", "", "Metric fields to return (comma-separated, required)")
}

func buildMetricParams() map[string]string {
	params := make(map[string]string)

	// Note: For multiple values, pass comma-separated to the CLI
	// The API accepts repeated params but our client uses map[string]string
	// so we pass the first value for each. Use comma-separated for multiple.
	if metricMonitors != "" {
		params["monitor"] = metricMonitors
	}
	if metricGroups != "" {
		params["group"] = metricGroups
	}
	if metricTags != "" {
		params["tag"] = metricTags
	}
	if metricTypes != "" {
		params["type"] = metricTypes
	}
	if metricStart > 0 {
		params["start"] = fmt.Sprintf("%d", metricStart)
	}
	if metricEnd > 0 {
		params["end"] = fmt.Sprintf("%d", metricEnd)
	}
	if metricStart == 0 && metricEnd == 0 && metricTime != "" {
		params["time"] = metricTime
	}
	if metricEnv != "" {
		params["env"] = metricEnv
	}
	if metricRegions != "" {
		params["region"] = metricRegions
	}
	if metricWithNulls {
		params["withNulls"] = "true"
	}

	return params
}

func splitAndTrimMetric(s string) []string {
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

func formatMetricValue(v interface{}) string {
	switch val := v.(type) {
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%.0f", val)
		}
		return fmt.Sprintf("%.2f", val)
	case int:
		return fmt.Sprintf("%d", val)
	case nil:
		return "-"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func metricOutputToTarget(content string) {
	if metricOutput != "" {
		if err := os.WriteFile(metricOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", metricOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", metricOutput))
	} else {
		fmt.Println(content)
	}
}
