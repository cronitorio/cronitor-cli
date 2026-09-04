package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

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
  cronitor integration get Workspace
  cronitor integration get Alerts --service slack
  cronitor integration services
  cronitor integration create --service discord --name "Alerts" --field key=https://example.com/webhook
  cronitor integration create --data '{"service":"opsgenie","name":"On-call","fields":{"key":"..."}}'
  cronitor integration delete Alerts
  cronitor integration delete Alerts --force

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
	integrationCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		resetSecretRedaction()
	}
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

	integrationGetCmd.Flags().StringVar(&integrationService, "service", "", "Service key to disambiguate the label")
	integrationDeleteCmd.Flags().BoolVar(&integrationForce, "force", false, "Force delete (sends force=1)")
	integrationDeleteCmd.Flags().StringVar(&integrationService, "service", "", "Service key to disambiguate the label")
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
		client := newIntegrationAPIClient()
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
			Headers: []string{"LABEL", "SERVICE", "METHOD", "AVAILABLE"},
		}
		for _, rec := range records {
			table.Rows = append(table.Rows, []string{
				integrationPublicLabel(rec),
				rec.Service,
				rec.Method,
				strconv.FormatBool(rec.Available),
			})
		}
		integrationOutputToTarget(table.Render())
	},
}

// --- GET ---
var integrationGetCmd = &cobra.Command{
	Use:   "get <label>",
	Short: "Get a notification integration by unique label",
	Long: `Get details for a specific integration.

Address integrations by unique label (unique per org, service, and visibility).
Pass --service when the same label exists on more than one service. The CLI
filters GET /integrations by service and label and prints the single match.

Examples:
  cronitor integration get Workspace
  cronitor integration get Alerts --service slack
  cronitor integration get Alerts --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		label := args[0]
		client := newIntegrationAPIClient()

		_, raw, err := resolveIntegrationByLabel(client, label, integrationService)
		if err != nil {
			failAndExit(integrationLookupErrorMessage(err, "get"))
			return
		}

		integrationOutputToTarget(FormatJSON(raw))
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
		client := newIntegrationAPIClient()
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
  cronitor integration create --service discord --name "Alerts" --field key=https://example.com/webhook
  cronitor integration create --service opsgenie --name "On-call" --field key=SECRET --identifier team-a
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

		client := newIntegrationAPIClient()
		resp, err := client.POST("/integrations", body, nil)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to create integration: %s", err))
			return
		}
		if !resp.IsSuccess() {
			failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			return
		}

		if integrationFormat == "json" {
			if len(resp.Body) > 0 {
				integrationOutputToTarget(FormatJSON(resp.Body))
			}
			return
		}

		label, service := extractPublicIdentity(resp.Body)
		if label == "" && service == "" {
			Success("Integration created")
			return
		}
		if label != "" && service != "" {
			Success(fmt.Sprintf("Created %s %s", service, label))
			return
		}
		if label != "" {
			Success(fmt.Sprintf("Created %s", label))
			return
		}
		Success(fmt.Sprintf("Created %s", service))
	},
}

// --- DELETE ---
var integrationDeleteCmd = &cobra.Command{
	Use:   "delete <label>",
	Short: "Delete a notification integration by unique label",
	Long: `Delete a notification integration.

Address integrations by unique label. Pass --service when the same label
exists on more than one service; otherwise the CLI looks the service up first.
The delete fails with the lists and monitors that still use the integration
unless --force is set.

Examples:
  cronitor integration delete Workspace
  cronitor integration delete Alerts --force`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		label := args[0]
		client := newIntegrationAPIClient()

		service := integrationService
		if service == "" {
			rec, _, err := resolveIntegrationByLabel(client, label, "")
			if err != nil {
				failAndExit(integrationLookupErrorMessage(err, "delete"))
				return
			}
			service = rec.Service
		}

		params := map[string]string{"service": service, "label": label}
		if integrationForce {
			params["force"] = "1"
		}

		resp, err := client.DELETE("/integrations", nil, params)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to delete integration: %s", err))
			return
		}
		if resp.IsNotFound() {
			failAndExit(fmt.Sprintf("Integration '%s' not found", label))
			return
		}
		if resp.StatusCode == 409 {
			failAndExit(integrationInUseMessage(label, resp.Body))
			return
		}
		if resp.IsSuccess() {
			Success(fmt.Sprintf("Integration '%s' deleted", label))
			return
		}
		failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
	},
}

func integrationLookupErrorMessage(err error, verb string) string {
	switch err.(type) {
	case integrationNotFoundError, integrationAmbiguousError:
		return err.Error()
	default:
		return fmt.Sprintf("Failed to %s integration: %s", verb, err)
	}
}

// integrationInUseMessage renders the 409 in_use body from DELETE /integrations.
func integrationInUseMessage(label string, body []byte) string {
	var payload struct {
		Error             string   `json:"error"`
		NotificationLists []string `json:"notification_lists"`
		Monitors          []string `json:"monitors"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Error != "in_use" {
		return fmt.Sprintf("API Error (409): %s", string(body))
	}
	var uses []string
	if len(payload.NotificationLists) > 0 {
		uses = append(uses, fmt.Sprintf("notification lists: %s", strings.Join(payload.NotificationLists, ", ")))
	}
	if len(payload.Monitors) > 0 {
		uses = append(uses, fmt.Sprintf("monitors: %s", strings.Join(payload.Monitors, ", ")))
	}
	msg := fmt.Sprintf("Integration '%s' is in use", label)
	if len(uses) > 0 {
		msg += " by " + strings.Join(uses, "; ")
	}
	return msg + ". Use --force to detach it and delete anyway"
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
