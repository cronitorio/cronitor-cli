package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

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
  cronitor monitor list --type job --state failing
  cronitor monitor get <key>
  cronitor monitor create --data '{"key":"my-job","type":"job"}'
  cronitor monitor update <key> --data '{"name":"New Name"}'
  cronitor monitor delete <key>
  cronitor monitor delete key1 key2 key3
  cronitor monitor pause <key>
  cronitor monitor unpause <key>
  cronitor monitor clone <key> --name "Cloned Monitor"
  cronitor monitor search "backup"

For full API documentation:
  Humans: https://cronitor.io/docs/monitors-api
  Agents: https://cronitor.io/docs/monitors-api.md`,
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
	monitorWithEvents      bool
	monitorWithInvocations bool
	monitorPage            int
	monitorPageSize        int
	monitorEnv             string
	monitorFormat          string
	monitorOutput          string
	monitorData            string
	monitorFile            string
	// List filters
	monitorType   []string
	monitorGroup  string
	monitorTag    []string
	monitorState  []string
	monitorSearch string
	monitorSort   string
)

func init() {
	RootCmd.AddCommand(monitorCmd)

	// Persistent flags for all monitor subcommands
	monitorCmd.PersistentFlags().IntVar(&monitorPage, "page", 1, "Page number for paginated results")
	monitorCmd.PersistentFlags().StringVar(&monitorEnv, "env", "", "Filter by environment")
	monitorCmd.PersistentFlags().StringVar(&monitorFormat, "format", "", "Output format: json, table, yaml")
	monitorCmd.PersistentFlags().StringVarP(&monitorOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var monitorListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all monitors",
	Long: `List all monitors in your Cronitor account.

Examples:
  cronitor monitor list
  cronitor monitor list --type job
  cronitor monitor list --type job --type check
  cronitor monitor list --group production
  cronitor monitor list --tag critical --tag database
  cronitor monitor list --state failing
  cronitor monitor list --state failing --state paused
  cronitor monitor list --search backup
  cronitor monitor list --sort name
  cronitor monitor list --sort -created
  cronitor monitor list --page-size 100
  cronitor monitor list --format yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if monitorPage > 1 {
			params["page"] = fmt.Sprintf("%d", monitorPage)
		}
		if monitorPageSize > 0 {
			params["pageSize"] = fmt.Sprintf("%d", monitorPageSize)
		}
		if monitorEnv != "" {
			params["env"] = monitorEnv
		}
		if monitorGroup != "" {
			params["group"] = monitorGroup
		}
		if monitorSearch != "" {
			params["search"] = monitorSearch
		}
		if monitorSort != "" {
			params["sort"] = monitorSort
		}

		// Handle array params by joining with comma (API may need multiple params)
		if len(monitorType) > 0 {
			params["type"] = strings.Join(monitorType, ",")
		}
		if len(monitorTag) > 0 {
			params["tag"] = strings.Join(monitorTag, ",")
		}
		if len(monitorState) > 0 {
			params["state"] = strings.Join(monitorState, ",")
		}
		if monitorWithEvents {
			params["withEvents"] = "true"
		}
		if monitorWithInvocations {
			params["withInvocations"] = "true"
		}

		// Check for YAML format
		format := monitorFormat
		if format == "yaml" {
			params["format"] = "yaml"
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

		// YAML format - output directly
		if format == "yaml" {
			outputToTarget(string(resp.Body))
			return
		}

		// Parse response
		var result struct {
			Monitors []struct {
				Key     string `json:"key"`
				Name    string `json:"name"`
				Type    string `json:"type"`
				Passing bool   `json:"passing"`
				Paused  bool   `json:"paused"`
				Group   string `json:"group"`
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

		if format == "" {
			format = "table"
		}

		if format == "json" {
			outputToTarget(FormatJSON(resp.Body))
			return
		}

		// Table output
		table := &UITable{
			Headers: []string{"NAME", "KEY", "TYPE", "STATUS"},
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
			table.Rows = append(table.Rows, []string{name, m.Key, m.Type, status})
		}

		output := table.Render()
		if result.PageInfo.TotalCount > 0 {
			output += mutedStyle.Render(fmt.Sprintf("\nShowing page %d • %d monitors total",
				result.PageInfo.Page, result.PageInfo.TotalCount))
		}
		outputToTarget(output)
	},
}

// --- EXPORT ---
var monitorExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export all monitors as YAML config",
	Long: `Export all monitors as a complete YAML configuration file.

Fetches all pages of monitors and outputs a single YAML document
suitable for backup or re-import with 'cronitor monitor create'.

Examples:
  cronitor monitor export                              # Print to stdout
  cronitor monitor export -o monitors.yaml             # Save to file
  cronitor monitor export --type job                   # Export only jobs
  cronitor monitor export --group production           # Export one group
  cronitor monitor export -o backup.yaml && cronitor monitor create -f backup.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if monitorEnv != "" {
			params["env"] = monitorEnv
		}
		if monitorGroup != "" {
			params["group"] = monitorGroup
		}
		if len(monitorType) > 0 {
			params["type"] = strings.Join(monitorType, ",")
		}
		if len(monitorTag) > 0 {
			params["tag"] = strings.Join(monitorTag, ",")
		}

		// First, get page 1 as JSON to determine total page count
		resp, err := client.GET("/monitors", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to export monitors: %s", err))
			os.Exit(1)
		}
		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var pageInfo struct {
			PageInfo struct {
				Page       int `json:"page"`
				PageSize   int `json:"pageSize"`
				TotalCount int `json:"totalMonitorCount"`
			} `json:"page_info"`
		}
		json.Unmarshal(resp.Body, &pageInfo)

		totalPages := 1
		if pageInfo.PageInfo.PageSize > 0 && pageInfo.PageInfo.TotalCount > 0 {
			totalPages = (pageInfo.PageInfo.TotalCount + pageInfo.PageInfo.PageSize - 1) / pageInfo.PageInfo.PageSize
		}

		// Now fetch all pages as YAML
		params["format"] = "yaml"
		var combined string
		for page := 1; page <= totalPages; page++ {
			params["page"] = fmt.Sprintf("%d", page)
			resp, err := client.GET("/monitors", params)
			if err != nil {
				Error(fmt.Sprintf("Failed to export monitors (page %d): %s", page, err))
				os.Exit(1)
			}
			if !resp.IsSuccess() {
				Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
				os.Exit(1)
			}
			body := strings.TrimSpace(string(resp.Body))
			if body == "" {
				break
			}
			if combined != "" {
				combined += "\n"
			}
			combined += body
		}

		outputToTarget(combined)
	},
}

