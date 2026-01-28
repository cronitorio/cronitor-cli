package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var apiEnvironmentsCmd = &cobra.Command{
	Use:   "environments [action] [key]",
	Short: "Manage environments",
	Long: `
Manage Cronitor environments.

Environments allow you to separate monitoring data between different
deployment stages (e.g., staging, production) while sharing monitor
configurations.

Actions:
  list     - List all environments (default)
  get      - Get a specific environment by key
  create   - Create a new environment
  update   - Update an existing environment
  delete   - Delete an environment

Examples:
  List all environments:
  $ cronitor api environments

  Get a specific environment:
  $ cronitor api environments get <key>

  Create an environment:
  $ cronitor api environments create --data '{"key":"staging","name":"Staging"}'

  Update an environment:
  $ cronitor api environments update <key> --data '{"name":"Updated Name"}'

  Delete an environment:
  $ cronitor api environments delete <key>

  Output as table:
  $ cronitor api environments --format table
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
			listEnvironments(client)
		case "get":
			if key == "" {
				fatal("environment key is required for get action", 1)
			}
			getEnvironment(client, key)
		case "create":
			createEnvironment(client)
		case "update":
			if key == "" {
				fatal("environment key is required for update action", 1)
			}
			updateEnvironment(client, key)
		case "delete":
			if key == "" {
				fatal("environment key is required for delete action", 1)
			}
			deleteEnvironment(client, key)
		default:
			// Treat first arg as a key for get if it doesn't match an action
			getEnvironment(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiEnvironmentsCmd)
}

func listEnvironments(client *lib.APIClient) {
	params := buildQueryParams()
	resp, err := client.GET("/environments", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list environments: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Default", "Created"},
		func(data []byte) [][]string {
			var result struct {
				Environments []struct {
					Key       string `json:"key"`
					Name      string `json:"name"`
					IsDefault bool   `json:"is_default"`
					CreatedAt string `json:"created_at"`
				} `json:"environments"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Environments))
			for i, e := range result.Environments {
				isDefault := ""
				if e.IsDefault {
					isDefault = "Yes"
				}
				rows[i] = []string{e.Key, e.Name, isDefault, e.CreatedAt}
			}
			return rows
		})
}

func getEnvironment(client *lib.APIClient, key string) {
	resp, err := client.GET(fmt.Sprintf("/environments/%s", key), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get environment: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Environment '%s' could not be found", key), 1)
	}

	outputResponse(resp, nil, nil)
}

func createEnvironment(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/environments", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create environment: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateEnvironment(client *lib.APIClient, key string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.PUT(fmt.Sprintf("/environments/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update environment: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteEnvironment(client *lib.APIClient, key string) {
	resp, err := client.DELETE(fmt.Sprintf("/environments/%s", key), nil, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to delete environment: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Environment '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Environment '%s' deleted successfully\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
