package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var apiKeyCmd = &cobra.Command{
	Use:     "apikey",
	Aliases: []string{"apikeys", "keys"},
	Short:   "Manage API keys",
	Long: `Manage your organization's API keys.

API key commands act as the user who created the API key in use, with that
user's current role. They need a key with account access: run
'cronitor auth login --admin' on a machine you trust. You can only grant
scopes that your own role has.

Kinds: custom (choose scopes with --scope), sdk, telemetry.
Scopes: monitor:telemetry, monitor:read, monitor:write, site:read, site:write,
statuspage:read, statuspage:write, issue:read, issue:write,
integration:read, integration:write.

The list table masks key values. --format json prints them in full.

Examples:
  cronitor apikey list
  cronitor apikey list --search deploy --page 2
  cronitor apikey list --format json
  cronitor apikey get <key>
  cronitor apikey create --name "Deploy bot" --scope monitor:read --scope monitor:write
  cronitor apikey create --name "Telemetry" --kind telemetry
  cronitor apikey update <key> --name "Deploy bot (old)"
  cronitor apikey update <key> --scope monitor:read
  cronitor apikey delete <key>
  cronitor apikey delete <key> --yes`,
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

var apiKeyKinds = []string{"custom", "sdk", "telemetry"}

var (
	apiKeyFormat string
	apiKeyOutput string
	apiKeyPage   int
	apiKeySearch string
	apiKeyName   string
	apiKeyKind   string
	apiKeyScopes []string
	apiKeyYes    bool
)

func init() {
	RootCmd.AddCommand(apiKeyCmd)
	apiKeyCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		resetSecretRedaction()
		if err := validateOutputFormat(apiKeyFormat); err != nil {
			failAndExit(err.Error())
		}
	}
	apiKeyCmd.PersistentFlags().StringVar(&apiKeyFormat, "format", "", "Output format: json, table")
	apiKeyCmd.PersistentFlags().StringVarP(&apiKeyOutput, "output", "o", "", "Write output to file")
}

func resetAPIKeyFlags() {
	apiKeyFormat = ""
	apiKeyOutput = ""
	apiKeyPage = 1
	apiKeySearch = ""
	apiKeyName = ""
	apiKeyKind = "custom"
	apiKeyScopes = nil
	apiKeyYes = false
}

