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

Examples:
  cronitor notification list
  cronitor notification get <key>
  cronitor notification create --data '{"name":"DevOps Team","email":["team@example.com"]}'
  cronitor notification update <key> --data '{"name":"Updated Name"}'
  cronitor notification delete <key>`,
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
	notificationPage   int
	notificationFormat string
	notificationOutput string
	notificationData   string
	notificationFile   string
)

func init() {
	RootCmd.AddCommand(notificationCmd)
	notificationCmd.PersistentFlags().IntVar(&notificationPage, "page", 1, "Page number")
	notificationCmd.PersistentFlags().StringVar(&notificationFormat, "format", "", "Output format: json, table")
	notificationCmd.PersistentFlags().StringVarP(&notificationOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var notificationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all notification lists",
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if notificationPage > 1 {
			params["page"] = fmt.Sprintf("%d", notificationPage)
		}

		resp, err := client.GET("/notification-lists", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list notification lists: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			NotificationLists []struct {
				Key      string   `json:"key"`
				Name     string   `json:"name"`
				Emails   []string `json:"emails"`
				Webhooks []string `json:"webhooks"`
			} `json:"notification_lists"`
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
			Headers: []string{"KEY", "NAME", "EMAILS", "WEBHOOKS"},
		}

		for _, n := range result.NotificationLists {
			emailCount := fmt.Sprintf("%d", len(n.Emails))
			webhookCount := fmt.Sprintf("%d", len(n.Webhooks))
			table.Rows = append(table.Rows, []string{n.Key, n.Name, emailCount, webhookCount})
		}

		notificationOutputToTarget(table.Render())
	},
}

// --- GET ---
var notificationGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific notification list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/notification-lists/%s", key), nil)
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
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getNotificationRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/notification-lists", body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create notification list: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success("Notification list created")
		notificationOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- UPDATE ---
var notificationUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a notification list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		body, err := getNotificationRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/notification-lists/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update notification list: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Notification list '%s' updated", key))
		notificationOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- DELETE ---
var notificationDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a notification list",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/notification-lists/%s", key), nil, nil)
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

	notificationCreateCmd.Flags().StringVarP(&notificationData, "data", "d", "", "JSON data")
	notificationCreateCmd.Flags().StringVarP(&notificationFile, "file", "f", "", "JSON file")
	notificationUpdateCmd.Flags().StringVarP(&notificationData, "data", "d", "", "JSON data")
	notificationUpdateCmd.Flags().StringVarP(&notificationFile, "file", "f", "", "JSON file")
}

func getNotificationRequestBody() ([]byte, error) {
	if notificationData != "" && notificationFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if notificationData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(notificationData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(notificationData), nil
	}

	if notificationFile != "" {
		data, err := os.ReadFile(notificationFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return data, nil
	}

	return nil, nil
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
