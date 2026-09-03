package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var integrationCmd = &cobra.Command{
	Use:     "integration",
	Aliases: []string{"integrations"},
	Short:   "Manage notification integrations",
	Long: `Manage Cronitor notification integrations.

Integrations connect Cronitor to notification services such as Slack, PagerDuty,
Discord, Microsoft Teams, Google Chat, Lark, Opsgenie, VictorOps, Datadog On-Call,
webhooks, and Telegram.

Examples:
  cronitor integration list
  cronitor integration list --service slack
  cronitor integration get slack:12
  cronitor integration services
  cronitor integration create --service discord --name "Alerts" --field url=https://example.com/webhook
  cronitor integration create --data '{"service":"opsgenie","name":"On-call","fields":{"api_key":"..."}}'
  cronitor integration delete discord:44
  cronitor integration delete discord:44 --force

Use 'cronitor connect' to add an integration interactively (OAuth, API key, or Telegram).

For full API documentation:
  Humans: https://cronitor.io/docs/integrations-api
  Agents: https://cronitor.io/docs/integrations-api.md`,
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
	integrationPage       int
	integrationPageSize   int
	integrationFormat     string
	integrationOutput     string
	integrationData       string
	integrationFile       string
	integrationService    string
	integrationName       string
	integrationFields     []string
	integrationIdentifier string
	integrationForce      bool
)

func init() {
	RootCmd.AddCommand(integrationCmd)
	integrationCmd.PersistentFlags().IntVar(&integrationPage, "page", 1, "Page number")
	integrationCmd.PersistentFlags().IntVar(&integrationPageSize, "page-size", 0, "Number of results per page")
	integrationCmd.PersistentFlags().StringVar(&integrationFormat, "format", "", "Output format: json, table")
	integrationCmd.PersistentFlags().StringVarP(&integrationOutput, "output", "o", "", "Write output to file")

	integrationCmd.AddCommand(integrationListCmd)
	integrationCmd.AddCommand(integrationGetCmd)
	integrationCmd.AddCommand(integrationServicesCmd)
	integrationCmd.AddCommand(integrationCreateCmd)
	integrationCmd.AddCommand(integrationDeleteCmd)

	integrationListCmd.Flags().StringVar(&integrationService, "service", "", "Filter by service key (e.g. slack, pagerduty)")

	integrationCreateCmd.Flags().StringVar(&integrationService, "service", "", "Service key (e.g. discord, opsgenie)")
	integrationCreateCmd.Flags().StringVar(&integrationName, "name", "", "Integration name")
	integrationCreateCmd.Flags().StringArrayVar(&integrationFields, "field", nil, "Field value as key=value (repeatable)")
	integrationCreateCmd.Flags().StringVar(&integrationIdentifier, "identifier", "", "Optional integration identifier")
	integrationCreateCmd.Flags().StringVarP(&integrationData, "data", "d", "", "JSON payload")
	integrationCreateCmd.Flags().StringVarP(&integrationFile, "file", "f", "", "JSON file")

	integrationDeleteCmd.Flags().BoolVar(&integrationForce, "force", false, "Force delete (sends force=1)")
}

func resetIntegrationFlags() {
	integrationPage = 1
	integrationPageSize = 0
	integrationFormat = ""
	integrationOutput = ""
	integrationData = ""
	integrationFile = ""
	integrationService = ""
	integrationName = ""
	integrationFields = nil
	integrationIdentifier = ""
	integrationForce = false
}

func integrationOutputToTarget(content string) {
	writeCLIOutput(integrationOutput, content)
}