// --- LIST ---
var apiKeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys",
	Long: `List API keys. The table masks each key; --format json prints them in full.

Examples:
  cronitor apikey list
  cronitor apikey list --page 2
  cronitor apikey list --search deploy
  cronitor apikey list --format json`,
	Run: func(cmd *cobra.Command, args []string) {
		client := newAPIKeyClient()
		params := make(map[string]string)
		if apiKeyPage > 1 {
			params["page"] = fmt.Sprintf("%d", apiKeyPage)
		}
		if apiKeySearch != "" {
			params["search"] = apiKeySearch
		}

		resp, err := client.GET("/keys", params)
		if err != nil {
			failAPIKey("Failed to list API keys: %s", err)
			return
		}

		if !resp.IsSuccess() {
			failAPIKey("API Error (%d): %s", resp.StatusCode, resp.ParseError())
			return
		}

		var result struct {
			Page       int `json:"page"`
			PageCount  int `json:"page_count"`
			TotalCount int `json:"total_count"`
			Data       []struct {
				Key      string   `json:"key"`
				Name     string   `json:"name"`
				Kind     string   `json:"kind"`
				Scopes   []string `json:"scopes"`
				LastUsed *string  `json:"last_used"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			failAPIKey("Failed to parse response: %s", err)
			return
		}

		format := apiKeyFormat
		if format == "" {
			format = "table"
		}

		if format == "json" {
			writeSecretOutput(apiKeyOutput, FormatJSON(resp.Body))
			return
		}

		table := &UITable{
			Headers: []string{"NAME", "KIND", "SCOPES", "KEY", "LAST USED"},
		}

		for _, k := range result.Data {
			lastUsed := "never"
			if k.LastUsed != nil && *k.LastUsed != "" {
				lastUsed = *k.LastUsed
			}
			table.Rows = append(table.Rows, []string{k.Name, k.Kind, strings.Join(k.Scopes, ", "), maskAPIKey(k.Key), lastUsed})
		}

		output := table.Render()
		if result.TotalCount > 0 {
			output += mutedStyle.Render(fmt.Sprintf("\nShowing page %d of %d • %d keys total",
				result.Page, result.PageCount, result.TotalCount))
		}
		writeSecretOutput(apiKeyOutput, output)
	},
}

// --- GET ---
var apiKeyGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get an API key",
	Long: `Get an API key as JSON.

Examples:
  cronitor apikey get <key>`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		rememberAPIKey(key)
		client := newAPIKeyClient()

		resp, err := client.GET(apiKeyPath(key), nil)
		if err != nil {
			failAPIKey("Failed to get API key: %s", err)
			return
		}

		if resp.IsNotFound() {
			failAPIKey("API key '%s' not found", maskAPIKey(key))
			return
		}

		if !resp.IsSuccess() {
			failAPIKey("API Error (%d): %s", resp.StatusCode, resp.ParseError())
			return
		}

		writeSecretOutput(apiKeyOutput, FormatJSON(resp.Body))
	},
}

// --- CREATE ---
var apiKeyCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API key",
	Long: `Create an API key. The full key is printed once; list output masks it.
With --output the key is written to that file (owner-only) instead of the
terminal; with --format json the file gets the full JSON.

The kind defaults to custom, which needs at least one --scope. Repeat --scope
to grant more than one.

Examples:
  cronitor apikey create --name "Deploy bot" --scope monitor:read --scope monitor:write
  cronitor apikey create --name "Telemetry" --kind telemetry
  cronitor apikey create --name "SDK" --kind sdk --format json
  cronitor apikey create --name "CI" --scope monitor:write --output ci-key.txt`,
	Run: func(cmd *cobra.Command, args []string) {
		if strings.TrimSpace(apiKeyName) == "" {
			failAPIKey("Name required. Use --name")
			return
		}
		if err := validateAPIKeyKind(apiKeyKind); err != nil {
			failAPIKey("%s", err.Error())
			return
		}
		if apiKeyKind == "custom" && len(apiKeyScopes) == 0 {
			failAPIKey("Custom keys need at least one --scope")
			return
		}

		scopes := apiKeyScopes
		if scopes == nil {
			scopes = []string{}
		}
		body, _ := json.Marshal(map[string]interface{}{"name": apiKeyName, "kind": apiKeyKind, "scopes": scopes})

		client := newAPIKeyClient()
		resp, err := client.POST("/keys", body, nil)
		if err != nil {
			failAPIKey("Failed to create API key: %s", err)
			return
		}

		if !resp.IsSuccess() {
			failAPIKey("API Error (%d): %s", resp.StatusCode, resp.ParseError())
			return
		}

		var result struct {
			Key  string `json:"key"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil || result.Key == "" {
			successForFormat(apiKeyFormat, "API key created")
		} else {
			successForFormat(apiKeyFormat, fmt.Sprintf("Created API key: %s", result.Name))
		}

		if apiKeyFormat == "json" {
			writeSecretOutput(apiKeyOutput, FormatJSON(resp.Body))
			return
		}
		if result.Key == "" {
			return
		}
		if apiKeyOutput != "" {
			writeSecretOutput(apiKeyOutput, result.Key)
			return
		}
		fmt.Printf("Key: %s\n", result.Key)
		Info("This is the only time the CLI shows this key unmasked. Store it somewhere safe.")
	},
}

// --- UPDATE ---
var apiKeyUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an API key's name or scopes",
	Long: `Update an API key. Only the fields you pass are changed. --scope replaces
the key's scopes; repeat it to grant more than one.

Examples:
  cronitor apikey update <key> --name "Deploy bot (old)"
  cronitor apikey update <key> --scope monitor:read --scope issue:read`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		rememberAPIKey(key)

		fields := map[string]interface{}{}
		if apiKeyName != "" {
			fields["name"] = apiKeyName
		}
		if len(apiKeyScopes) > 0 {
			fields["scopes"] = apiKeyScopes
		}
		if len(fields) == 0 {
			failAPIKey("Nothing to update. Use --name or --scope")
			return
		}
		body, _ := json.Marshal(fields)

		client := newAPIKeyClient()
		resp, err := client.PATCH(apiKeyPath(key), body, nil)
		if err != nil {
			failAPIKey("Failed to update API key: %s", err)
			return
		}

		if resp.IsNotFound() {
			failAPIKey("API key '%s' not found", maskAPIKey(key))
			return
		}

		if !resp.IsSuccess() {
			failAPIKey("API Error (%d): %s", resp.StatusCode, resp.ParseError())
			return
		}

		successForFormat(apiKeyFormat, fmt.Sprintf("API key '%s' updated", maskAPIKey(key)))
		if apiKeyFormat == "json" {
			writeSecretOutput(apiKeyOutput, FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var apiKeyDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete an API key",
	Long: `Delete an API key. Anything using it stops authenticating.

Asks for confirmation unless --yes is set. The last telemetry or SDK key
can't be deleted.

Examples:
  cronitor apikey delete <key>
  cronitor apikey delete <key> --yes`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		rememberAPIKey(key)
		masked := maskAPIKey(key)

		if err := confirmAccountChange(fmt.Sprintf("Delete API key %s?", masked), apiKeyYes); err != nil {
			failAPIKey("API key '%s' not deleted: %s", masked, err)
			return
		}

		client := newAPIKeyClient()
		resp, err := client.DELETE(apiKeyPath(key), nil, nil)
		if err != nil {
			failAPIKey("Failed to delete API key: %s", err)
			return
		}

		if resp.IsNotFound() {
			failAPIKey("API key '%s' not found", masked)
			return
		}

		if !resp.IsSuccess() {
			failAPIKey("API Error (%d): %s", resp.StatusCode, resp.ParseError())
			return
		}

		successForFormat(apiKeyFormat, fmt.Sprintf("API key '%s' deleted", masked))
	},
}

func init() {
	apiKeyCmd.AddCommand(apiKeyListCmd)
	apiKeyCmd.AddCommand(apiKeyGetCmd)
	apiKeyCmd.AddCommand(apiKeyCreateCmd)
	apiKeyCmd.AddCommand(apiKeyUpdateCmd)
	apiKeyCmd.AddCommand(apiKeyDeleteCmd)

	// List flags
	apiKeyListCmd.Flags().IntVar(&apiKeyPage, "page", 1, "Page number")
	apiKeyListCmd.Flags().StringVar(&apiKeySearch, "search", "", "Filter keys by name")

	// Create flags
	apiKeyCreateCmd.Flags().StringVar(&apiKeyName, "name", "", "Key name")
	apiKeyCreateCmd.Flags().StringVar(&apiKeyKind, "kind", "custom", "Key kind: custom, sdk, telemetry")
	apiKeyCreateCmd.Flags().StringArrayVar(&apiKeyScopes, "scope", nil, "Scope to grant, e.g. monitor:read (repeatable; required for custom keys)")

	// Update flags
	apiKeyUpdateCmd.Flags().StringVar(&apiKeyName, "name", "", "New key name")
	apiKeyUpdateCmd.Flags().StringArrayVar(&apiKeyScopes, "scope", nil, "Scope to grant, replacing the current scopes (repeatable)")

	// Delete flags
	apiKeyDeleteCmd.Flags().BoolVar(&apiKeyYes, "yes", false, "Delete without prompting")
}

// newAPIKeyClient is the API client for key endpoints. Its log lines go
// through redactAPIKeyLogMessage, so --verbose and CRONITOR_LOG never record
// a key value. Keys named on the command line are remembered as secrets
// before the request (rememberAPIKey), which covers the URL path and error
// strings. Keys seen in a response body are remembered too, so a later
// ParseError or network error cannot print them.
func newAPIKeyClient() *lib.APIClient {
	return lib.NewAPIClient(dev, func(msg string) {
		log(redactAPIKeyLogMessage(msg))
	})
}

func failAPIKey(format string, args ...interface{}) {
	failAndExit(redactAPIKeyLogMessage(fmt.Sprintf(format, args...)))
}

// apiKeyJSONField finds "key":"..." even when the surrounding text is not
// valid JSON. The closing quote is optional so a body cut off inside the
// key string is still redacted.
var apiKeyJSONField = regexp.MustCompile(`"key"\s*:\s*"((?:\\.|[^"\\])*)"?`)

// redactAPIKeyLogMessage blanks every "key" value in a logged JSON body and
// replaces any remembered key everywhere else (URL path, error text). On
// these endpoints "key" is the API key itself, which the shared redactor
// does not treat as secret because elsewhere it names a monitor.
func redactAPIKeyLogMessage(msg string) string {
	rememberAPIKeysFromJSONText(msg)
	msg = redactSecrets(msg)
	return apiKeyJSONField.ReplaceAllString(msg, `"key":"[REDACTED]"`)
}

func rememberAPIKeysFromJSONText(msg string) {
	for _, m := range apiKeyJSONField.FindAllStringSubmatch(msg, -1) {
		if len(m) < 2 {
			continue
		}
		if s, err := strconv.Unquote(`"` + m[1] + `"`); err == nil {
			rememberAPIKey(s)
		} else {
			rememberAPIKey(m[1])
		}
	}
}

// rememberAPIKey registers a key for redaction, in the form it is typed
// and the form it takes in a request path.
func rememberAPIKey(key string) {
	rememberSecret(key)
	if escaped := url.PathEscape(key); escaped != key {
		rememberSecret(escaped)
	}
}

func apiKeyPath(key string) string {
	return "/keys/" + url.PathEscape(key)
}

// maskAPIKey shows the first and last four characters of a key. Keys too
// short to hide anything that way are hidden entirely.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "…"
	}
	return key[:4] + "…" + key[len(key)-4:]
}

func validateAPIKeyKind(kind string) error {
	for _, k := range apiKeyKinds {
		if kind == k {
			return nil
		}
	}
	return fmt.Errorf("Invalid kind '%s'. Use one of: %s", kind, strings.Join(apiKeyKinds, ", "))
}
