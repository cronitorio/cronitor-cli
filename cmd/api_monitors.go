package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var pauseHours string
var withLatestEvents bool

var apiMonitorsCmd = &cobra.Command{
	Use:   "monitors [action] [key]",
	Short: "Manage monitors",
	Long: `
Manage Cronitor monitors (jobs, checks, heartbeats, sites).

Actions:
  list     - List all monitors (default)
  get      - Get a specific monitor by key
  create   - Create a new monitor
  update   - Update an existing monitor
  delete   - Delete one or more monitors
  pause    - Pause a monitor (stop alerting)
  unpause  - Unpause a monitor (resume alerting)

Examples:
  List all monitors:
  $ cronitor api monitors

  List monitors with pagination:
  $ cronitor api monitors --page 2

  Get a specific monitor:
  $ cronitor api monitors get my-job-key

  Create a monitor from JSON data:
  $ cronitor api monitors create --data '{"key":"my-job","type":"job","schedule":"0 * * * *"}'

  Create a monitor from a file:
  $ cronitor api monitors create --file monitor.json

  Update a monitor:
  $ cronitor api monitors update my-job-key --data '{"name":"Updated Name"}'

  Delete a monitor:
  $ cronitor api monitors delete my-job-key

  Delete multiple monitors:
  $ cronitor api monitors delete --data '["key1","key2","key3"]'

  Pause a monitor for 24 hours:
  $ cronitor api monitors pause my-job-key --hours 24

  Pause a monitor indefinitely:
  $ cronitor api monitors pause my-job-key

  Unpause a monitor:
  $ cronitor api monitors unpause my-job-key

  Output as table:
  $ cronitor api monitors --format table

  Get a monitor with latest events:
  $ cronitor api monitors get my-job-key --with-events
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
			listMonitors(client)
		case "get":
			if key == "" {
				fatal("monitor key is required for get action", 1)
			}
			getMonitor(client, key)
		case "create":
			createMonitor(client)
		case "update":
			if key == "" {
				fatal("monitor key is required for update action", 1)
			}
			updateMonitor(client, key)
		case "delete":
			deleteMonitors(client, key)
		case "pause":
			if key == "" {
				fatal("monitor key is required for pause action", 1)
			}
			pauseMonitor(client, key)
		case "unpause":
			if key == "" {
				fatal("monitor key is required for unpause action", 1)
			}
			unpauseMonitor(client, key)
		default:
			// Treat first arg as a key for get if it doesn't match an action
			getMonitor(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiMonitorsCmd)
	apiMonitorsCmd.Flags().StringVar(&pauseHours, "hours", "", "Number of hours to pause (for pause action)")
	apiMonitorsCmd.Flags().BoolVar(&withLatestEvents, "with-events", false, "Include latest events in monitor response")
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

func createMonitor(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	// Check if it's an array (bulk create) or single object
	var testArray []json.RawMessage
	isBulk := json.Unmarshal(body, &testArray) == nil && len(testArray) > 0

	var resp *lib.APIResponse
	if isBulk {
		// Bulk create uses PUT
		resp, err = client.PUT("/monitors", body, nil)
	} else {
		// Single create uses POST
		resp, err = client.POST("/monitors", body, nil)
	}

	if err != nil {
		fatal(fmt.Sprintf("Failed to create monitor(s): %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateMonitor(client *lib.APIClient, key string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	// Ensure key is in the body
	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}
	bodyMap["key"] = key
	body, _ = json.Marshal(bodyMap)

	// Wrap in array for PUT endpoint
	body = []byte(fmt.Sprintf("[%s]", string(body)))

	resp, err := client.PUT("/monitors", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update monitor: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteMonitors(client *lib.APIClient, key string) {
	var resp *lib.APIResponse
	var err error

	if key != "" {
		// Single delete
		resp, err = client.DELETE(fmt.Sprintf("/monitors/%s", key), nil, nil)
	} else {
		// Bulk delete requires body
		body, bodyErr := readStdinIfEmpty()
		if bodyErr != nil {
			fatal(bodyErr.Error(), 1)
		}

		if body == nil {
			fatal("monitor key or JSON array of keys is required for delete action", 1)
		}

		resp, err = client.DELETE("/monitors", body, nil)
	}

	if err != nil {
		fatal(fmt.Sprintf("Failed to delete monitor(s): %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Monitor '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		if key != "" {
			fmt.Printf("Monitor '%s' deleted successfully\n", key)
		} else {
			fmt.Println("Monitors deleted successfully")
		}
	} else {
		outputResponse(resp, nil, nil)
	}
}

func pauseMonitor(client *lib.APIClient, key string) {
	endpoint := fmt.Sprintf("/monitors/%s/pause", key)
	if pauseHours != "" {
		endpoint = fmt.Sprintf("%s/%s", endpoint, pauseHours)
	}

	resp, err := client.GET(endpoint, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to pause monitor: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Monitor '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		if pauseHours != "" {
			fmt.Printf("Monitor '%s' paused for %s hours\n", key, pauseHours)
		} else {
			fmt.Printf("Monitor '%s' paused indefinitely\n", key)
		}
	} else {
		outputResponse(resp, nil, nil)
	}
}

func unpauseMonitor(client *lib.APIClient, key string) {
	endpoint := fmt.Sprintf("/monitors/%s/pause/0", key)

	resp, err := client.GET(endpoint, nil)
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
