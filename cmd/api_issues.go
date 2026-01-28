package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var issueState string
var issueSeverity string

var apiIssuesCmd = &cobra.Command{
	Use:   "issues [action] [key]",
	Short: "Manage issues and incidents",
	Long: `
Manage Cronitor issues and incidents.

Issues are Cronitor's incident management hub - they help your team coordinate
response when monitors fail. Issues automatically open when monitors fail and
close when they recover.

Actions:
  list     - List all issues (default)
  get      - Get a specific issue by key
  create   - Create a new issue
  update   - Update an existing issue
  delete   - Delete an issue
  bulk     - Perform bulk operations on issues

Examples:
  List all issues:
  $ cronitor api issues

  List open issues:
  $ cronitor api issues --state open

  List issues filtered by severity:
  $ cronitor api issues --severity critical

  Get a specific issue:
  $ cronitor api issues get <issue-key>

  Create an issue:
  $ cronitor api issues create --data '{"title":"Service Outage","severity":"critical","monitors":["web-api"]}'

  Update an issue:
  $ cronitor api issues update <issue-key> --data '{"state":"resolved"}'

  Add an update to an issue:
  $ cronitor api issues update <issue-key> --data '{"message":"Investigating the root cause"}'

  Delete an issue:
  $ cronitor api issues delete <issue-key>

  Bulk resolve issues:
  $ cronitor api issues bulk --data '{"action":"resolve","keys":["issue-1","issue-2"]}'

  Output as table:
  $ cronitor api issues --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		action := "list"
		var key string

		if len(args) > 0 {
			action = args[0]
		}
		if len(args) > 1 {
			key = args[1]
		}

		client := getAPIClient()

		switch action {
		case "list":
			listIssues(client)
		case "get":
			if key == "" {
				fatal("issue key is required for get action", 1)
			}
			getIssue(client, key)
		case "create":
			createIssue(client)
		case "update":
			if key == "" {
				fatal("issue key is required for update action", 1)
			}
			updateIssue(client, key)
		case "delete":
			if key == "" {
				fatal("issue key is required for delete action", 1)
			}
			deleteIssue(client, key)
		case "bulk":
			bulkIssues(client)
		default:
			// Treat first arg as a key for get if it doesn't match an action
			getIssue(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiIssuesCmd)
	apiIssuesCmd.Flags().StringVar(&issueState, "state", "", "Filter by state (open, resolved)")
	apiIssuesCmd.Flags().StringVar(&issueSeverity, "severity", "", "Filter by severity (critical, warning, info)")
}

func listIssues(client *lib.APIClient) {
	params := buildQueryParams()
	if issueState != "" {
		params["state"] = issueState
	}
	if issueSeverity != "" {
		params["severity"] = issueSeverity
	}

	resp, err := client.GET("/issues", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list issues: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Title", "State", "Severity", "Monitors", "Created"},
		func(data []byte) [][]string {
			var result struct {
				Issues []struct {
					Key       string   `json:"key"`
					Title     string   `json:"title"`
					State     string   `json:"state"`
					Severity  string   `json:"severity"`
					Monitors  []string `json:"monitors"`
					CreatedAt string   `json:"created_at"`
				} `json:"issues"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Issues))
			for i, issue := range result.Issues {
				monitors := ""
				if len(issue.Monitors) > 0 {
					monitors = fmt.Sprintf("%v", issue.Monitors)
				}
				rows[i] = []string{issue.Key, issue.Title, issue.State, issue.Severity, monitors, issue.CreatedAt}
			}
			return rows
		})
}

func getIssue(client *lib.APIClient, key string) {
	resp, err := client.GET(fmt.Sprintf("/issues/%s", key), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get issue: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Issue '%s' could not be found", key), 1)
	}

	outputResponse(resp, nil, nil)
}

func createIssue(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/issues", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create issue: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateIssue(client *lib.APIClient, key string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update issue: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteIssue(client *lib.APIClient, key string) {
	resp, err := client.DELETE(fmt.Sprintf("/issues/%s", key), nil, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to delete issue: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Issue '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Issue '%s' deleted successfully\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}

func bulkIssues(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for bulk action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/issues/bulk", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to perform bulk operation: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}
