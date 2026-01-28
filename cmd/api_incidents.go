package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	incidentNew        string
	incidentUpdate     string
	incidentResolve    bool
	incidentStatuspage string
)

var apiIncidentsCmd = &cobra.Command{
	Use:   "incidents [id]",
	Short: "Manage status page incidents",
	Long: `
Manage Cronitor status page incidents.

Incidents communicate problems on your status pages - either created
automatically by monitor failures or manually for planned maintenance.

Examples:
  List all incidents:
  $ cronitor api incidents

  Filter by status page:
  $ cronitor api incidents --statuspage <key>

  Get a specific incident:
  $ cronitor api incidents <id>

  Create an incident:
  $ cronitor api incidents --new '{"title":"API Degradation","severity":"warning","statuspage":"my-page"}'

  Update an incident:
  $ cronitor api incidents <id> --update '{"message":"Deploying fix..."}'

  Resolve an incident:
  $ cronitor api incidents <id> --resolve

  Output as table:
  $ cronitor api incidents --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		id := ""
		if len(args) > 0 {
			id = args[0]
		}

		switch {
		case incidentNew != "":
			createIncident(client, incidentNew)
		case incidentUpdate != "":
			if id == "" {
				fatal("incident ID is required for --update", 1)
			}
			updateIncident(client, id, incidentUpdate)
		case incidentResolve:
			if id == "" {
				fatal("incident ID is required for --resolve", 1)
			}
			resolveIncident(client, id)
		case id != "":
			getIncident(client, id)
		default:
			listIncidents(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiIncidentsCmd)
	apiIncidentsCmd.Flags().StringVar(&incidentNew, "new", "", "Create incident with JSON data")
	apiIncidentsCmd.Flags().StringVar(&incidentUpdate, "update", "", "Add update to incident with JSON data")
	apiIncidentsCmd.Flags().BoolVar(&incidentResolve, "resolve", false, "Resolve the incident")
	apiIncidentsCmd.Flags().StringVar(&incidentStatuspage, "statuspage", "", "Filter by status page key")
}

func listIncidents(client *lib.APIClient) {
	params := buildQueryParams()
	if incidentStatuspage != "" {
		params["statuspage"] = incidentStatuspage
	}

	resp, err := client.GET("/incidents", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list incidents: %s", err), 1)
	}

	outputResponse(resp, []string{"ID", "Title", "Status", "Severity", "Created"},
		func(data []byte) [][]string {
			var result struct {
				Incidents []struct {
					ID        string `json:"id"`
					Title     string `json:"title"`
					Status    string `json:"status"`
					Severity  string `json:"severity"`
					CreatedAt string `json:"created_at"`
				} `json:"incidents"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Incidents))
			for i, inc := range result.Incidents {
				rows[i] = []string{inc.ID, inc.Title, inc.Status, inc.Severity, inc.CreatedAt}
			}
			return rows
		})
}

func getIncident(client *lib.APIClient, id string) {
	resp, err := client.GET(fmt.Sprintf("/incidents/%s", id), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get incident: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Incident '%s' could not be found", id), 1)
	}

	outputResponse(resp, nil, nil)
}

func createIncident(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST("/incidents", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create incident: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateIncident(client *lib.APIClient, id string, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST(fmt.Sprintf("/incidents/%s/updates", id), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update incident: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func resolveIncident(client *lib.APIClient, id string) {
	body := []byte(`{"status":"resolved"}`)

	resp, err := client.PUT(fmt.Sprintf("/incidents/%s", id), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to resolve incident: %s", err), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Incident '%s' resolved\n", id)
	} else {
		outputResponse(resp, nil, nil)
	}
}
