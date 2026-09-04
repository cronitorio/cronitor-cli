package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var connectCmd = &cobra.Command{
	Use:   "connect [service]",
	Short: "Connect a notification integration",
	Long: `Connect a notification integration.

With no service argument, lists the integration catalogue (service, method, available).

OAuth services (slack, pagerduty):
  Starts a connect session, prints the authorize URL, opens a browser unless
  --no-browser is set, then polls until the integration is complete.

API-key services (discord, microsoft-teams, gchat, larksuite, opsgenie,
victorops, datadog-on-call, webhook):
  Prompts for catalogue fields unless provided with --field, then creates the
  integration. Secret values are not echoed. Fields the catalogue marks
  "(optional)" can be left empty.

Telegram:
  Prints bot instructions and waits for a new Telegram integration. A match is
  a new label versus the pre-connect snapshot. If --name is set, that new
  label must also match.

Service names are catalogue keys. Friendly aliases are not accepted.

Examples:
  cronitor connect
  cronitor connect slack
  cronitor connect slack --no-browser
  cronitor connect pagerduty --timeout 10m --add-to default
  cronitor connect opsgenie --name "On-call" --field key=SECRET
  cronitor connect telegram
  cronitor connect telegram --name "On-call bot"`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("API key required. Run 'cronitor configure' or use --api-key flag")
		}
		if len(args) > 1 {
			return errors.New("accepts at most one service argument")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		resetSecretRedaction()
		client := newIntegrationAPIClient()
		if len(args) == 0 {
			runConnectCatalogue(client)
			return
		}
		runConnectService(client, args[0])
	},
}

var (
	connectName       string
	connectFields     []string
	connectNoBrowser  bool
	connectTimeout    string
	connectAddTo      string
	connectFormat     string
	connectOutput     string
	connectIdentifier string
)

func init() {
	RootCmd.AddCommand(connectCmd)
	connectCmd.Flags().StringVar(&connectName, "name", "", "Integration name (used for create and Telegram matching)")
	connectCmd.Flags().StringArrayVar(&connectFields, "field", nil, "Field value as key=value (repeatable)")
	connectCmd.Flags().BoolVar(&connectNoBrowser, "no-browser", false, "Print the authorize URL without opening a browser")
	connectCmd.Flags().StringVar(&connectTimeout, "timeout", "15m", "How long to wait for OAuth or Telegram (e.g. 15m, 30s)")
	connectCmd.Flags().StringVar(&connectAddTo, "add-to", "", "Notification list key to append the new integration to")
	connectCmd.Flags().StringVar(&connectFormat, "format", "", "Output format: json, table")
	connectCmd.Flags().StringVarP(&connectOutput, "output", "o", "", "Write output to file")
	connectCmd.Flags().StringVar(&connectIdentifier, "identifier", "", "Optional identifier for API-key integrations")
}

func resetConnectFlags() {
	connectName = ""
	connectFields = nil
	connectNoBrowser = false
	connectTimeout = "15m"
	connectAddTo = ""
	connectFormat = ""
	connectOutput = ""
	connectIdentifier = ""
}

func runConnectCatalogue(client *lib.APIClient) {
	services, raw, err := fetchCatalogue(client)
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to list services: %s", err))
		return
	}

	format := connectFormat
	if format == "" {
		format = "table"
	}
	if format == "json" {
		writeCLIOutput(connectOutput, FormatJSON(raw))
		return
	}
	writeCLIOutput(connectOutput, renderServicesTable(services))
}

func runConnectService(client *lib.APIClient, serviceKey string) {
	services, _, err := fetchCatalogue(client)
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to load services catalogue: %s", err))
		return
	}

	svc, ok := findCatalogueService(services, serviceKey)
	if !ok {
		failAndExit(fmt.Sprintf("Unknown service %q. Available: %s", serviceKey, strings.Join(catalogueServiceKeys(services), ", ")))
		return
	}

	switch connectFlowFor(svc) {
	case "oauth":
		runOAuthConnect(client, svc)
	case "telegram":
		runTelegramConnect(client, svc)
	default:
		runAPIKeyConnect(client, svc)
	}
}

