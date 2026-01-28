package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	environmentNew    string
	environmentUpdate string
	environmentDelete bool
)

var apiEnvironmentsCmd = &cobra.Command{
	Use:   "environments [key]",
	Short: "Manage environments",
	Long: `
Manage Cronitor environments.

Environments separate monitoring data between deployment stages (staging, production)
while sharing monitor configurations.

Examples:
  List all environments:
  $ cronitor api environments

  Get a specific environment:
  $ cronitor api environments <key>

  Create an environment:
  $ cronitor api environments --new '{"key":"staging","name":"Staging"}'

  Update an environment:
  $ cronitor api environments <key> --update '{"name":"Updated Name"}'

  Delete an environment:
  $ cronitor api environments <key> --delete

  Output as table:
  $ cronitor api environments --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		switch {
		case environmentNew != "":
			createEnvironment(client, environmentNew)
		case environmentUpdate != "":
			if key == "" {
				fatal("environment key is required for --update", 1)
			}
			updateEnvironment(client, key, environmentUpdate)
		case environmentDelete:
			if key == "" {
				fatal("environment key is required for --delete", 1)
			}
			deleteEnvironment(client, key)
		case key != "":
			getEnvironment(client, key)
		default:
			listEnvironments(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiEnvironmentsCmd)
	apiEnvironmentsCmd.Flags().StringVar(&environmentNew, "new", "", "Create environment with JSON data")
	apiEnvironmentsCmd.Flags().StringVar(&environmentUpdate, "update", "", "Update environment with JSON data")
	apiEnvironmentsCmd.Flags().BoolVar(&environmentDelete, "delete", false, "Delete the environment")
}

func listEnvironments(client *lib.APIClient) {
	params := buildQueryParams()
	resp, err := client.GET("/environments", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list environments: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Default"},
		func(data []byte) [][]string {
			var result struct {
				Environments []struct {
					Key       string `json:"key"`
					Name      string `json:"name"`
					IsDefault bool   `json:"is_default"`
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
				rows[i] = []string{e.Key, e.Name, isDefault}
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

func createEnvironment(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST("/environments", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create environment: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateEnvironment(client *lib.APIClient, key string, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
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
		fmt.Printf("Environment '%s' deleted\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
