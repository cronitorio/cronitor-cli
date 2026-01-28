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
	Use:     "environment",
	Aliases: []string{"env"},
	Short:   "Manage environments",
	Long: `Manage Cronitor environments.

Examples:
  cronitor environment list
  cronitor environment get <key>
  cronitor environment create --data '{"name":"Production","key":"production"}'
  cronitor environment update <key> --data '{"name":"Updated Name"}'
  cronitor environment delete <key>`,
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
	environmentPage   int
	environmentFormat string
	environmentOutput string
	environmentData   string
	environmentFile   string
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
				Key  string `json:"key"`
				Name string `json:"name"`
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
			Headers: []string{"KEY", "NAME"},
		}

		for _, e := range result.Environments {
			table.Rows = append(table.Rows, []string{e.Key, e.Name})
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
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getEnvironmentRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/environments", body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create environment: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success("Environment created")
		environmentOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- UPDATE ---
var environmentUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an environment",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		body, err := getEnvironmentRequestBody()
		if err != nil {
			Error(err.Error())
			os.Exit(1)
		}
		if body == nil {
			Error("JSON data required. Use --data or --file")
			os.Exit(1)
		}

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
		environmentOutputToTarget(FormatJSON(resp.Body))
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

	environmentCreateCmd.Flags().StringVarP(&environmentData, "data", "d", "", "JSON data")
	environmentCreateCmd.Flags().StringVarP(&environmentFile, "file", "f", "", "JSON file")
	environmentUpdateCmd.Flags().StringVarP(&environmentData, "data", "d", "", "JSON data")
	environmentUpdateCmd.Flags().StringVarP(&environmentFile, "file", "f", "", "JSON file")
}

func getEnvironmentRequestBody() ([]byte, error) {
	if environmentData != "" && environmentFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if environmentData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(environmentData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(environmentData), nil
	}

	if environmentFile != "" {
		data, err := os.ReadFile(environmentFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return data, nil
	}

	return nil, nil
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
