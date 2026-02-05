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

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage issues",
	Long: `Manage Cronitor issues and incidents.

Severity levels: missing_data, operational, maintenance, degraded_performance, minor_outage, outage
States: unresolved, investigating, identified, monitoring, resolved

Examples:
  cronitor issue list
  cronitor issue list --state unresolved
  cronitor issue list --severity outage --time 24h
  cronitor issue get <key>
  cronitor issue create "Database connection issues" --severity outage
  cronitor issue update <key> --state investigating
  cronitor issue resolve <key>
  cronitor issue delete <key>

For full API documentation, see https://cronitor.io/docs/issues-api.md`,
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

var (
	issuePage     int
	issuePageSize int
	issueFormat   string
	issueOutput   string
	issueData     string
	issueState    string
	issueSeverity string
	issueMonitor  string
	issueGroup    string
	issueTag      string
	issueEnv      string
	issueSearch   string
	issueTime     string
	issueOrderBy  string
	// Expansion flags
	issueWithStatuspageDetails bool
	issueWithMonitorDetails    bool
	issueWithAlertDetails      bool
	issueWithComponentDetails  bool
	// Bulk flags
	issueBulkAction   string
	issueBulkIssues   string
	issueBulkState    string
	issueBulkAssignTo string
)

func init() {
	RootCmd.AddCommand(issueCmd)
	issueCmd.PersistentFlags().IntVar(&issuePage, "page", 1, "Page number")
	issueCmd.PersistentFlags().IntVar(&issuePageSize, "page-size", 0, "Results per page (max 1000)")
	issueCmd.PersistentFlags().StringVar(&issueFormat, "format", "", "Output format: json, table")
	issueCmd.PersistentFlags().StringVarP(&issueOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all issues",
	Long: `List all issues in your Cronitor account.

Examples:
  cronitor issue list
  cronitor issue list --state unresolved
  cronitor issue list --severity outage
  cronitor issue list --monitor my-job
  cronitor issue list --group production
  cronitor issue list --time 24h
  cronitor issue list --search "database"
  cronitor issue list --order-by -started`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if issuePage > 1 {
			params["page"] = fmt.Sprintf("%d", issuePage)
		}
		if issuePageSize > 0 {
			params["pageSize"] = fmt.Sprintf("%d", issuePageSize)
		}
		if issueState != "" {
			params["state"] = issueState
		}
		if issueSeverity != "" {
			params["severity"] = issueSeverity
		}
		if issueMonitor != "" {
			params["job"] = issueMonitor
		}
		if issueGroup != "" {
			params["group"] = issueGroup
		}
		if issueTag != "" {
			params["tag"] = issueTag
		}
		if issueEnv != "" {
			params["env"] = issueEnv
		}
		if issueSearch != "" {
			params["search"] = issueSearch
		}
		if issueTime != "" {
			params["time"] = issueTime
		}
		if issueOrderBy != "" {
			params["orderBy"] = issueOrderBy
		}
		if issueWithStatuspageDetails {
			params["withStatusPageDetails"] = "true"
		}
		if issueWithMonitorDetails {
			params["withMonitorDetails"] = "true"
		}
		if issueWithAlertDetails {
			params["withAlertDetails"] = "true"
		}
		if issueWithComponentDetails {
			params["withComponentDetails"] = "true"
		}

		resp, err := client.GET("/issues", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list issues: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			Issues []struct {
				Key      string `json:"key"`
				Name     string `json:"name"`
				State    string `json:"state"`
				Severity string `json:"severity"`
				Started  string `json:"started"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		format := issueFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			issueOutputToTarget(FormatJSON(resp.Body))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "STATE", "SEVERITY", "STARTED"},
		}

		for _, issue := range result.Issues {
			state := issue.State
			switch state {
			case "unresolved":
				state = errorStyle.Render("unresolved")
			case "investigating", "identified":
				state = warningStyle.Render(state)
			case "monitoring":
				state = mutedStyle.Render(state)
			case "resolved":
				state = successStyle.Render("resolved")
			}

			severity := issue.Severity
			switch severity {
			case "outage", "minor_outage":
				severity = errorStyle.Render(severity)
			case "degraded_performance":
				severity = warningStyle.Render(severity)
			case "maintenance":
				severity = mutedStyle.Render(severity)
			}

			name := issue.Name
			if len(name) > 40 {
				name = name[:37] + "..."
			}

			started := ""
			if issue.Started != "" && len(issue.Started) >= 10 {
				started = issue.Started[:10]
			}

			table.Rows = append(table.Rows, []string{name, issue.Key, state, severity, started})
		}

		issueOutputToTarget(table.Render())
	},
}

// --- GET ---
var issueGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific issue",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		params := make(map[string]string)
		if issueWithStatuspageDetails {
			params["withStatusPageDetails"] = "true"
		}
		if issueWithMonitorDetails {
			params["withMonitorDetails"] = "true"
		}
		if issueWithAlertDetails {
			params["withAlertDetails"] = "true"
		}
		if issueWithComponentDetails {
			params["withComponentDetails"] = "true"
		}

		resp, err := client.GET(fmt.Sprintf("/issues/%s", key), params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get issue: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Issue '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		issueOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	Long: `Create a new issue.

Severity levels: missing_data, operational, maintenance, degraded_performance, minor_outage, outage

Examples:
  cronitor issue create --data '{"name":"Database connection issues","severity":"outage"}'
  cronitor issue create --data '{"name":"Scheduled maintenance","severity":"maintenance","state":"monitoring"}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if issueData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(issueData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/issues", []byte(issueData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create issue: %s", err))
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
			Success(fmt.Sprintf("Created issue: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Issue created")
		}

		if issueFormat == "json" {
			issueOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- UPDATE ---
var issueUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an issue",
	Long: `Update an existing issue.

Examples:
  cronitor issue update my-issue --data '{"state":"investigating"}'
  cronitor issue update my-issue --data '{"severity":"outage"}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if issueData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(issueData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update issue: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Issue '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Issue '%s' updated", key))
		issueOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- RESOLVE ---
var issueResolveCmd = &cobra.Command{
	Use:   "resolve <key>",
	Short: "Resolve an issue",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		// Fetch current issue to get required fields
		getResp, err := client.GET(fmt.Sprintf("/issues/%s", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to resolve issue: %s", err))
			os.Exit(1)
		}
		if getResp.IsNotFound() {
			Error(fmt.Sprintf("Issue '%s' not found", key))
			os.Exit(1)
		}
		if !getResp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", getResp.StatusCode, getResp.ParseError()))
			os.Exit(1)
		}

		var current map[string]interface{}
		json.Unmarshal(getResp.Body, &current)
		current["state"] = "resolved"
		body, _ := json.Marshal(current)

		resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to resolve issue: %s", err))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Issue '%s' resolved", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

// --- DELETE ---
var issueDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete an issue",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/issues/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete issue: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Issue '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Issue '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

// --- BULK ---
var issueBulkCmd = &cobra.Command{
	Use:   "bulk",
	Short: "Perform bulk actions on issues",
	Long: `Perform bulk actions on multiple issues at once.

Actions: delete, change_state, assign_to

Examples:
  cronitor issue bulk --action delete --issues KEY1,KEY2,KEY3
  cronitor issue bulk --action change_state --issues KEY1,KEY2 --state resolved
  cronitor issue bulk --action assign_to --issues KEY1,KEY2 --assign-to user@example.com`,
	Run: func(cmd *cobra.Command, args []string) {
		if issueBulkAction == "" {
			Error("Action required. Use --action (delete, change_state, assign_to)")
			os.Exit(1)
		}
		if issueBulkIssues == "" {
			Error("Issues required. Use --issues KEY1,KEY2,KEY3")
			os.Exit(1)
		}

		issues := strings.Split(issueBulkIssues, ",")
		for i := range issues {
			issues[i] = strings.TrimSpace(issues[i])
		}

		body := map[string]interface{}{
			"action": issueBulkAction,
			"issues": issues,
		}

		switch issueBulkAction {
		case "change_state":
			if issueBulkState == "" {
				Error("State required for change_state action. Use --state")
				os.Exit(1)
			}
			body["state"] = issueBulkState
		case "assign_to":
			if issueBulkAssignTo == "" {
				Error("Assignee required for assign_to action. Use --assign-to")
				os.Exit(1)
			}
			body["assign_to"] = issueBulkAssignTo
		case "delete":
			// No extra fields needed
		default:
			Error(fmt.Sprintf("Unknown action '%s'. Use: delete, change_state, assign_to", issueBulkAction))
			os.Exit(1)
		}

		jsonBody, err := json.Marshal(body)
		if err != nil {
			Error(fmt.Sprintf("Failed to encode request: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/issues/bulk", jsonBody, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to perform bulk action: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Bulk %s completed for %d issues", issueBulkAction, len(issues)))
		if issueFormat == "json" {
			issueOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

func init() {
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueGetCmd)
	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueUpdateCmd)
	issueCmd.AddCommand(issueResolveCmd)
	issueCmd.AddCommand(issueDeleteCmd)
	issueCmd.AddCommand(issueBulkCmd)

	// List filters
	issueListCmd.Flags().StringVar(&issueState, "state", "", "Filter by state: unresolved, investigating, identified, monitoring, resolved")
	issueListCmd.Flags().StringVar(&issueSeverity, "severity", "", "Filter by severity: outage, minor_outage, degraded_performance, maintenance, operational, missing_data")
	issueListCmd.Flags().StringVar(&issueMonitor, "monitor", "", "Filter by monitor key")
	issueListCmd.Flags().StringVar(&issueGroup, "group", "", "Filter by group key")
	issueListCmd.Flags().StringVar(&issueTag, "tag", "", "Filter by monitor tag")
	issueListCmd.Flags().StringVar(&issueEnv, "env", "", "Filter by environment key")
	issueListCmd.Flags().StringVar(&issueSearch, "search", "", "Search issue/monitor names and keys")
	issueListCmd.Flags().StringVar(&issueTime, "time", "", "Time range: 24h, 7d, 30d")
	issueListCmd.Flags().StringVar(&issueOrderBy, "order-by", "", "Sort: started, -started, relevance, -relevance")

	// List expansion flags
	issueListCmd.Flags().BoolVar(&issueWithStatuspageDetails, "with-statuspage-details", false, "Include status page details")
	issueListCmd.Flags().BoolVar(&issueWithMonitorDetails, "with-monitor-details", false, "Include monitor details")
	issueListCmd.Flags().BoolVar(&issueWithAlertDetails, "with-alert-details", false, "Include alert details")
	issueListCmd.Flags().BoolVar(&issueWithComponentDetails, "with-component-details", false, "Include component details")

	// Get expansion flags
	issueGetCmd.Flags().BoolVar(&issueWithStatuspageDetails, "with-statuspage-details", false, "Include status page details")
	issueGetCmd.Flags().BoolVar(&issueWithMonitorDetails, "with-monitor-details", false, "Include monitor details")
	issueGetCmd.Flags().BoolVar(&issueWithAlertDetails, "with-alert-details", false, "Include alert details")
	issueGetCmd.Flags().BoolVar(&issueWithComponentDetails, "with-component-details", false, "Include component details")

	// Create flags
	issueCreateCmd.Flags().StringVarP(&issueData, "data", "d", "", "JSON payload")

	// Update flags
	issueUpdateCmd.Flags().StringVarP(&issueData, "data", "d", "", "JSON payload")

	// Bulk flags
	issueBulkCmd.Flags().StringVar(&issueBulkAction, "action", "", "Bulk action: delete, change_state, assign_to")
	issueBulkCmd.Flags().StringVar(&issueBulkIssues, "issues", "", "Comma-separated issue keys")
	issueBulkCmd.Flags().StringVar(&issueBulkState, "state", "", "New state (for change_state action)")
	issueBulkCmd.Flags().StringVar(&issueBulkAssignTo, "assign-to", "", "Assignee (for assign_to action)")
}

func issueOutputToTarget(content string) {
	if issueOutput != "" {
		if err := os.WriteFile(issueOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", issueOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", issueOutput))
	} else {
		fmt.Println(content)
	}
}