// --- SEARCH ---
var monitorSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search monitors",
	Long: `Search monitors using advanced query syntax.

Supported search scopes (use quotes when using colons):
  job:        Search job-type monitors (e.g., "job:backup")
  check:      Search check-type monitors
  heartbeat:  Search heartbeat-type monitors
  group:      Search by group name (e.g., "group:production")
  tag:        Search by tag (e.g., "tag:critical")
  ungrouped:  Find monitors without a group (no value needed)

Examples:
  cronitor monitor search backup                   # Simple text search
  cronitor monitor search "job:backup"             # Search job monitors for "backup"
  cronitor monitor search "group:production"       # Search monitors in "production" group
  cronitor monitor search "tag:critical"           # Search monitors with "critical" tag
  cronitor monitor search "ungrouped:"             # Find all ungrouped monitors
  cronitor monitor search backup --format yaml     # Output results as YAML`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		query := args[0]
		client := lib.NewAPIClient(dev, log)

		params := map[string]string{"query": query}
		if monitorPage > 1 {
			params["page"] = fmt.Sprintf("%d", monitorPage)
		}

		format := monitorFormat
		if format == "yaml" {
			params["format"] = "yaml"
		}

		resp, err := client.GET("/search", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to search: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		// YAML format - output directly
		if format == "yaml" {
			outputToTarget(string(resp.Body))
			return
		}

		if format == "json" || format == "" {
			outputToTarget(FormatJSON(resp.Body))
			return
		}

		// Parse for table output
		var result struct {
			Monitors []struct {
				Key     string `json:"key"`
				Name    string `json:"name"`
				Type    string `json:"type"`
				Passing bool   `json:"passing"`
				Paused  bool   `json:"paused"`
			} `json:"monitors"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			outputToTarget(FormatJSON(resp.Body))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "TYPE", "STATUS"},
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
			table.Rows = append(table.Rows, []string{name, m.Key, m.Type, status})
		}
		outputToTarget(table.Render())
	},
}

// --- GET ---
var monitorGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific monitor",
	Long: `Get details for a specific monitor.

Examples:
  cronitor monitor get my-job
  cronitor monitor get my-job --with-events
  cronitor monitor get my-job --with-invocations`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		params := make(map[string]string)
		if monitorWithEvents {
			params["withEvents"] = "true"
		}
		if monitorWithInvocations {
			params["withInvocations"] = "true"
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
  cronitor monitor create --data '{"key":"my-job","type":"job","schedule":"0 0 * * *"}'
  cronitor monitor create --file monitor.json
  cronitor monitor create --file monitors.yaml
  cat monitor.json | cronitor monitor create`,
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getMonitorRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON/YAML data required. Use --data, --file, or pipe to stdin")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)

		// Check if bulk create (array) or YAML
		var testArray []json.RawMessage
		isBulk := json.Unmarshal(body, &testArray) == nil && len(testArray) > 0

		// Check if YAML (starts with jobs:, checks:, heartbeats:, or sites:)
		bodyStr := strings.TrimSpace(string(body))
		isYAML := strings.HasPrefix(bodyStr, "jobs:") ||
			strings.HasPrefix(bodyStr, "checks:") ||
			strings.HasPrefix(bodyStr, "heartbeats:") ||
			strings.HasPrefix(bodyStr, "sites:")

		var resp *lib.APIResponse
		if isYAML {
			headers := map[string]string{"Content-Type": "application/yaml"}
			resp, err = client.PUT("/monitors", body, headers)
		} else if isBulk {
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
  cronitor monitor update my-job --data '{"schedule":"0 0 * * *","assertions":["metric.duration < 5min"]}'
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

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Monitor '%s' not found", key))
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
	Use:   "delete <key> [keys...]",
	Short: "Delete one or more monitors",
	Long: `Delete one or more monitors.

Examples:
  cronitor monitor delete my-job
  cronitor monitor delete job1 job2 job3`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)

		if len(args) == 1 {
			// Single delete
			key := args[0]
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
		} else {
			// Bulk delete
			body := map[string][]string{"monitors": args}
			bodyJSON, _ := json.Marshal(body)

			resp, err := client.DELETE("/monitors", bodyJSON, nil)
			if err != nil {
				Error(fmt.Sprintf("Failed to delete monitors: %s", err))
				os.Exit(1)
			}

			if !resp.IsSuccess() {
				Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
				os.Exit(1)
			}

			var result struct {
				DeletedCount   int `json:"deleted_count"`
				RequestedCount int `json:"requested_count"`
				Errors         struct {
					Missing []string `json:"missing"`
				} `json:"errors"`
			}
			json.Unmarshal(resp.Body, &result)

			Success(fmt.Sprintf("Deleted %d of %d monitors", result.DeletedCount, result.RequestedCount))
			if len(result.Errors.Missing) > 0 {
				Warning(fmt.Sprintf("Not found: %s", strings.Join(result.Errors.Missing, ", ")))
			}
		}
	},
}

