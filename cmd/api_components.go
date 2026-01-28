package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	componentNew        string
	componentUpdate     string
	componentDelete     bool
	componentStatuspage string
)

var apiComponentsCmd = &cobra.Command{
	Use:   "components [key]",
	Short: "Manage status page components",
	Long: `
Manage Cronitor status page components.

Components are the building blocks of status pages. Each component represents
a monitor or group of monitors that feed into your status page.

Examples:
  List all components:
  $ cronitor api components

  List components for a specific status page:
  $ cronitor api components --statuspage <statuspage-key>

  Get a specific component:
  $ cronitor api components <key>

  Create a component:
  $ cronitor api components --new '{"name":"API Server","statuspage":"my-status-page","monitor":"api-check"}'

  Update a component:
  $ cronitor api components <key> --update '{"name":"Updated Name"}'

  Delete a component:
  $ cronitor api components <key> --delete

  Output as table:
  $ cronitor api components --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		switch {
		case componentNew != "":
			createComponent(client, componentNew)
		case componentUpdate != "":
			if key == "" {
				fatal("component key is required for --update", 1)
			}
			updateComponent(client, key, componentUpdate)
		case componentDelete:
			if key == "" {
				fatal("component key is required for --delete", 1)
			}
			deleteComponent(client, key)
		case key != "":
			getComponent(client, key)
		default:
			listComponents(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiComponentsCmd)
	apiComponentsCmd.Flags().StringVar(&componentNew, "new", "", "Create component with JSON data")
	apiComponentsCmd.Flags().StringVar(&componentUpdate, "update", "", "Update component with JSON data")
	apiComponentsCmd.Flags().BoolVar(&componentDelete, "delete", false, "Delete the component")
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

func createComponent(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST("/statuspage_components", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create component: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateComponent(client *lib.APIClient, key string, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
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
		fmt.Printf("Component '%s' deleted\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