func runOAuthConnect(client *lib.APIClient, svc catalogueService) {
	body, _ := json.Marshal(map[string]string{"service": svc.Service})
	resp, err := client.POST("/integrations/connect", body, nil)
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to start connect session: %s", err))
		return
	}
	if !resp.IsSuccess() {
		failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
		return
	}

	var start struct {
		AuthorizeURL string  `json:"authorize_url"`
		Token        string  `json:"token"`
		ExpiresAt    string  `json:"expires_at"`
		PollInterval float64 `json:"poll_interval"`
	}
	if err := json.Unmarshal(resp.Body, &start); err != nil {
		failAndExit(fmt.Sprintf("Failed to parse connect session: %s", err))
		return
	}
	if start.AuthorizeURL == "" || start.Token == "" {
		failAndExit("Connect session did not include authorize_url and token")
		return
	}

	connectHumanPrintln(start.AuthorizeURL)

	if !connectNoBrowser {
		openBrowserFn(start.AuthorizeURL)
	}

	timeout, err := parseTimeoutFlag(connectTimeout)
	if err != nil {
		failAndExit(err.Error())
		return
	}

	raw, rec, err := pollConnectSession(client, start.Token, defaultPollInterval(secondsToDuration(start.PollInterval)), timeout, parseExpiresAt(start.ExpiresAt))
	if err != nil {
		failAndExit(err.Error())
		return
	}

	label, service := rec.Label, rec.Service
	if label == "" {
		label = rec.Name
	}
	if service == "" {
		service = svc.Service
	}
	printConnected(label, service, raw, connectFormat, connectOutput)
	confirmAddTo(client, service, label)
}

func pollConnectSession(client *lib.APIClient, token string, interval, timeout time.Duration, expiresAt time.Time) ([]byte, integrationRecord, error) {
	deadline := nowFn().Add(timeout)
	if !expiresAt.IsZero() && expiresAt.Before(deadline) {
		deadline = expiresAt
	}

	for {
		if !nowFn().Before(deadline) {
			return nil, integrationRecord{}, fmt.Errorf("connect timed out")
		}

		resp, err := client.GET(fmt.Sprintf("/integrations/connect/%s", token), nil)
		if err != nil {
			return nil, integrationRecord{}, fmt.Errorf("failed to poll connect session: %w", err)
		}
		if !resp.IsSuccess() {
			return nil, integrationRecord{}, fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
		}

		switch parseConnectStatus(resp.Body) {
		case "complete":
			label, service := extractPublicIdentity(resp.Body)
			return resp.Body, integrationRecord{Label: label, Name: label, Service: service}, nil
		case "failed":
			return nil, integrationRecord{}, fmt.Errorf("connect failed: %s", connectStatusMessage(resp.Body))
		case "expired":
			return nil, integrationRecord{}, fmt.Errorf("connect session expired")
		}

		// pending or any other non-terminal status: keep polling
		sleepForPoll(interval, deadline)
		if !nowFn().Before(deadline) {
			return nil, integrationRecord{}, fmt.Errorf("connect timed out")
		}
	}
}

func parseConnectStatus(body []byte) string {
	var payload struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(payload.Status))
}

func connectStatusMessage(body []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &payload) == nil && payload.Error != "" {
		return payload.Error
	}
	return "session failed"
}

func sleepForPoll(interval time.Duration, deadline time.Time) {
	remaining := deadline.Sub(nowFn())
	if remaining <= 0 {
		return
	}
	if interval > remaining {
		interval = remaining
	}
	if interval > 0 {
		sleepFn(interval)
	}
}

