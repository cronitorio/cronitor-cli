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

var environmentCmd = &cobra.Command{
	GroupID: GroupAPI,
	Use:     "environment",
	Aliases: []string{"env"},
	Short:   "Manage environments",
	Long: `Manage Cronitor environments.

Environments allow you to separate monitors by deployment stage (production, staging, etc.)
and control which environments trigger alerts.

Examples:
  cronitor environment list
  cronitor environment get production
  cronitor environment create staging --name "Staging" --no-alerts
  cronitor environment create production --name "Production" --with-alerts
  cronitor environment update staging --name "QA Environment"
  cronitor environment delete old-env

For full API documentation:
  Humans: https://cronitor.io/docs/environments-api
  Agents: https://cronitor.io/docs/environments-api.md`,
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
	environmentPage     int
	environmentFormat   string
	environmentOutput   string
	environmentData     string
)

func init() {
	RootCmd.AddCommand(environmentCmd)
	environmentCmd.PersistentFlags().IntVar(&environmentPage, "page", 1, "Page number")
	environmentCmd.PersistentFlags().StringVar(&environmentFormat, "format", "", "Output format: json, table")
	environmentCmd.PersistentFlags().StringVarP(&environmentOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var environmentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all environments",
	Long: `List all environments.

Examples:
  cronitor environment list
  cronitor environment list --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if environmentPage > 1 {
			params["page"] = fmt.Sprintf("%d", environmentPage)
		}

		resp, err := client.GET("/environments", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list environments: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			Environments []struct {
				Key            string `json:"key"`
				Name           string `json:"name"`
				WithAlerts     bool   `json:"with_alerts"`
				Default        bool   `json:"default"`
				ActiveMonitors int    `json:"active_monitors"`
			} `json:"environments"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		format := environmentFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			environmentOutputToTarget(FormatJSON(resp.Body))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "ALERTS", "MONITORS", "DEFAULT"},
		}

		for _, e := range result.Environments {
			alerts := mutedStyle.Render("off")
			if e.WithAlerts {
				alerts = successStyle.Render("on")
			}
			isDefault := ""
			if e.Default {
				isDefault = "yes"
			}
			monitors := fmt.Sprintf("%d", e.ActiveMonitors)
			table.Rows = append(table.Rows, []string{e.Name, e.Key, alerts, monitors, isDefault})
		}

		environmentOutputToTarget(table.Render())
	},
}

// --- GET ---
var environmentGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific environment",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/environments/%s", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to get environment: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Environment '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		environmentOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var environmentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new environment",
	Long: `Create a new environment.

Examples:
  cronitor environment create --data '{"key":"staging","name":"Staging Environment"}'
  cronitor environment create --data '{"key":"production","name":"Production","with_alerts":true}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if environmentData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(environmentData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/environments", []byte(environmentData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create environment: %s", err))
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
			Success(fmt.Sprintf("Created environment: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Environment created")
		}

		if environmentFormat == "json" {
			environmentOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- UPDATE ---
var environmentUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an environment",
	Long: `Update an existing environment.

Examples:
  cronitor environment update staging --data '{"name":"Staging Environment"}'
  cronitor environment update production --data '{"with_alerts":true}'
  cronitor environment update dev --data '{"with_alerts":false}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if environmentData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(environmentData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/environments/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update environment: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Environment '%s' updated", key))
		if environmentFormat == "json" {
			environmentOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var environmentDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete an environment",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/environments/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete environment: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Environment '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Environment '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

func init() {
	environmentCmd.AddCommand(environmentListCmd)
	environmentCmd.AddCommand(environmentGetCmd)
	environmentCmd.AddCommand(environmentCreateCmd)
	environmentCmd.AddCommand(environmentUpdateCmd)
	environmentCmd.AddCommand(environmentDeleteCmd)

	// Create flags
	environmentCreateCmd.Flags().StringVarP(&environmentData, "data", "d", "", "JSON payload")

	// Update flags
	environmentUpdateCmd.Flags().StringVarP(&environmentData, "data", "d", "", "JSON payload")
}

func environmentOutputToTarget(content string) {
	if environmentOutput != "" {
		if err := os.WriteFile(environmentOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", environmentOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", environmentOutput))
	} else {
		fmt.Println(content)
	}
}
