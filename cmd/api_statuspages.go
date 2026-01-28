package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	statuspageNew    string
	statuspageUpdate string
	statuspageDelete bool
)

var apiStatuspagesCmd = &cobra.Command{
	Use:   "statuspages [key]",
	Short: "Manage status pages",
	Long: `
Manage Cronitor status pages.

Status pages turn your Cronitor monitoring data into public (or private)
communication. Your monitors feed directly into status components, creating
a real-time view of your system health.

Examples:
  List all status pages:
  $ cronitor api statuspages

  Get a specific status page:
  $ cronitor api statuspages <key>

  Create a status page:
  $ cronitor api statuspages --new '{"name":"API Status","hosted_subdomain":"api-status"}'

  Update a status page:
  $ cronitor api statuspages <key> --update '{"name":"Updated Status Page"}'

  Delete a status page:
  $ cronitor api statuspages <key> --delete

  Output as table:
  $ cronitor api statuspages --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		switch {
		case statuspageNew != "":
			createStatuspage(client, statuspageNew)
		case statuspageUpdate != "":
			if key == "" {
				fatal("status page key is required for --update", 1)
			}
			updateStatuspage(client, key, statuspageUpdate)
		case statuspageDelete:
			if key == "" {
				fatal("status page key is required for --delete", 1)
			}
			deleteStatuspage(client, key)
		case key != "":
			getStatuspage(client, key)
		default:
			listStatuspages(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiStatuspagesCmd)
	apiStatuspagesCmd.Flags().StringVar(&statuspageNew, "new", "", "Create status page with JSON data")
	apiStatuspagesCmd.Flags().StringVar(&statuspageUpdate, "update", "", "Update status page with JSON data")
	apiStatuspagesCmd.Flags().BoolVar(&statuspageDelete, "delete", false, "Delete the status page")
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

func createStatuspage(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST("/statuspages", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create status page: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateStatuspage(client *lib.APIClient, key string, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
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
		fmt.Printf("Status page '%s' deleted\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
