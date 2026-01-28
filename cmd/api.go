package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// API command flags
var (
	apiData       string
	apiFile       string
	apiFormat     string
	apiPage       int
	apiEnv        string
	apiMonitor    string
	apiOutput     string
	apiRaw        bool
)

// apiCmd represents the api command
var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Interact with the Cronitor API",
	Long: `
Interact with the Cronitor API to manage monitors, issues, status pages, and more.

This command provides access to all Cronitor API resources:
  monitors       - Manage monitors (jobs, checks, heartbeats, sites)
  issues         - Manage issues and incidents
  statuspages    - Manage status pages
  components     - Manage status page components
  incidents      - Manage status page incidents
  metrics        - View monitor metrics and performance data
  notifications  - Manage notification lists
  environments   - Manage environments

Examples:
  List all monitors:
  $ cronitor api monitors

  Get a specific monitor:
  $ cronitor api monitors get <key>

  Get a monitor with latest events:
  $ cronitor api monitors get <key> --with-events

  Create a new monitor:
  $ cronitor api monitors create --data '{"key":"my-job","type":"job"}'

  Update a monitor:
  $ cronitor api monitors update <key> --data '{"name":"Updated Name"}'

  Delete a monitor:
  $ cronitor api monitors delete <key>

  List issues:
  $ cronitor api issues

  Get metrics for a monitor:
  $ cronitor api metrics --monitor <key>
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("you must provide an API key with this command or save a key using 'cronitor configure'")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	RootCmd.AddCommand(apiCmd)

	// Global API flags
	apiCmd.PersistentFlags().StringVarP(&apiData, "data", "d", "", "JSON data for create/update operations")
	apiCmd.PersistentFlags().StringVarP(&apiFile, "file", "f", "", "JSON file for create/update operations")
	apiCmd.PersistentFlags().StringVar(&apiFormat, "format", "json", "Output format: json, table")
	apiCmd.PersistentFlags().IntVar(&apiPage, "page", 1, "Page number for paginated results")
	apiCmd.PersistentFlags().StringVar(&apiEnv, "env", "", "Filter by environment")
	apiCmd.PersistentFlags().StringVar(&apiMonitor, "monitor", "", "Filter by monitor key")
	apiCmd.PersistentFlags().StringVarP(&apiOutput, "output", "o", "", "Output to file instead of stdout")
	apiCmd.PersistentFlags().BoolVar(&apiRaw, "raw", false, "Output raw JSON without formatting")
}

// getAPIClient returns a configured API client
func getAPIClient() *lib.APIClient {
	return lib.NewAPIClient(dev, log)
}

// getRequestBody returns the request body from --data or --file flag
func getRequestBody() ([]byte, error) {
	if apiData != "" && apiFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if apiData != "" {
		// Validate JSON
		var js json.RawMessage
		if err := json.Unmarshal([]byte(apiData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON in --data: %w", err)
		}
		return []byte(apiData), nil
	}

	if apiFile != "" {
		data, err := os.ReadFile(apiFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", apiFile, err)
		}
		// Validate JSON
		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			return nil, fmt.Errorf("invalid JSON in file %s: %w", apiFile, err)
		}
		return data, nil
	}

	return nil, nil
}

// outputResponse outputs the API response in the requested format
func outputResponse(resp *lib.APIResponse, tableHeaders []string, tableExtractor func([]byte) [][]string) {
	if !resp.IsSuccess() {
		fatal(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()), 1)
	}

	var output string
	if apiFormat == "table" && tableHeaders != nil && tableExtractor != nil {
		output = formatAsTable(resp.Body, tableHeaders, tableExtractor)
	} else if apiRaw {
		output = string(resp.Body)
	} else {
		output = resp.FormatJSON()
	}

	writeOutput(output)
}

// formatAsTable formats the response as a table
func formatAsTable(data []byte, headers []string, extractor func([]byte) [][]string) string {
	rows := extractor(data)
	if rows == nil {
		return string(data)
	}

	var buf strings.Builder
	table := tablewriter.NewWriter(&buf)
	table.SetHeader(headers)
	table.SetAutoWrapText(false)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.AppendBulk(rows)
	table.Render()

	return buf.String()
}

// writeOutput writes the output to stdout or a file
func writeOutput(output string) {
	if apiOutput != "" {
		if err := os.WriteFile(apiOutput, []byte(output), 0644); err != nil {
			fatal(fmt.Sprintf("Failed to write to file %s: %s", apiOutput, err), 1)
		}
		fmt.Printf("Output written to %s\n", apiOutput)
	} else {
		fmt.Println(output)
	}
}

// readStdinIfEmpty reads from stdin if no data is provided
func readStdinIfEmpty() ([]byte, error) {
	body, err := getRequestBody()
	if err != nil {
		return nil, err
	}

	if body == nil {
		// Check if stdin has data
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			body, err = io.ReadAll(os.Stdin)
			if err != nil {
				return nil, fmt.Errorf("failed to read from stdin: %w", err)
			}
			// Validate JSON
			var js json.RawMessage
			if err := json.Unmarshal(body, &js); err != nil {
				return nil, fmt.Errorf("invalid JSON from stdin: %w", err)
			}
		}
	}

	return body, nil
}

// buildQueryParams builds query parameters from common flags
func buildQueryParams() map[string]string {
	params := make(map[string]string)
	if apiPage > 1 {
		params["page"] = strconv.Itoa(apiPage)
	}
	if apiEnv != "" {
		params["env"] = apiEnv
	}
	if apiMonitor != "" {
		params["monitor"] = apiMonitor
	}
	return params
}
