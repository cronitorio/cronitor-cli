package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var notificationCmd = &cobra.Command{
	Use:     "notification",
	Aliases: []string{"notifications"},
	Short:   "Manage notification lists",
	Long: `Manage Cronitor notification lists.

Notification lists define where alerts are sent when monitors fail or recover.
Supported channels: email, slack, pagerduty, opsgenie, victorops, microsoft-teams,
discord, telegram, gchat, larksuite, webhooks, and SMS (phones).

Examples:
  cronitor notification list
  cronitor notification get default
  cronitor notification create "DevOps Team" --emails "dev@example.com,ops@example.com"
  cronitor notification create "Slack Alerts" --slack "#alerts"
  cronitor notification update my-list --name "New Name"
  cronitor notification delete old-list

For full API documentation:
  Humans: https://cronitor.io/docs/notifications-api
  Agents: https://cronitor.io/docs/notifications-api.md`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("API key required. Run 'cronitor configure' or use --api-key flag")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var (
	notificationPage     int
	notificationPageSize int
	notificationFormat   string
	notificationOutput   string
	notificationData     string
)

func init() {
	RootCmd.AddCommand(notificationCmd)
	notificationCmd.PersistentFlags().IntVar(&notificationPage, "page", 1, "Page number")
	notificationCmd.PersistentFlags().IntVar(&notificationPageSize, "page-size", 0, "Number of results per page")
	notificationCmd.PersistentFlags().StringVar(&notificationFormat, "format", "", "Output format: json, table")
	notificationCmd.PersistentFlags().StringVarP(&notificationOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var notificationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all notification lists",
	Long: `List all notification lists.

Examples:
  cronitor notification list
  cronitor notification list --page 2
  cronitor notification list --page-size 100
  cronitor notification list --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if notificationPage > 1 {
			params["page"] = fmt.Sprintf("%d", notificationPage)
		}
		if notificationPageSize > 0 {
			params["pageSize"] = fmt.Sprintf("%d", notificationPageSize)
		}

		resp, err := client.GET("/notifications", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list notification lists: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			Templates []struct {
				Key           string `json:"key"`
				Name          string `json:"name"`
				Notifications struct {
					Emails   []string `json:"emails"`
					Slack    []string `json:"slack"`
					Webhooks []string `json:"webhooks"`
					Phones   []string `json:"phones"`
				} `json:"notifications"`
				Monitors []string `json:"monitors"`
			} `json:"templates"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		format := notificationFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			notificationOutputToTarget(FormatJSON(resp.Body))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "EMAILS", "SLACK", "MONITORS"},
		}

		for _, n := range result.Templates {
			emailCount := fmt.Sprintf("%d", len(n.Notifications.Emails))
			slackCount := fmt.Sprintf("%d", len(n.Notifications.Slack))
			monitorCount := fmt.Sprintf("%d", len(n.Monitors))
			table.Rows = append(table.Rows, []string{n.Name, n.Key, emailCount, slackCount, monitorCount})
		}

		notificationOutputToTarget(table.Render())
	},
}

// --- GET ---
var notificationGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific notification list",
	Long: `Get details for a specific notification list.

Examples:
  cronitor notification get default
  cronitor notification get devops-team
  cronitor notification get my-list --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/notifications/%s", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to get notification list: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Notification list '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		notificationOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var notificationCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new notification list",
	Long: `Create a new notification list.

Examples:
  cronitor notification create --data '{"name":"DevOps Team","notifications":{"emails":["dev@example.com"]}}'
  cronitor notification create --data '{"name":"Slack Alerts","notifications":{"slack":["#alerts"]}}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if notificationData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(notificationData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/notifications", []byte(notificationData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create notification list: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(resp.Body, &result); err == nil {
			Success(fmt.Sprintf("Created notification list: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Notification list created")
		}

		if notificationFormat == "json" {
			notificationOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- UPDATE ---
var notificationUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a notification list",
	Long: `Update an existing notification list.

Examples:
  cronitor notification update my-list --data '{"name":"New Name"}'
  cronitor notification update my-list --data '{"notifications":{"emails":["new@example.com"]}}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if notificationData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(notificationData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/notifications/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update notification list: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Notification '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Notification list '%s' updated", key))
		if notificationFormat == "json" {
			notificationOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var notificationDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a notification list",
	Long: `Delete a notification list.

Note: The default notification list cannot be deleted.

Examples:
  cronitor notification delete old-list`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/notifications/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete notification list: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Notification list '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Notification list '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

func init() {
	notificationCmd.AddCommand(notificationListCmd)
	notificationCmd.AddCommand(notificationGetCmd)
	notificationCmd.AddCommand(notificationCreateCmd)
	notificationCmd.AddCommand(notificationUpdateCmd)
	notificationCmd.AddCommand(notificationDeleteCmd)

	// Create command flags
	notificationCreateCmd.Flags().StringVarP(&notificationData, "data", "d", "", "JSON payload")

	// Update command flags
	notificationUpdateCmd.Flags().StringVarP(&notificationData, "data", "d", "", "JSON payload")
}

func notificationOutputToTarget(content string) {
	if notificationOutput != "" {
		if err := os.WriteFile(notificationOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", notificationOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", notificationOutput))
	} else {
		fmt.Println(content)
	}
}
