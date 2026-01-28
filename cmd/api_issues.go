package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	issueNew      string
	issueUpdate   string
	issueDelete   bool
	issueResolve  bool
	issueState    string
	issueSeverity string
)

var apiIssuesCmd = &cobra.Command{
	Use:   "issues [key]",
	Short: "Manage issues",
	Long: `
Manage Cronitor issues and incidents.

Issues are Cronitor's incident management hub - they help your team coordinate
response when monitors fail.

Examples:
  List all issues:
  $ cronitor api issues

  List open issues:
  $ cronitor api issues --state open

  Filter by severity:
  $ cronitor api issues --severity critical

  Get a specific issue:
  $ cronitor api issues <key>

  Create an issue:
  $ cronitor api issues --new '{"title":"Service Outage","severity":"critical"}'

  Update an issue:
  $ cronitor api issues <key> --update '{"message":"Investigating..."}'

  Resolve an issue:
  $ cronitor api issues <key> --resolve

  Delete an issue:
  $ cronitor api issues <key> --delete

  Output as table:
  $ cronitor api issues --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		switch {
		case issueNew != "":
			createIssue(client, issueNew)
		case issueUpdate != "":
			if key == "" {
				fatal("issue key is required for --update", 1)
			}
			updateIssue(client, key, issueUpdate)
		case issueResolve:
			if key == "" {
				fatal("issue key is required for --resolve", 1)
			}
			resolveIssue(client, key)
		case issueDelete:
			if key == "" {
				fatal("issue key is required for --delete", 1)
			}
			deleteIssue(client, key)
		case key != "":
			getIssue(client, key)
		default:
			listIssues(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiIssuesCmd)
	apiIssuesCmd.Flags().StringVar(&issueNew, "new", "", "Create issue with JSON data")
	apiIssuesCmd.Flags().StringVar(&issueUpdate, "update", "", "Update issue with JSON data")
	apiIssuesCmd.Flags().BoolVar(&issueDelete, "delete", false, "Delete the issue")
	apiIssuesCmd.Flags().BoolVar(&issueResolve, "resolve", false, "Resolve the issue")
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

	outputResponse(resp, []string{"Key", "Title", "State", "Severity", "Created"},
		func(data []byte) [][]string {
			var result struct {
				Issues []struct {
					Key       string `json:"key"`
					Title     string `json:"title"`
					State     string `json:"state"`
					Severity  string `json:"severity"`
					CreatedAt string `json:"created_at"`
				} `json:"issues"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Issues))
			for i, issue := range result.Issues {
				rows[i] = []string{issue.Key, issue.Title, issue.State, issue.Severity, issue.CreatedAt}
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

func createIssue(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST("/issues", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create issue: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateIssue(client *lib.APIClient, key string, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update issue: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func resolveIssue(client *lib.APIClient, key string) {
	body := []byte(`{"state":"resolved"}`)

	resp, err := client.PUT(fmt.Sprintf("/issues/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to resolve issue: %s", err), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Issue '%s' resolved\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
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
		fmt.Printf("Issue '%s' deleted\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
