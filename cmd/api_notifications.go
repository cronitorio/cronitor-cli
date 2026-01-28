package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var apiNotificationsCmd = &cobra.Command{
	Use:   "notifications [action] [key]",
	Short: "Manage notification lists",
	Long: `
Manage Cronitor notification lists.

Notification lists define where alerts are sent when monitors detect issues.
Each list can contain multiple notification channels like email, Slack,
PagerDuty, webhooks, and more.

Actions:
  list     - List all notification lists (default)
  get      - Get a specific notification list by key
  create   - Create a new notification list
  update   - Update an existing notification list
  delete   - Delete a notification list

Examples:
  List all notification lists:
  $ cronitor api notifications

  Get a specific notification list:
  $ cronitor api notifications get <key>

  Create a notification list:
  $ cronitor api notifications create --data '{"key":"ops-team","name":"Ops Team","templates":["email:ops@company.com","slack:#alerts"]}'

  Update a notification list:
  $ cronitor api notifications update <key> --data '{"name":"Updated Name"}'

  Delete a notification list:
  $ cronitor api notifications delete <key>

  Output as table:
  $ cronitor api notifications --format table
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
			listNotifications(client)
		case "get":
			if key == "" {
				fatal("notification list key is required for get action", 1)
			}
			getNotification(client, key)
		case "create":
			createNotification(client)
		case "update":
			if key == "" {
				fatal("notification list key is required for update action", 1)
			}
			updateNotification(client, key)
		case "delete":
			if key == "" {
				fatal("notification list key is required for delete action", 1)
			}
			deleteNotification(client, key)
		default:
			// Treat first arg as a key for get if it doesn't match an action
			getNotification(client, action)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiNotificationsCmd)
}

func listNotifications(client *lib.APIClient) {
	params := buildQueryParams()
	resp, err := client.GET("/notification-lists", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list notification lists: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Channels", "Environments"},
		func(data []byte) [][]string {
			var result struct {
				NotificationLists []struct {
					Key          string   `json:"key"`
					Name         string   `json:"name"`
					Templates    []string `json:"templates"`
					Environments []string `json:"environments"`
				} `json:"notification_lists"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.NotificationLists))
			for i, n := range result.NotificationLists {
				channels := fmt.Sprintf("%d channels", len(n.Templates))
				if len(n.Templates) <= 3 {
					channels = fmt.Sprintf("%v", n.Templates)
				}
				envs := "all"
				if len(n.Environments) > 0 {
					envs = fmt.Sprintf("%v", n.Environments)
				}
				rows[i] = []string{n.Key, n.Name, channels, envs}
			}
			return rows
		})
}

func getNotification(client *lib.APIClient, key string) {
	resp, err := client.GET(fmt.Sprintf("/notification-lists/%s", key), nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to get notification list: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Notification list '%s' could not be found", key), 1)
	}

	outputResponse(resp, nil, nil)
}

func createNotification(client *lib.APIClient) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for create action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.POST("/notification-lists", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create notification list: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateNotification(client *lib.APIClient, key string) {
	body, err := readStdinIfEmpty()
	if err != nil {
		fatal(err.Error(), 1)
	}

	if body == nil {
		fatal("request body is required for update action (use --data, --file, or pipe JSON to stdin)", 1)
	}

	resp, err := client.PUT(fmt.Sprintf("/notification-lists/%s", key), body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to update notification list: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func deleteNotification(client *lib.APIClient, key string) {
	resp, err := client.DELETE(fmt.Sprintf("/notification-lists/%s", key), nil, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to delete notification list: %s", err), 1)
	}

	if resp.IsNotFound() {
		fatal(fmt.Sprintf("Notification list '%s' could not be found", key), 1)
	}

	if resp.IsSuccess() {
		fmt.Printf("Notification list '%s' deleted successfully\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
