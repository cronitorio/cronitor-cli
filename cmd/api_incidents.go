package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var incidentStatuspage string

var apiIncidentsCmd = &cobra.Command{
	Use:   "incidents [action] [key]",
	Short: "Manage status page incidents",
	Long: `
Manage Cronitor status page incidents.

Incidents allow you to communicate about problems as they occur - either
privately with teammates or publicly on your status pages. Incidents can
be created automatically by monitor failures or manually for planned
maintenance.

Actions:
  list     - List all incidents (default)
  get      - Get a specific incident by ID
  create   - Create a new incident
  update   - Update/add message to an existing incident
  resolve  - Resolve an incident

Examples:
  List all incidents:
  $ cronitor api incidents

  List incidents for a specific status page:
  $ cronitor api incidents --statuspage <key>

  Get a specific incident:
  $ cronitor api incidents get <incident-id>

  Create an incident:
  $ cronitor api incidents create --data '{"title":"API Degradation","message":"Investigating elevated error rates","severity":"warning","statuspage":"my-status-page"}'

  Add an update to an incident:
  $ cronitor api incidents update <incident-id> --data '{"message":"Root cause identified, deploying fix"}'

  Resolve an incident:
  $ cronitor api incidents resolve <incident-id> --data '{"message":"Issue has been resolved"}'

  Output as table:
  $ cronitor api incidents --format table
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
			listIncidents(client)
		case "get":
			if key == "" {
				fatal("incident ID is required for get action", 1)
			}
			getIncident(client, key)
		case "create":
			createIncident(client)
		case "update":
			if key == "" {
				fatal("incident ID is required for update action", 1)
			}
			updateIncident(client, key)
		case "resolve":
			if key == "" {
				fatal("incident ID is required for resolve action", 1)
			}
			resolveIncident(client, key)
		default:
			// Treat first arg as an ID for get if it doesn't match an action
			getIncident(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiIncidentsCmd)
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

	outputResponse(resp, []string{"ID", "Title", "Status", "Severity", "Status Page", "Created"},
		func(data []byte) [][]string {
			var result struct {
				Incidents []struct {
					ID         string `json:"id"`
					Title      string `json:"title"`
					Status     string `json:"status"`
					Severity   string `json:"severity"`
					StatusPage string `json:"statuspage"`
					CreatedAt  string `json:"created_at"`
				} `json:"incidents"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Incidents))
			for i, inc := range result.Incidents {
				rows[i] = []string{inc.ID, inc.Title, inc.Status, inc.Severity, inc.StatusPage, inc.CreatedAt}
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

func createIncident(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/incidents", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create incident: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateIncident(client *lib.APIClient, id string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	// Add update via the updates endpoint
	resp, err := client.POST(fmt.Sprintf("/incidents/%s/updates", id), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update incident: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func resolveIncident(client *lib.APIClient, id string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	// Build resolve payload
	var payload map[string]interface{}
	if body != nil {
		if err := json.Unmarshal(body, &payload); err != nil {
			payload = make(map[string]interface{})
		}
	} else {
		payload = make(map[string]interface{})
	}
	payload["status"] = "resolved"

	resolveBody, _ := json.Marshal(payload)

	resp, err := client.PUT(fmt.Sprintf("/incidents/%s", id), resolveBody, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to resolve incident: %s", err), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Incident '%s' resolved successfully\n", id)
	} else {
		outputResponse(resp, nil, nil)
	}
}
