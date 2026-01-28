package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	notificationNew    string
	notificationUpdate string
	notificationDelete bool
)

var apiNotificationsCmd = &cobra.Command{
	Use:   "notifications [key]",
	Short: "Manage notification lists",
	Long: `
Manage Cronitor notification lists.

Notification lists define where alerts are sent when monitors detect issues.

Examples:
  List all notification lists:
  $ cronitor api notifications

  Get a specific notification list:
  $ cronitor api notifications <key>

  Create a notification list:
  $ cronitor api notifications --new '{"key":"ops-team","name":"Ops","templates":["email:ops@co.com"]}'

  Update a notification list:
  $ cronitor api notifications <key> --update '{"name":"Updated Name"}'

  Delete a notification list:
  $ cronitor api notifications <key> --delete

  Output as table:
  $ cronitor api notifications --format table
`,
	Run: func(cmd *cobra.Command, args []string) {
		client := getAPIClient()
		key := ""
		if len(args) > 0 {
			key = args[0]
		}

		switch {
		case notificationNew != "":
			createNotification(client, notificationNew)
		case notificationUpdate != "":
			if key == "" {
				fatal("notification list key is required for --update", 1)
			}
			updateNotification(client, key, notificationUpdate)
		case notificationDelete:
			if key == "" {
				fatal("notification list key is required for --delete", 1)
			}
			deleteNotification(client, key)
		case key != "":
			getNotification(client, key)
		default:
			listNotifications(client)
		}
	},
}

func init() {
	apiCmd.AddCommand(apiNotificationsCmd)
	apiNotificationsCmd.Flags().StringVar(&notificationNew, "new", "", "Create notification list with JSON data")
	apiNotificationsCmd.Flags().StringVar(&notificationUpdate, "update", "", "Update notification list with JSON data")
	apiNotificationsCmd.Flags().BoolVar(&notificationDelete, "delete", false, "Delete the notification list")
}

func listNotifications(client *lib.APIClient) {
	params := buildQueryParams()
	resp, err := client.GET("/notification-lists", params)
	if err != nil {
		fatal(fmt.Sprintf("Failed to list notification lists: %s", err), 1)
	}

	outputResponse(resp, []string{"Key", "Name", "Channels"},
		func(data []byte) [][]string {
			var result struct {
				NotificationLists []struct {
					Key       string   `json:"key"`
					Name      string   `json:"name"`
					Templates []string `json:"templates"`
				} `json:"notification_lists"`
			}
			if err := json.Unmarshal(data, &result); err != nil {
				return nil
			}

			rows := make([][]string, len(result.NotificationLists))
			for i, n := range result.NotificationLists {
				channels := fmt.Sprintf("%d", len(n.Templates))
				rows[i] = []string{n.Key, n.Name, channels}
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

func createNotification(client *lib.APIClient, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
	}

	resp, err := client.POST("/notification-lists", body, nil)
	if err != nil {
		fatal(fmt.Sprintf("Failed to create notification list: %s", err), 1)
	}

	outputResponse(resp, nil, nil)
}

func updateNotification(client *lib.APIClient, key string, jsonData string) {
	body := []byte(jsonData)

	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		fatal(fmt.Sprintf("Invalid JSON: %s", err), 1)
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
		fmt.Printf("Notification list '%s' deleted\n", key)
	} else {
		outputResponse(resp, nil, nil)
	}
}
