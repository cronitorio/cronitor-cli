package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var metricsStart string
var metricsEnd string
var metricsGroup string
var metricsAggregates bool

var apiMetricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "View monitor metrics and performance data",
	Long: `
View monitor metrics and performance data.

The metrics API provides detailed time-series metrics data for your monitors,
including performance statistics, success rates, and execution counts.

Examples:
  Get metrics for all monitors:
  $ cronitor api metrics

  Get metrics for a specific monitor:
  $ cronitor api metrics --monitor <key>

  Get metrics with time range:
  $ cronitor api metrics --monitor <key> --start 2024-01-01 --end 2024-01-31

  Get metrics for a group:
  $ cronitor api metrics --group <group-name>

  Get aggregated statistics (summary without time-series):
  $ cronitor api metrics --aggregates

  Get aggregates for a specific monitor:
  $ cronitor api metrics --aggregates --monitor <key>

  Filter by environment:
  $ cronitor api metrics --monitor <key> --env production

  Output as table:
  $ cronitor api metrics --monitor <key> --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()

		if metricsAggregates {
			getAggregates(client)
		} else {
			getMetrics(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiMetricsCmd)
	apiMetricsCmd.Flags().StringVar(&metricsStart, "start", "", "Start date/time for metrics (e.g., 2024-01-01)")
	apiMetricsCmd.Flags().StringVar(&metricsEnd, "end", "", "End date/time for metrics (e.g., 2024-01-31)")
	apiMetricsCmd.Flags().StringVar(&metricsGroup, "group", "", "Filter by monitor group")
	apiMetricsCmd.Flags().BoolVar(&metricsAggregates, "aggregates", false, "Get aggregated statistics instead of time-series")
}

func getMetrics(client *lib.APIClient) {
	params := buildQueryParams()
	if metricsStart != "" {
		params["start"] = metricsStart
	}
	if metricsEnd != "" {
		params["end"] = metricsEnd
	}
	if metricsGroup != "" {
		params["group"] = metricsGroup
	}

	resp, err := client.GET("/metrics", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get metrics: %s", err), 1)
	}

	outputResponse(resp, []string{"Monitor", "Metric", "Value", "Timestamp"},
		func(data []byte) [][]string {
			var result struct {
				Metrics []struct {
					Monitor   string  `json:"monitor"`
					Metric    string  `json:"metric"`
					Value     float64 `json:"value"`
					Timestamp string  `json:"timestamp"`
				} `json:"metrics"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Metrics))
			for i, m := range result.Metrics {
				rows[i] = []string{m.Monitor, m.Metric, fmt.Sprintf("%.2f", m.Value), m.Timestamp}
			}
			return rows
		})
}

func getAggregates(client *lib.APIClient) {
	params := buildQueryParams()
	if metricsStart != "" {
		params["start"] = metricsStart
	}
	if metricsEnd != "" {
		params["end"] = metricsEnd
	}
	if metricsGroup != "" {
		params["group"] = metricsGroup
	}

	resp, err := client.GET("/aggregates", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get aggregates: %s", err), 1)
	}

	outputResponse(resp, []string{"Monitor", "Total Runs", "Successes", "Failures", "Avg Duration", "Success Rate"},
		func(data []byte) [][]string {
			var result struct {
				Aggregates []struct {
					Monitor     string  `json:"monitor"`
					TotalRuns   int     `json:"total_runs"`
					Successes   int     `json:"successes"`
					Failures    int     `json:"failures"`
					AvgDuration float64 `json:"avg_duration"`
					SuccessRate float64 `json:"success_rate"`
				} `json:"aggregates"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Aggregates))
			for i, a := range result.Aggregates {
				rows[i] = []string{
					a.Monitor,
					fmt.Sprintf("%d", a.TotalRuns),
					fmt.Sprintf("%d", a.Successes),
					fmt.Sprintf("%d", a.Failures),
					fmt.Sprintf("%.2fs", a.AvgDuration),
					fmt.Sprintf("%.1f%%", a.SuccessRate*100),
				}
			}
			return rows
		})
}
