package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Manage monitors",
	Long: `Manage Cronitor monitors (jobs, checks, heartbeats, sites).

Examples:
  cronitor monitor list
  cronitor monitor get <key>
  cronitor monitor create --data '{"key":"my-job","type":"job"}'
  cronitor monitor update <key> --data '{"name":"New Name"}'
  cronitor monitor delete <key>
  cronitor monitor pause <key>
  cronitor monitor unpause <key>`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("API key required. Run 'cronitor configure' or use --api-key flag")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Flags
var (
	monitorWithEvents bool
	monitorPage       int
	monitorEnv        string
	monitorFormat     string
	monitorOutput     string
	monitorData       string
	monitorFile       string
)

func init() {
	RootCmd.AddCommand(monitorCmd)

	// Persistent flags for all monitor subcommands
	monitorCmd.PersistentFlags().IntVar(&monitorPage, "page", 1, "Page number for paginated results")
	monitorCmd.PersistentFlags().StringVar(&monitorEnv, "env", "", "Filter by environment")
	monitorCmd.PersistentFlags().StringVar(&monitorFormat, "format", "", "Output format: json, table (default: table for list, json for get)")
	monitorCmd.PersistentFlags().StringVarP(&monitorOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var monitorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all monitors",
	Long: `List all monitors in your Cronitor account.

Examples:
  cronitor monitor list
  cronitor monitor list --page 2
  cronitor monitor list --env production
  cronitor monitor list --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if monitorPage > 1 {
			params["page"] = fmt.Sprintf("%d", monitorPage)
		}
		if monitorEnv != "" {
			params["env"] = monitorEnv
		}

		resp, err := client.GET("/monitors", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list monitors: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		// Parse response
		var result struct {
			Monitors []struct {
				Key     string `json:"key"`
				Name    string `json:"name"`
				Type    string `json:"type"`
				Passing bool   `json:"passing"`
				Paused  bool   `json:"paused"`
			} `json:"monitors"`
			PageInfo struct {
				Page       int `json:"page"`
				PageSize   int `json:"pageSize"`
				TotalCount int `json:"totalMonitorCount"`
			} `json:"page_info"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		format := monitorFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			outputToTarget(FormatJSON(resp.Body))
			return
		}

		// Table output
		table := &UITable{
			Headers: []string{"KEY", "NAME", "TYPE", "STATUS"},
		}

		for _, m := range result.Monitors {
			name := m.Name
			if name == "" {
				name = m.Key
			}
			status := successStyle.Render("passing")
			if m.Paused {
				status = warningStyle.Render("paused")
			} else if !m.Passing {
				status = errorStyle.Render("failing")
			}
			table.Rows = append(table.Rows, []string{m.Key, name, m.Type, status})
		}

		output := table.Render()
		if result.PageInfo.TotalCount > 0 {
			output += mutedStyle.Render(fmt.Sprintf("\nShowing page %d • %d monitors total",
				result.PageInfo.Page, result.PageInfo.TotalCount))
		}
		outputToTarget(output)
	},
}