func runAPIKeyConnect(client *lib.APIClient, svc catalogueService) {
	provided, err := parseFieldFlags(connectFields)
	if err != nil {
		failAndExit(err.Error())
		return
	}

	fields, err := promptCatalogueFields(parseCatalogueFields(svc.Fields), provided)
	if err != nil {
		failAndExit(err.Error())
		return
	}

	name := connectName
	if name == "" {
		name = svc.ServiceName
	}
	if name == "" {
		name = svc.Service
	}

	payload := map[string]interface{}{
		"service": svc.Service,
		"name":    name,
		"fields":  fields,
	}
	if connectIdentifier != "" {
		payload["identifier"] = connectIdentifier
	}
	body, err := json.Marshal(payload)
	if err != nil {
		failAndExit(err.Error())
		return
	}

	resp, err := client.POST("/integrations", body, nil)
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to create integration: %s", err))
		return
	}
	if !resp.IsSuccess() {
		failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
		return
	}

	label, service := extractPublicIdentity(resp.Body)
	if service == "" {
		service = svc.Service
	}
	if label == "" {
		label = name
	}
	printConnected(label, service, resp.Body, connectFormat, connectOutput)
	confirmAddTo(client, service, label)
}

func runTelegramConnect(client *lib.APIClient, svc catalogueService) {
	printTelegramInstructions()

	timeout, err := parseTimeoutFlag(connectTimeout)
	if err != nil {
		failAndExit(err.Error())
		return
	}

	params := map[string]string{"service": "telegram"}
	preList, _, err := listIntegrations(client, params)
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to list Telegram integrations: %s", err))
		return
	}
	known := map[string]struct{}{}
	for _, rec := range preList {
		if key := integrationPublicLabel(rec); key != "" {
			known[key] = struct{}{}
		}
	}

	deadline := nowFn().Add(timeout)
	interval := defaultPollInterval(0)
	connectInfo("Waiting for a Telegram integration...")

	for {
		if !nowFn().Before(deadline) {
			failAndExit("connect timed out waiting for Telegram")
			return
		}

		records, raw, err := listIntegrations(client, params)
		if err != nil {
			failAndExit(fmt.Sprintf("Failed to list Telegram integrations: %s", err))
			return
		}

		if rec, ok := matchTelegramIntegration(records, known, connectName); ok {
			label, service := integrationPublicLabel(rec), rec.Service
			if service == "" {
				service = "telegram"
			}
			matchRaw := raw
			if connectFormat == "json" {
				if encoded, err := json.Marshal(rec); err == nil {
					matchRaw = encoded
				}
			}
			printConnected(label, service, matchRaw, connectFormat, connectOutput)
			confirmAddTo(client, service, label)
			return
		}

		sleepForPoll(interval, deadline)
		if !nowFn().Before(deadline) {
			failAndExit("connect timed out waiting for Telegram")
			return
		}
	}
}

func matchTelegramIntegration(records []integrationRecord, knownLabels map[string]struct{}, name string) (integrationRecord, bool) {
	name = strings.TrimSpace(name)
	for _, rec := range records {
		key := integrationPublicLabel(rec)
		if key == "" {
			continue
		}
		if _, exists := knownLabels[key]; exists {
			continue
		}
		if name != "" && rec.Name != name && rec.Label != name {
			continue
		}
		return rec, true
	}
	return integrationRecord{}, false
}

func confirmAddTo(client *lib.APIClient, service, label string) {
	if connectAddTo == "" {
		return
	}
	if err := addToNotificationList(client, connectAddTo, service, label); err != nil {
		failAndExit(fmt.Sprintf("Connected, but failed to add to notification list: %s", err))
		return
	}
	if !connectIsJSON() {
		Success(fmt.Sprintf("Added to notification list '%s'", connectAddTo))
	}
}

func connectIsJSON() bool {
	return connectFormat == "json"
}

func connectHumanPrintln(s string) {
	if connectIsJSON() {
		fmt.Fprintln(os.Stderr, s)
		return
	}
	fmt.Println(s)
}

func connectInfo(msg string) {
	if connectIsJSON() {
		return
	}
	Info(msg)
}

func printTelegramInstructions() {
	connectHumanPrintln(`To connect Telegram:
1. Open Telegram and start a chat with the Cronitor Telegram bot
2. Follow the bot instructions to link this Cronitor account
3. This command will detect the new integration automatically`)
}
