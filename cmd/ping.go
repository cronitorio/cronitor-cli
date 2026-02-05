package cmd

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

var run bool
var complete bool
var fail bool
var tick bool
var ok bool
var msg string
var series string
var pingStatusCode int
var pingDuration float64
var pingMetrics string

var pingCmd = &cobra.Command{
	Use:   "ping <key>",
	Short: "Send a telemetry ping to Cronitor",
	Long: `Send telemetry events to Cronitor monitors.

States:
  --run       Job has started running
  --complete  Job completed successfully
  --fail      Job failed
  --ok        Manually reset monitor to healthy state
  --tick      Send a heartbeat (for heartbeat monitors)

Metrics (for --complete or --fail):
  count:<name>        Event count
  duration:<name>     Duration in seconds
  error_count:<name>  Error count

Examples:
  Report job started:
    cronitor ping d3x0c1 --run

  Report job completed with duration:
    cronitor ping d3x0c1 --complete --duration 45.2

  Report failure with exit code and message:
    cronitor ping d3x0c1 --fail --status-code 1 --msg "Connection refused"

  Send custom metrics:
    cronitor ping d3x0c1 --complete --metric "count:processed=100,error_count:failed=2"

  Correlate run/complete events:
    cronitor ping d3x0c1 --run --series "job-123"
    cronitor ping d3x0c1 --complete --series "job-123" --duration 30.5

  Reset monitor to healthy:
    cronitor ping d3x0c1 --ok

  Send heartbeat:
    cronitor ping d3x0c1 --tick

For full API documentation:
  Humans: https://cronitor.io/docs/telemetry-api
  Agents: https://cronitor.io/docs/telemetry-api.md`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("a unique monitor key is required")
		}

		if len(getEndpointFromFlag()) == 0 {
			return errors.New("a state flag is required (--run, --complete, --fail, --ok, or --tick)")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup
		uniqueIdentifier := args[0]

		// Parse duration if provided
		var duration *float64
		if cmd.Flags().Changed("duration") {
			duration = &pingDuration
		}

		// Parse status code if provided
		var exitCode *int
		if cmd.Flags().Changed("status-code") {
			exitCode = &pingStatusCode
		}

		// Parse metrics if provided
		var metrics map[string]int
		if pingMetrics != "" {
			metrics = parseMetrics(pingMetrics)
		}

		wg.Add(1)
		go sendPing(getEndpointFromFlag(), uniqueIdentifier, msg, series, makeStamp(), duration, exitCode, metrics, "", &wg)
		wg.Wait()
	},
}

func getEndpointFromFlag() string {
	if fail {
		return "fail"
	} else if complete {
		return "complete"
	} else if run {
		return "run"
	} else if tick {
		return "tick"
	} else if ok {
		return "ok"
	}

	return ""
}

// parseMetrics parses metric strings like "count:processed=100,error_count:failed=2"
func parseMetrics(metricStr string) map[string]int {
	metrics := make(map[string]int)
	parts := strings.Split(metricStr, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Format: type:name=value (e.g., count:processed=100)
		eqIdx := strings.LastIndex(part, "=")
		if eqIdx == -1 {
			continue
		}
		key := strings.TrimSpace(part[:eqIdx])
		valStr := strings.TrimSpace(part[eqIdx+1:])
		if val, err := strconv.Atoi(valStr); err == nil {
			metrics[key] = val
		}
	}
	return metrics
}

func init() {
	RootCmd.AddCommand(pingCmd)

	// State flags
	pingCmd.Flags().BoolVar(&run, "run", false, "Report job started")
	pingCmd.Flags().BoolVar(&complete, "complete", false, "Report job completed successfully")
	pingCmd.Flags().BoolVar(&fail, "fail", false, "Report job failed")
	pingCmd.Flags().BoolVar(&ok, "ok", false, "Manually reset monitor to healthy state")
	pingCmd.Flags().BoolVar(&tick, "tick", false, "Send a heartbeat")

	// Data flags
	pingCmd.Flags().StringVar(&msg, "msg", "", "Message to include (max 2000 chars)")
	pingCmd.Flags().StringVar(&series, "series", "", "Unique ID to correlate run/complete events")
	pingCmd.Flags().IntVar(&pingStatusCode, "status-code", 0, "Exit/status code")
	pingCmd.Flags().Float64Var(&pingDuration, "duration", 0, "Execution duration in seconds")
	pingCmd.Flags().StringVar(&pingMetrics, "metric", "", "Custom metrics: type:name=value (comma-separated)")
}
