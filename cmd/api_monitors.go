package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	monitorNew        string
	monitorUpdate     string
	monitorDelete     bool
	monitorPause      string
	monitorUnpause    bool
	withLatestEvents  bool
)

var apiMonitorsCmd = &cobra.Command{
	Use:   "monitors [key]",
	Short: "Manage monitors",
	Long: `
Manage Cronitor monitors (jobs, checks, heartbeats, sites).

Examples:
  List all monitors:
  $ cronitor api monitors

  List with pagination:
  $ cronitor api monitors --page 2

  Get a specific monitor:
  $ cronitor api monitors <key>

  Get with latest events:
  $ cronitor api monitors <key> --with-events

  Create a monitor:
  $ cronitor api monitors --new '{"key":"my-job","type":"job"}'

  Update a monitor:
  $ cronitor api monitors <key> --update '{"name":"New Name"}'

  Delete a monitor:
  $ cronitor api monitors <key> --delete

  Pause a monitor (indefinitely):
  $ cronitor api monitors <key> --pause

  Pause for 24 hours:
  $ cronitor api monitors <key> --pause 24

  Unpause a monitor:
  $ cronitor api monitors <key> --unpause

  Output as table:
  $ cronitor api monitors --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		// Determine action based on flags
		switch {
		case monitorNew != "":
			createMonitor(client, monitorNew)
		case monitorUpdate != "":
			if key == "" {
				fatal("monitor key is required for --update", 1)
			}
			updateMonitor(client, key, monitorUpdate)
		case monitorDelete:
			if key == "" {
				fatal("monitor key is required for --delete", 1)
			}
			deleteMonitor(client, key)
		case cmd.Flags().Changed("pause"):
			if key == "" {
				fatal("monitor key is required for --pause", 1)
			}
			pauseMonitor(client, key, monitorPause)
		case monitorUnpause:
			if key == "" {
				fatal("monitor key is required for --unpause", 1)
			}
			unpauseMonitor(client, key)
		case key != "":
			getMonitor(client, key)
		default:
			listMonitors(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiMonitorsCmd)
	apiMonitorsCmd.Flags().StringVar(&monitorNew, "new", "", "Create monitor with JSON data")
	apiMonitorsCmd.Flags().StringVar(&monitorUpdate, "update", "", "Update monitor with JSON data")
	apiMonitorsCmd.Flags().BoolVar(&monitorDelete, "delete", false, "Delete the monitor")
	apiMonitorsCmd.Flags().StringVar(&monitorPause, "pause", "", "Pause monitor (optionally specify hours)")
	apiMonitorsCmd.Flags().BoolVar(&monitorUnpause, "unpause", false, "Unpause the monitor")
	apiMonitorsCmd.Flags().BoolVar(&withLatestEvents, "with-events", false, "Include latest events")
	apiMonitorsCmd.Flags().Lookup("pause").NoOptDefVal = "0" // Allow --pause without value
}

func listMonitors(client *lib.APIClient) {
	params := buildQueryParams()
	resp, err := client.GET("/monitors", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list monitors: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Type", "Status", "Alerts"},
		func(data []byte) [][]string {
			var result struct {
				Monitors []struct {
					Key     string `json:"key"`
					Name    string `json:"name"`
					Type    string `json:"type"`
					Passing bool   `json:"passing"`
					Paused  bool   `json:"paused"`
				} `json:"monitors"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.Monitors))
			for i, m := range result.Monitors {
				status := "Passing"
				if !m.Passing {
					status = "Failing"
				}
				alerts := "On"
				if m.Paused {
					alerts = "Muted"
				}
				name := m.Name
				if name == "" {
					name = m.Key
				}
				rows[i] = []string{m.Key, name, m.Type, status, alerts}
			}
			return rows
		})
}

func getMonitor(client *lib.APIClient, key string) {
	params := buildQueryParams()
	if withLatestEvents {
		params["withLatestEvents"] = "true"
	}
	resp, err := client.GET(fmt.Sprintf("/monitors/%s", key), params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get monitor: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Monitor '%s' could not be found", key), 1)
	}

	outputResponse(resp, nil, nil)
}

func createMonitor(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	// Validate JSON
	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	// Check if it's an array (bulk create) or single object
	var testArray []json.RawMessage
	isBulk := json.Unmarshal(body, &testArray) == nil && len(testArray) > 0

	var resp *lib.APIResponse
	var err error
	if isBulk {
		resp, err = client.PUT("/monitors", body, nil)
	} else {
		resp, err = client.POST("/monitors", body, nil)
	}

	if err != nil {
		fatal(fmt.Sprintf("Failed to create monitor: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateMonitor(client *lib.APIClient, key string, jsonData string) {
	// Parse and add key to body
	var bodyMap map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &bodyMap); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}
	bodyMap["key"] = key
	body, _ := json.Marshal(bodyMap)

	// Wrap in array for PUT endpoint
	body = []byte(fmt.Sprintf("[%s]", string(body)))

	resp, err := client.PUT("/monitors", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update monitor: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteMonitor(client *lib.APIClient, key string) {
	resp, err := client.DELETE(fmt.Sprintf("/monitors/%s", key), nil, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to delete monitor: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Monitor '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Monitor '%s' deleted\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}

func pauseMonitor(client *lib.APIClient, key string, hours string) {
	endpoint := fmt.Sprintf("/monitors/%s/pause", key)
	if hours != "" && hours != "0" {
		endpoint = fmt.Sprintf("%s/%s", endpoint, hours)
	}

	resp, err := client.GET(endpoint, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to pause monitor: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Monitor '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		if hours != "" && hours != "0" {
			fmt.Printf("Monitor '%s' paused for %s hours\n", key, hours)
		} else {
			fmt.Printf("Monitor '%s' paused\n", key)
		}
	} else {
		outputResponse(resp, nil, nil)
	}
}

func unpauseMonitor(client *lib.APIClient, key string) {
	resp, err := client.GET(fmt.Sprintf("/monitors/%s/pause/0", key), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to unpause monitor: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Monitor '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Monitor '%s' unpaused\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
