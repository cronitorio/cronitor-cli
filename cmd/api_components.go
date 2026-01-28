package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var componentStatuspage string

var apiComponentsCmd = &cobra.Command{
	Use:   "components [action] [key]",
	Short: "Manage status page components",
	Long: `
Manage Cronitor status page components.

Components are the building blocks of status pages. Each component represents
a monitor or group of monitors that feed into your status page.

Actions:
  list     - List all components (default)
  get      - Get a specific component by key
  create   - Create a new component
  update   - Update an existing component
  delete   - Delete a component

Examples:
  List all components:
  $ cronitor api components

  List components for a specific status page:
  $ cronitor api components --statuspage <statuspage-key>

  Get a specific component:
  $ cronitor api components get <key>

  Create a component:
  $ cronitor api components create --data '{"name":"API Server","statuspage":"my-status-page","monitor":"api-check"}'

  Update a component:
  $ cronitor api components update <key> --data '{"name":"Updated Name"}'

  Delete a component:
  $ cronitor api components delete <key>

  Output as table:
  $ cronitor api components --format table
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
			listComponents(client)
		case "get":
			if key == "" {
				fatal("component key is required for get action", 1)
			}
			getComponent(client, key)
		case "create":
			createComponent(client)
		case "update":
			if key == "" {
				fatal("component key is required for update action", 1)
			}
			updateComponent(client, key)
		case "delete":
			if key == "" {
				fatal("component key is required for delete action", 1)
			}
			deleteComponent(client, key)
		default:
			// Treat first arg as a key for get if it doesn't match an action
			getComponent(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiComponentsCmd)
	apiComponentsCmd.Flags().StringVar(&componentStatuspage, "statuspage", "", "Filter by status page key")
}

func listComponents(client *lib.APIClient) {
	params := buildQueryParams()
	if componentStatuspage != "" {
		params["statuspage"] = componentStatuspage
	}

	resp, err := client.GET("/statuspage_components", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list components: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Status Page", "Monitor", "Status"},
		func(data []byte) [][]string {
			var result struct {
				Components []struct {
					Key        string `json:"key"`
					Name       string `json:"name"`
					StatusPage string `json:"statuspage"`
					Monitor    string `json:"monitor"`
					Status     string `json:"status"`
				} `json:"components"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Components))
			for i, c := range result.Components {
				rows[i] = []string{c.Key, c.Name, c.StatusPage, c.Monitor, c.Status}
			}
			return rows
		})
}

func getComponent(client *lib.APIClient, key string) {
	resp, err := client.GET(fmt.Sprintf("/statuspage_components/%s", key), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get component: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Component '%s' could not be found", key), 1)
	}

	outputResponse(resp, nil, nil)
}

func createComponent(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/statuspage_components", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create component: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateComponent(client *lib.APIClient, key string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.PUT(fmt.Sprintf("/statuspage_components/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update component: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteComponent(client *lib.APIClient, key string) {
	resp, err := client.DELETE(fmt.Sprintf("/statuspage_components/%s", key), nil, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to delete component: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Component '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Component '%s' deleted successfully\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