// --- CLONE ---
var monitorCloneName string

var monitorCloneCmd = &cobra.Command{
	Use:   "clone <key>",
	Short: "Clone an existing monitor",
	Long: `Create a copy of an existing monitor.

Examples:
  cronitor monitor clone my-job
  cronitor monitor clone my-job --name "My Job Copy"`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		body := map[string]string{"key": key}
		if monitorCloneName != "" {
			body["name"] = monitorCloneName
		}
		bodyJSON, _ := json.Marshal(body)

		resp, err := client.POST("/monitors/clone", bodyJSON, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to clone monitor: %s", err))
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

		var result struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		json.Unmarshal(resp.Body, &result)

		Success(fmt.Sprintf("Monitor cloned as '%s'", result.Key))
		outputToTarget(FormatJSON(resp.Body))
	},
}

// --- PAUSE ---
var monitorPauseHours string

var monitorPauseCmd = &cobra.Command{
	Use:   "pause <key>",
	Short: "Pause a monitor",
	Long: `Pause a monitor to stop receiving alerts.

For job, heartbeat & site monitors: telemetry is still recorded but no alerts are sent.
For check monitors: outbound requests stop entirely.

Examples:
  cronitor monitor pause my-job            # Pause indefinitely
  cronitor monitor pause my-job --hours 24 # Pause for 24 hours
  cronitor monitor pause my-job --hours 2  # Pause for 2 hours`,
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
	monitorCmd.AddCommand(monitorExportCmd)
	monitorCmd.AddCommand(monitorSearchCmd)
	monitorCmd.AddCommand(monitorGetCmd)
	monitorCmd.AddCommand(monitorCreateCmd)
	monitorCmd.AddCommand(monitorUpdateCmd)
	monitorCmd.AddCommand(monitorDeleteCmd)
	monitorCmd.AddCommand(monitorCloneCmd)
	monitorCmd.AddCommand(monitorPauseCmd)
	monitorCmd.AddCommand(monitorUnpauseCmd)

	// List filters
	monitorListCmd.Flags().StringArrayVar(&monitorType, "type", nil, "Filter by type: job, check, heartbeat, site (can specify multiple)")
	monitorListCmd.Flags().StringVar(&monitorGroup, "group", "", "Filter by group key")
	monitorListCmd.Flags().StringArrayVar(&monitorTag, "tag", nil, "Filter by tag (can specify multiple)")
	monitorListCmd.Flags().StringArrayVar(&monitorState, "state", nil, "Filter by state: passing, failing, paused (can specify multiple)")
	monitorListCmd.Flags().StringVar(&monitorSearch, "search", "", "Search across monitor names and keys")
	monitorListCmd.Flags().IntVar(&monitorPageSize, "page-size", 0, "Number of results per page (default 50)")
	monitorListCmd.Flags().StringVar(&monitorSort, "sort", "", "Sort order: created, -created, name, -name")
	monitorListCmd.Flags().BoolVar(&monitorWithEvents, "with-events", false, "Include latest events for each monitor")
	monitorListCmd.Flags().BoolVar(&monitorWithInvocations, "with-invocations", false, "Include recent invocations for each monitor")

	// Get flags
	monitorGetCmd.Flags().BoolVar(&monitorWithEvents, "with-events", false, "Include latest events")
	monitorGetCmd.Flags().BoolVar(&monitorWithInvocations, "with-invocations", false, "Include recent invocations")

	// Create/Update flags
	monitorCreateCmd.Flags().StringVarP(&monitorData, "data", "d", "", "JSON or YAML data")
	monitorCreateCmd.Flags().StringVarP(&monitorFile, "file", "f", "", "JSON or YAML file")
	monitorUpdateCmd.Flags().StringVarP(&monitorData, "data", "d", "", "JSON data")
	monitorUpdateCmd.Flags().StringVarP(&monitorFile, "file", "f", "", "JSON file")

	// Export filters
	monitorExportCmd.Flags().StringArrayVar(&monitorType, "type", nil, "Filter by type: job, check, heartbeat, site")
	monitorExportCmd.Flags().StringVar(&monitorGroup, "group", "", "Filter by group key")
	monitorExportCmd.Flags().StringArrayVar(&monitorTag, "tag", nil, "Filter by tag")

	// Clone flags
	monitorCloneCmd.Flags().StringVar(&monitorCloneName, "name", "", "Name for the cloned monitor")

	// Pause flags
	monitorPauseCmd.Flags().StringVar(&monitorPauseHours, "hours", "", "Hours to pause (default: indefinite)")
}

// Helper functions
func getMonitorRequestBody() ([]byte, error) {
	if monitorData != "" && monitorFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if monitorData != "" {
		// Try JSON first
		var js json.RawMessage
		if err := json.Unmarshal([]byte(monitorData), &js); err != nil {
			// Might be YAML, return as-is
			return []byte(monitorData), nil
		}
		return []byte(monitorData), nil
	}

	if monitorFile != "" {
		data, err := os.ReadFile(monitorFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
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
