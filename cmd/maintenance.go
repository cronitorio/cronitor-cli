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
	maintenancePage        int
	maintenanceFormat      string
	maintenanceOutput      string
	maintenancePast        bool
	maintenanceOngoing     bool
	maintenanceUpcoming    bool
	maintenanceStatuspage  string
	maintenanceEnv         string
	maintenanceWithMonitors bool
	// Create flags
	maintenanceName        string
	maintenanceDesc        string
	maintenanceStart       string
	maintenanceEnd         string
	maintenanceMonitors    string
	maintenanceGroups      string
	maintenanceStatuspages string
	maintenanceAllMonitors bool
	// Data flags
	maintenanceData        string
	maintenanceFile        string
)

var maintenanceCmd = &cobra.Command{
	GroupID: GroupAPI,
	Use:     "maintenance",
	Aliases: []string{"maint"},
	Short:   "Manage maintenance windows",
	Long: `Manage maintenance windows.

Maintenance windows suppress alerts for monitors during scheduled maintenance periods.

Examples:
  cronitor maintenance list
  cronitor maintenance list --ongoing
  cronitor maintenance list --upcoming
  cronitor maintenance get <key>
  cronitor maintenance create "Deploy v2.0" --start "2024-01-15T02:00:00Z" --end "2024-01-15T04:00:00Z"
  cronitor maintenance create "DB Migration" --start "2024-01-20T00:00:00Z" --end "2024-01-20T02:00:00Z" --monitors "db-job,db-check"
  cronitor maintenance delete <key>

For full API documentation:
  Humans: https://cronitor.io/docs/maintenance-windows-api
  Agents: https://cronitor.io/docs/maintenance-windows-api.md`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	RootCmd.AddCommand(maintenanceCmd)
	maintenanceCmd.PersistentFlags().IntVar(&maintenancePage, "page", 1, "Page number")
	maintenanceCmd.PersistentFlags().StringVar(&maintenanceFormat, "format", "", "Output format: json, table")
	maintenanceCmd.PersistentFlags().StringVarP(&maintenanceOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var maintenanceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List maintenance windows",
	Long: `List maintenance windows.

Examples:
  cronitor maintenance list
  cronitor maintenance list --ongoing
  cronitor maintenance list --upcoming
  cronitor maintenance list --past
  cronitor maintenance list --statuspage my-page
  cronitor maintenance list --env production`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if maintenancePage > 1 {
			params["page"] = fmt.Sprintf("%d", maintenancePage)
		}
		if maintenancePast {
			params["past"] = "true"
		}
		if maintenanceOngoing {
			params["ongoing"] = "true"
		}
		if maintenanceUpcoming {
			params["upcoming"] = "true"
		}
		if maintenanceStatuspage != "" {
			params["statuspage"] = maintenanceStatuspage
		}
		if maintenanceEnv != "" {
			params["env"] = maintenanceEnv
		}
		if maintenanceWithMonitors {
			params["withAllAffectedMonitors"] = "true"
		}

		resp, err := client.GET("/maintenance_windows", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list maintenance windows: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if maintenanceFormat == "json" {
			maintenanceOutputToTarget(FormatJSON(resp.Body))
			return
		}

		var result struct {
			Windows []struct {
				Key      string `json:"key"`
				Name     string `json:"name"`
				Start    string `json:"start"`
				End      string `json:"end"`
				State    string `json:"state"`
				Duration int    `json:"duration"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		if len(result.Windows) == 0 {
			maintenanceOutputToTarget(mutedStyle.Render("No maintenance windows found"))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "START", "END", "STATE"},
		}

		for _, w := range result.Windows {
			state := w.State
			switch state {
			case "ongoing":
				state = warningStyle.Render("ongoing")
			case "upcoming":
				state = mutedStyle.Render("upcoming")
			case "past":
				state = successStyle.Render("completed")
			}

			start := ""
			if len(w.Start) >= 16 {
				start = w.Start[:16]
			}
			end := ""
			if len(w.End) >= 16 {
				end = w.End[:16]
			}

			table.Rows = append(table.Rows, []string{w.Name, w.Key, start, end, state})
		}

		maintenanceOutputToTarget(table.Render())
	},
}

// --- GET ---
var maintenanceGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a maintenance window",
	Long: `Get details of a specific maintenance window.

Examples:
  cronitor maintenance get <key>
  cronitor maintenance get <key> --with-monitors`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if maintenanceWithMonitors {
			params["withAllAffectedMonitors"] = "true"
		}

		resp, err := client.GET(fmt.Sprintf("/maintenance_windows/%s", key), params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get maintenance window: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Maintenance window '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		maintenanceOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var maintenanceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a maintenance window",
	Long: `Create a new maintenance window.

Times should be in ISO 8601 format (e.g., "2024-01-15T02:00:00Z").

Examples:
  cronitor maintenance create --data '{"name":"Deploy v2.0","start":"2024-01-15T02:00:00Z","end":"2024-01-15T04:00:00Z"}'
  cronitor maintenance create --data '{"name":"DB Migration","start":"2024-01-20T00:00:00Z","end":"2024-01-20T02:00:00Z","monitors":["db-job","db-check"]}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if maintenanceData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(maintenanceData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/maintenance_windows", []byte(maintenanceData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create maintenance window: %s", err))
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
		if err := json.Unmarshal(resp.Body, &result); err == nil {
			Success(fmt.Sprintf("Created maintenance window: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Maintenance window created")
		}

		if maintenanceFormat == "json" {
			maintenanceOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- UPDATE ---
var maintenanceUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a maintenance window",
	Long: `Update an existing maintenance window.

Use --data to provide a JSON payload with the fields to update.

Examples:
  cronitor maintenance update my-window --data '{"name":"New Name"}'
  cronitor maintenance update my-window --data '{"start":"2024-01-15T03:00:00Z","end":"2024-01-15T05:00:00Z"}'
  cronitor maintenance update my-window --file update.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		body, err := getMaintenanceRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}

		if body == nil {
			Error("Update data required. Use --data or --file")
			os.Exit(1)
		}

		// Inject key into body
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			bodyMap["key"] = key
			body, _ = json.Marshal(bodyMap)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/maintenance_windows/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update maintenance window: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Maintenance window '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Maintenance window '%s' updated", key))
		if maintenanceFormat == "json" {
			maintenanceOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var maintenanceDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a maintenance window",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/maintenance_windows/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete maintenance window: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Maintenance window '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Maintenance window '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

func init() {
	maintenanceCmd.AddCommand(maintenanceListCmd)
	maintenanceCmd.AddCommand(maintenanceGetCmd)
	maintenanceCmd.AddCommand(maintenanceCreateCmd)
	maintenanceCmd.AddCommand(maintenanceUpdateCmd)
	maintenanceCmd.AddCommand(maintenanceDeleteCmd)

	// List flags
	maintenanceListCmd.Flags().BoolVar(&maintenancePast, "past", false, "Include past windows")
	maintenanceListCmd.Flags().BoolVar(&maintenanceOngoing, "ongoing", false, "Show only ongoing windows")
	maintenanceListCmd.Flags().BoolVar(&maintenanceUpcoming, "upcoming", false, "Show only upcoming windows")
	maintenanceListCmd.Flags().StringVar(&maintenanceStatuspage, "statuspage", "", "Filter by status page key")
	maintenanceListCmd.Flags().StringVar(&maintenanceEnv, "env", "", "Filter by environment")
	maintenanceListCmd.Flags().BoolVar(&maintenanceWithMonitors, "with-monitors", false, "Include affected monitor details")

	// Get flags
	maintenanceGetCmd.Flags().BoolVar(&maintenanceWithMonitors, "with-monitors", false, "Include affected monitor details")

	// Create flags
	maintenanceCreateCmd.Flags().StringVarP(&maintenanceData, "data", "d", "", "JSON payload")

	// Update flags
	maintenanceUpdateCmd.Flags().StringVarP(&maintenanceData, "data", "d", "", "JSON payload")
}

func splitAndTrimMaint(s string) []string {
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

func getMaintenanceRequestBody() ([]byte, error) {
	if maintenanceData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(maintenanceData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(maintenanceData), nil
	}
	return nil, nil
}

func maintenanceOutputToTarget(content string) {
	if maintenanceOutput != "" {
		if err := os.WriteFile(maintenanceOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", maintenanceOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", maintenanceOutput))
	} else {
		fmt.Println(content)
	}
}
