package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var apiStatuspagesCmd = &cobra.Command{
	Use:   "statuspages [action] [key]",
	Short: "Manage status pages",
	Long: `
Manage Cronitor status pages.

Status pages turn your Cronitor monitoring data into public (or private)
communication. Your monitors feed directly into status components, creating
a real-time view of your system health.

Actions:
  list     - List all status pages (default)
  get      - Get a specific status page by key
  create   - Create a new status page
  update   - Update an existing status page
  delete   - Delete a status page

Examples:
  List all status pages:
  $ cronitor api statuspages

  Get a specific status page:
  $ cronitor api statuspages get <key>

  Create a status page:
  $ cronitor api statuspages create --data '{"name":"API Status","hosted_subdomain":"api-status"}'

  Update a status page:
  $ cronitor api statuspages update <key> --data '{"name":"Updated Status Page"}'

  Delete a status page:
  $ cronitor api statuspages delete <key>

  Output as table:
  $ cronitor api statuspages --format table
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
			listStatuspages(client)
		case "get":
			if key == "" {
				fatal("status page key is required for get action", 1)
			}
			getStatuspage(client, key)
		case "create":
			createStatuspage(client)
		case "update":
			if key == "" {
				fatal("status page key is required for update action", 1)
			}
			updateStatuspage(client, key)
		case "delete":
			if key == "" {
				fatal("status page key is required for delete action", 1)
			}
			deleteStatuspage(client, key)
		default:
			// Treat first arg as a key for get if it doesn't match an action
			getStatuspage(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiStatuspagesCmd)
}

func listStatuspages(client *lib.APIClient) {
	params := buildQueryParams()
	resp, err := client.GET("/statuspages", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list status pages: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Subdomain", "Status", "Environment"},
		func(data []byte) [][]string {
			var result struct {
				StatusPages []struct {
					Key             string `json:"key"`
					Name            string `json:"name"`
					HostedSubdomain string `json:"hosted_subdomain"`
					Status          string `json:"status"`
					Environment     string `json:"environment"`
				} `json:"statuspages"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.StatusPages))
			for i, sp := range result.StatusPages {
				rows[i] = []string{sp.Key, sp.Name, sp.HostedSubdomain, sp.Status, sp.Environment}
			}
			return rows
		})
}

func getStatuspage(client *lib.APIClient, key string) {
	resp, err := client.GET(fmt.Sprintf("/statuspages/%s", key), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get status page: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Status page '%s' could not be found", key), 1)
	}

	outputResponse(resp, nil, nil)
}

func createStatuspage(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/statuspages", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create status page: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateStatuspage(client *lib.APIClient, key string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.PUT(fmt.Sprintf("/statuspages/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update status page: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteStatuspage(client *lib.APIClient, key string) {
	resp, err := client.DELETE(fmt.Sprintf("/statuspages/%s", key), nil, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to delete status page: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Status page '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Status page '%s' deleted successfully\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