// --- GET ---
var monitorGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific monitor",
	Long: `Get details for a specific monitor.

Examples:
  cronitor monitor get my-job
  cronitor monitor get my-job --with-events`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		params := make(map[string]string)
		if monitorWithEvents {
			params["withLatestEvents"] = "true"
		}

		resp, err := client.GET(fmt.Sprintf("/monitors/%s", key), params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get monitor: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Monitor '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		outputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var monitorCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new monitor",
	Long: `Create a new monitor.

Examples:
  cronitor monitor create --data '{"key":"my-job","type":"job"}'
  cronitor monitor create --file monitor.json
  cat monitor.json | cronitor monitor create`,
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getMonitorRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data, --file, or pipe JSON to stdin")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)

		// Check if bulk create (array)
		var testArray []json.RawMessage
		isBulk := json.Unmarshal(body, &testArray) == nil && len(testArray) > 0

		var resp *lib.APIResponse
		if isBulk {
			resp, err = client.PUT("/monitors", body, nil)
		} else {
			resp, err = client.POST("/monitors", body, nil)
		}

		if err != nil {
			Error(fmt.Sprintf("Failed to create monitor: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success("Monitor created")
		outputToTarget(FormatJSON(resp.Body))
	},
}

// --- UPDATE ---
var monitorUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an existing monitor",
	Long: `Update an existing monitor.

Examples:
  cronitor monitor update my-job --data '{"name":"New Name"}'
  cronitor monitor update my-job --file updates.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		body, err := getMonitorRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		// Parse and add key
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(body, &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ = json.Marshal(bodyMap)
		body = []byte(fmt.Sprintf("[%s]", string(body)))

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT("/monitors", body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update monitor: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Monitor '%s' updated", key))
		outputToTarget(FormatJSON(resp.Body))
	},
}

// --- DELETE ---
var monitorDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a monitor",
	Long: `Delete a monitor.

Examples:
  cronitor monitor delete my-job`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/monitors/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete monitor: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Monitor '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Monitor '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

// --- PAUSE ---
var monitorPauseHours string

var monitorPauseCmd = &cobra.Command{
	Use:   "pause <key>",
	Short: "Pause a monitor",
	Long: `Pause a monitor to stop receiving alerts.

Examples:
  cronitor monitor pause my-job            # Pause indefinitely
  cronitor monitor pause my-job --hours 24 # Pause for 24 hours`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		endpoint := fmt.Sprintf("/monitors/%s/pause", key)
		if monitorPauseHours != "" && monitorPauseHours != "0" {
			endpoint = fmt.Sprintf("%s/%s", endpoint, monitorPauseHours)
		}

		resp, err := client.GET(endpoint, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to pause monitor: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Monitor '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			if monitorPauseHours != "" && monitorPauseHours != "0" {
				Success(fmt.Sprintf("Monitor '%s' paused for %s hours", key, monitorPauseHours))
			} else {
				Success(fmt.Sprintf("Monitor '%s' paused", key))
			}
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

// --- UNPAUSE ---
var monitorUnpauseCmd = &cobra.Command{
	Use:   "unpause <key>",
	Short: "Unpause a monitor",
	Long: `Unpause a monitor to resume receiving alerts.

Examples:
  cronitor monitor unpause my-job`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/monitors/%s/pause/0", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to unpause monitor: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Monitor '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Monitor '%s' unpaused", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

func init() {
	monitorCmd.AddCommand(monitorListCmd)
	monitorCmd.AddCommand(monitorGetCmd)
	monitorCmd.AddCommand(monitorCreateCmd)
	monitorCmd.AddCommand(monitorUpdateCmd)
	monitorCmd.AddCommand(monitorDeleteCmd)
	monitorCmd.AddCommand(monitorPauseCmd)
	monitorCmd.AddCommand(monitorUnpauseCmd)

	// Get flags
	monitorGetCmd.Flags().BoolVar(&monitorWithEvents, "with-events", false, "Include latest events")

	// Create/Update flags
	monitorCreateCmd.Flags().StringVarP(&monitorData, "data", "d", "", "JSON data")
	monitorCreateCmd.Flags().StringVarP(&monitorFile, "file", "f", "", "JSON file")
	monitorUpdateCmd.Flags().StringVarP(&monitorData, "data", "d", "", "JSON data")
	monitorUpdateCmd.Flags().StringVarP(&monitorFile, "file", "f", "", "JSON file")

	// Pause flags
	monitorPauseCmd.Flags().StringVar(&monitorPauseHours, "hours", "", "Hours to pause (default: indefinite)")
}

// Helper functions
func getMonitorRequestBody() ([]byte, error) {
	if monitorData != "" && monitorFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if monitorData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(monitorData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(monitorData), nil
	}

	if monitorFile != "" {
		data, err := os.ReadFile(monitorFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			return nil, fmt.Errorf("invalid JSON in file: %w", err)
		}
		return data, nil
	}

	// Try stdin
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := os.ReadFile("/dev/stdin")
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
		if len(data) > 0 {
			var js json.RawMessage
			if err := json.Unmarshal(data, &js); err != nil {
				return nil, fmt.Errorf("invalid JSON from stdin: %w", err)
			}
			return data, nil
		}
	}

	return nil, nil
}

func outputToTarget(content string) {
	if monitorOutput != "" {
		if err := os.WriteFile(monitorOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", monitorOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", monitorOutput))
	} else {
		fmt.Println(content)
	}
}