// --- LIST ---
var integrationListCmd = &cobra.Command{
	Use:   "list",
	Short: "List notification integrations",
	Long: `List notification integrations.

Examples:
  cronitor integration list
  cronitor integration list --service slack
  cronitor integration list --page 2 --page-size 50
  cronitor integration list --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if integrationService != "" {
			params["service"] = integrationService
		}
		if integrationPage > 1 {
			params["page"] = strconv.Itoa(integrationPage)
		}
		if integrationPageSize > 0 {
			params["pageSize"] = strconv.Itoa(integrationPageSize)
		}

		records, raw, err := listIntegrations(client, params)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to list integrations: %s", err))
			return
		}

		format := integrationFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			integrationOutputToTarget(FormatJSON(raw))
			return
		}

		table := &UITable{
			Headers: []string{"ID", "SERVICE", "NAME", "METHOD", "AVAILABLE"},
		}
		for _, rec := range records {
			table.Rows = append(table.Rows, []string{
				rec.ID,
				rec.Service,
				integrationDisplayName(rec),
				rec.Method,
				strconv.FormatBool(rec.Available),
			})
		}
		integrationOutputToTarget(table.Render())
	},
}

// --- GET ---
var integrationGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a notification integration",
	Long: `Get details for a specific integration.

IDs use the form service:pk, for example discord:44 or slack:12.

Examples:
  cronitor integration get slack:12
  cronitor integration get discord:44 --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/integrations/%s", id), nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to get integration: %s", err))
			return
		}
		if resp.IsNotFound() {
			failAndExit(fmt.Sprintf("Integration '%s' not found", id))
			return
		}
		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return
		}

		integrationOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- SERVICES ---
var integrationServicesCmd = &cobra.Command{
	Use:   "services",
	Short: "List available integration services",
	Long: `List the integration service catalogue.

Examples:
  cronitor integration services
  cronitor integration services --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		services, raw, err := fetchCatalogue(client)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to list services: %s", err))
			return
		}

		format := integrationFormat
		if format == "" {
			format = "table"
		}
		if format == "json" {
			integrationOutputToTarget(FormatJSON(raw))
			return
		}
		integrationOutputToTarget(renderServicesTable(services))
	},
}

// --- CREATE ---
var integrationCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a notification integration",
	Long: `Create a notification integration from flags or a JSON payload.

Examples:
  cronitor integration create --service discord --name "Alerts" --field url=https://example.com/webhook
  cronitor integration create --service opsgenie --name "On-call" --field api_key=SECRET --identifier team-a
  cronitor integration create --data '{"service":"webhook","name":"Hook","fields":{"url":"https://example.com"}}'
  cronitor integration create --file integration.json`,
	Run: func(cmd *cobra.Command, args []string) {
		body, err := getIntegrationRequestBody()
		if err != nil {
			failAndExit(err.Error())
			return
		}
		if body == nil {
			failAndExit("Create data required. Use --service/--name/--field, --data, or --file")
			return
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/integrations", body, nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to create integration: %s", err))
			return
		}
		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return
		}

		id, label, service := extractConnectIdentity(resp.Body)
		if id == "" && label == "" {
			Success("Integration created")
		} else {
			display := label
			if display == "" {
				display = id
			}
			if id != "" && label != "" && id != label {
				Success(fmt.Sprintf("Created %s %s (%s)", service, label, id))
			} else {
				Success(fmt.Sprintf("Created integration %s", display))
			}
		}

		if integrationFormat == "json" {
			integrationOutputToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var integrationDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a notification integration",
	Long: `Delete a notification integration.

Examples:
  cronitor integration delete slack:12
  cronitor integration delete discord:44 --force`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id := args[0]
		client := lib.NewAPIClient(dev, log)

		params := map[string]string{}
		if integrationForce {
			params["force"] = "1"
		}

		resp, err := client.DELETE(fmt.Sprintf("/integrations/%s", id), nil, params)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to delete integration: %s", err))
			return
		}
		if resp.IsNotFound() {
			failAndExit(fmt.Sprintf("Integration '%s' not found", id))
			return
		}
		if resp.IsSuccess() {
			Success(fmt.Sprintf("Integration '%s' deleted", id))
			return
		}
		failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
	},
}

func getIntegrationRequestBody() ([]byte, error) {
	if integrationData != "" && integrationFile != "" {
		return nil, errors.New("cannot specify both --data and --file")
	}

	if integrationData != "" {
		var js json.RawMessage
		if err := json.Unmarshal([]byte(integrationData), &js); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return []byte(integrationData), nil
	}

	if integrationFile != "" {
		data, err := os.ReadFile(integrationFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		var js json.RawMessage
		if err := json.Unmarshal(data, &js); err != nil {
			return nil, fmt.Errorf("invalid JSON in file: %w", err)
		}
		return data, nil
	}

	if integrationService == "" {
		return nil, nil
	}

	fields, err := parseFieldFlags(integrationFields)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"service": integrationService,
		"fields":  fields,
	}
	if integrationName != "" {
		payload["name"] = integrationName
	}
	if integrationIdentifier != "" {
		payload["identifier"] = integrationIdentifier
	}
	return json.Marshal(payload)
}
