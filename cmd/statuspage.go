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

var statuspageCmd = &cobra.Command{
	Use:   "statuspage",
	Short: "Manage status pages",
	Long: `Manage Cronitor status pages.

Examples:
  cronitor statuspage list
  cronitor statuspage get <key>
  cronitor statuspage create --data '{"name":"My Status Page"}'
  cronitor statuspage update <key> --data '{"name":"New Name"}'
  cronitor statuspage delete <key>`,
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
	statuspagePage   int
	statuspageFormat string
	statuspageOutput string
	statuspageData   string
	statuspageFile   string
)

func init() {
	RootCmd.AddCommand(statuspageCmd)
	statuspageCmd.PersistentFlags().IntVar(&statuspagePage, "page", 1, "Page number")
	statuspageCmd.PersistentFlags().StringVar(&statuspageFormat, "format", "", "Output format: json, table")
	statuspageCmd.PersistentFlags().StringVarP(&statuspageOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var statuspageListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all status pages",
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if statuspagePage > 1 {
			params["page"] = fmt.Sprintf("%d", statuspagePage)
		}

		resp, err := client.GET("/statuspages", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list status pages: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		var result struct {
			StatusPages []struct {
				Key       string `json:"key"`
				Name      string `json:"name"`
				Subdomain string `json:"subdomain"`
				Status    string `json:"status"`
			} `json:"statuspages"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		format := statuspageFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			statuspageOutputToTarget(FormatJSON(resp.Body))
			return
		}

		table := &UITable{
			Headers: []string{"KEY", "NAME", "SUBDOMAIN", "STATUS"},
		}

		for _, sp := range result.StatusPages {
			status := successStyle.Render(sp.Status)
			if sp.Status != "operational" {
				status = warningStyle.Render(sp.Status)
			}
			table.Rows = append(table.Rows, []string{sp.Key, sp.Name, sp.Subdomain, status})
		}

		statuspageOutputToTarget(table.Render())
	},
}

// --- GET ---
var statuspageGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific status page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/statuspages/%s", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to get status page: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Status page '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		statuspageOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var statuspageCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new status page",
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getStatuspageRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/statuspages", body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create status page: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success("Status page created")
		statuspageOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- UPDATE ---
var statuspageUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a status page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		body, err := getStatuspageRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/statuspages/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update status page: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Status page '%s' updated", key))
		statuspageOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- DELETE ---
var statuspageDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a status page",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/statuspages/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete status page: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Status page '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Status page '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

func init() {
	statuspageCmd.AddCommand(statuspageListCmd)
	statuspageCmd.AddCommand(statuspageGetCmd)
	statuspageCmd.AddCommand(statuspageCreateCmd)
	statuspageCmd.AddCommand(statuspageUpdateCmd)
	statuspageCmd.AddCommand(statuspageDeleteCmd)

	statuspageCreateCmd.Flags().StringVarP(&statuspageData, "data", "d", "", "JSON data")
	statuspageCreateCmd.Flags().StringVarP(&statuspageFile, "file", "f", "", "JSON file")
	statuspageUpdateCmd.Flags().StringVarP(&statuspageData, "data", "d", "", "JSON data")
	statuspageUpdateCmd.Flags().StringVarP(&statuspageFile, "file", "f", "", "JSON file")
}

func getStatuspageRequestBody() ([]byte, error) {
	if statuspageData != "" && statuspageFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if statuspageData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(statuspageData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(statuspageData), nil
	}

	if statuspageFile != "" {
		data, err := os.ReadFile(statuspageFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return data, nil
	}

	return nil, nil
}

func statuspageOutputToTarget(content string) {
	if statuspageOutput != "" {
		if err := os.WriteFile(statuspageOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to %s: %s", statuspageOutput, err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", statuspageOutput))
	} else {
		fmt.Println(content)
	}
}
