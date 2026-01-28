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

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage issues",
	Long: `Manage Cronitor issues and incidents.

Examples:
  cronitor issue list
  cronitor issue list --state open
  cronitor issue get <key>
  cronitor issue create --data '{"monitor":"my-job","summary":"Issue title"}'
  cronitor issue update <key> --data '{"state":"resolved"}'
  cronitor issue resolve <key>
  cronitor issue delete <key>`,
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
	issueFormat   string
	issueOutput   string
	issueData     string
	issueFile     string
	issueState    string
	issueSeverity string
	issueMonitor  string
)

func init() {
	RootCmd.AddCommand(issueCmd)
	issueCmd.PersistentFlags().IntVar(&issuePage, "page", 1, "Page number")
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
  cronitor issue list --state open
  cronitor issue list --severity high
  cronitor issue list --monitor my-job`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if issuePage > 1 {
			params["page"] = fmt.Sprintf("%d", issuePage)
		}
		if issueState != "" {
			params["state"] = issueState
		}
		if issueSeverity != "" {
			params["severity"] = issueSeverity
		}
		if issueMonitor != "" {
			params["monitor"] = issueMonitor
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
				Summary  string `json:"summary"`
				Monitor  string `json:"monitor"`
				State    string `json:"state"`
				Severity string `json:"severity"`
			} `json:"issues"`
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
			Headers: []string{"KEY", "SUMMARY", "MONITOR", "STATE", "SEVERITY"},
		}

		for _, issue := range result.Issues {
			state := issue.State
			if state == "open" {
				state = errorStyle.Render("open")
			} else if state == "resolved" {
				state = successStyle.Render("resolved")
			} else {
				state = mutedStyle.Render(state)
			}

			severity := issue.Severity
			if severity == "high" || severity == "critical" {
				severity = errorStyle.Render(severity)
			} else if severity == "medium" {
				severity = warningStyle.Render(severity)
			}

			summary := issue.Summary
			if len(summary) > 40 {
				summary = summary[:37] + "..."
			}

			table.Rows = append(table.Rows, []string{issue.Key, summary, issue.Monitor, state, severity})
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

		resp, err := client.GET(fmt.Sprintf("/issues/%s", key), nil)
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
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getIssueRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/issues", body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create issue: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success("Issue created")
		issueOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- UPDATE ---
var issueUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an issue",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		body, err := getIssueRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update issue: %s", err))
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

		body := []byte(`{"state":"resolved"}`)
		resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to resolve issue: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Issue '%s' not found", key))
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

func init() {
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueGetCmd)
	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueUpdateCmd)
	issueCmd.AddCommand(issueResolveCmd)
	issueCmd.AddCommand(issueDeleteCmd)

	// List filters
	issueListCmd.Flags().StringVar(&issueState, "state", "", "Filter by state: open, resolved")
	issueListCmd.Flags().StringVar(&issueSeverity, "severity", "", "Filter by severity")
	issueListCmd.Flags().StringVar(&issueMonitor, "monitor", "", "Filter by monitor key")

	// Create/Update flags
	issueCreateCmd.Flags().StringVarP(&issueData, "data", "d", "", "JSON data")
	issueCreateCmd.Flags().StringVarP(&issueFile, "file", "f", "", "JSON file")
	issueUpdateCmd.Flags().StringVarP(&issueData, "data", "d", "", "JSON data")
	issueUpdateCmd.Flags().StringVarP(&issueFile, "file", "f", "", "JSON file")
}

func getIssueRequestBody() ([]byte, error) {
	if issueData != "" && issueFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if issueData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(issueData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(issueData), nil
	}

	if issueFile != "" {
		data, err := os.ReadFile(issueFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return data, nil
	}

	return nil, nil
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
