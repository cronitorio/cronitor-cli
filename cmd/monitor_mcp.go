package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const mcpAPIVersion = "2025-11-28"

var monitorMcpCmd = &cobra.Command{
	Use:   "mcp [url]",
	Short: "Create or update an MCP server check",
	Long: `Create or update a check that monitors a remote MCP server: it connects, lists the
server's tools and alerts when the tools change from the saved baseline.

Without --key a new check is created. With --key the check is updated if it exists,
otherwise it is created with that key. The URL can be left out when updating.

Authenticate with --header (repeatable; values are stored write-only and never printed)
or with --oauth, which prints a sign-in URL, opens it in your browser and waits for you
to finish signing in (press Ctrl-C to stop waiting).

Examples:
  cronitor monitor mcp https://mcp.example.com/mcp --key docs-mcp --header "Authorization: Bearer $TOKEN"
  cronitor monitor mcp https://mcp.example.com/mcp --key docs-mcp --oauth
  cronitor monitor mcp https://mcp.example.com/mcp --tools search,fetch --include-descriptions
  cronitor monitor mcp --key docs-mcp --accept-baseline`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mcpURL := ""
		if len(args) == 1 {
			mcpURL = args[0]
		}
		runMonitorMcp(cmd.Flags(), mcpURL)
	},
}

var (
	mcpKey                 string
	mcpName                string
	mcpHeaders             []string
	mcpOAuth               bool
	mcpTools               []string
	mcpIncludeDescriptions bool
	mcpAcceptBaseline      bool
)

func init() {
	monitorCmd.AddCommand(monitorMcpCmd)
	f := monitorMcpCmd.Flags()
	f.StringVar(&mcpKey, "key", "", "Monitor key; updates the check when it exists")
	f.StringVar(&mcpName, "name", "", "Monitor name")
	f.StringArrayVar(&mcpHeaders, "header", nil, `Request header as "Name: value" (repeatable)`)
	f.BoolVar(&mcpOAuth, "oauth", false, "Sign in to the MCP server with OAuth in a browser")
	f.StringSliceVar(&mcpTools, "tools", nil, "Only watch these tools (comma-separated)")
	f.BoolVar(&mcpIncludeDescriptions, "include-descriptions", false, "Alert when tool descriptions change")
	f.BoolVar(&mcpAcceptBaseline, "accept-baseline", false, "Save the server's current tools as the new baseline (existing check)")
}

func resetMcpFlags() {
	mcpKey, mcpName, mcpHeaders, mcpOAuth = "", "", nil, false
	mcpTools, mcpIncludeDescriptions, mcpAcceptBaseline = nil, false, false
	monitorMcpCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
}

type mcpMonitor struct {
	Key     string `json:"key"`
	Request struct {
		URL string `json:"url"`
	} `json:"request"`
	Mcp struct {
		Auth            string   `json:"auth"`
		Headers         []string `json:"headers"`
		Tools           []string `json:"tools"`
		ProtocolVersion string   `json:"protocol_version"`
		BaselineTools   []string `json:"baseline_tools"`
		Oauth           *struct {
			Issuer string `json:"issuer"`
		} `json:"oauth"`
	} `json:"mcp"`
}

func runMonitorMcp(flags *pflag.FlagSet, mcpURL string) {
	if mcpOAuth && len(mcpHeaders) > 0 {
		failAndExit("Use --header or --oauth, not both")
		return
	}
	headers, err := parseMcpHeaders(mcpHeaders)
	if err != nil {
		failAndExit(err.Error())
		return
	}

	client := lib.NewAPIClient(dev, log)
	client.APIVersion = mcpAPIVersion

	var existing *mcpMonitor
	if mcpKey != "" {
		existing, err = getMcpMonitor(client, mcpKey)
		if err != nil {
			failAndExit(err.Error())
			return
		}
	}
	if existing == nil && mcpAcceptBaseline {
		failAndExit("--accept-baseline needs --key of an existing MCP check")
		return
	}
	if existing == nil && mcpURL == "" {
		failAndExit("The MCP server URL is required to create a check")
		return
	}

	mcp := map[string]interface{}{}
	if headers != nil {
		mcp["headers"] = headers
	}
	if flags.Changed("tools") {
		mcp["tools"] = mcpTools
	}
	if flags.Changed("include-descriptions") {
		mcp["include_descriptions"] = mcpIncludeDescriptions
	}
	if mcpAcceptBaseline {
		mcp["accept_baseline"] = true
	}
	if mcpOAuth {
		signInURL := mcpURL
		if signInURL == "" {
			signInURL = existing.Request.URL
		}
		connection, err := mcpOAuthSignIn(client, signInURL, existing != nil)
		if err != nil {
			failAndExit(err.Error())
			return
		}
		mcp["oauth_connection"] = connection
	}

	body := map[string]interface{}{"mcp": mcp}
	if mcpURL != "" {
		body["request"] = map[string]string{"url": mcpURL}
	}
	if mcpName != "" {
		body["name"] = mcpName
	}

	var resp *lib.APIResponse
	payload := func() []byte { b, _ := json.Marshal(body); return b }
	if existing != nil {
		resp, err = client.PUT("/monitors/"+url.PathEscape(mcpKey), payload(), nil)
	} else {
		body["type"], body["platform"] = "check", "mcp"
		if mcpKey != "" {
			body["key"] = mcpKey
		}
		resp, err = client.POST("/monitors", payload(), nil)
	}
	if err != nil {
		failAndExit(fmt.Sprintf("Failed to save MCP check: %s", err))
		return
	}
	if !resp.IsSuccess() {
		failAndExit(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, mcpErrorText(resp)))
		return
	}

	var saved mcpMonitor
	if err := json.Unmarshal(resp.Body, &saved); err != nil {
		failAndExit(fmt.Sprintf("Failed to parse response: %s", err))
		return
	}
	verb := "created"
	if existing != nil {
		verb = "updated"
	}
	Success(fmt.Sprintf("MCP check '%s' %s", saved.Key, verb))
	printMcpSummary(saved)
}

// parseMcpHeaders turns "Name: value" flags into a map. Errors never include
// the value.
func parseMcpHeaders(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	headers := map[string]string{}
	for _, h := range raw {
		name, value, ok := strings.Cut(h, ":")
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		if !ok || name == "" || value == "" || strings.ContainsAny(name, " \t") {
			return nil, fmt.Errorf(`Invalid --header: use "Name: value"`)
		}
		rememberSecret(value)
		headers[name] = value
	}
	return headers, nil
}

func getMcpMonitor(client *lib.APIClient, key string) (*mcpMonitor, error) {
	resp, err := client.GET("/monitors/"+url.PathEscape(key), nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to get monitor: %s", err)
	}
	if resp.IsNotFound() {
		return nil, nil
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
	}
	var m mcpMonitor
	if err := json.Unmarshal(resp.Body, &m); err != nil {
		return nil, fmt.Errorf("Failed to parse monitor: %s", err)
	}
	return &m, nil
}

// mcpOAuthSignIn starts an OAuth sign-in, shows the URL and waits until the
// connection is ready. It returns the connection id to attach to the monitor.
func mcpOAuthSignIn(client *lib.APIClient, mcpURL string, forExisting bool) (string, error) {
	start := map[string]string{"url": mcpURL}
	if forExisting {
		start["monitor"] = mcpKey
	}
	body, _ := json.Marshal(start)
	resp, err := client.POST("/monitors/mcp/oauth/start", body, nil)
	if err != nil {
		return "", fmt.Errorf("Failed to start OAuth sign-in: %s", err)
	}
	if !resp.IsSuccess() {
		return "", fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
	}
	var started struct {
		AuthorizeURL string `json:"authorize_url"`
		Connection   string `json:"connection"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if json.Unmarshal(resp.Body, &started) != nil || started.AuthorizeURL == "" || started.Connection == "" {
		return "", fmt.Errorf("OAuth start did not return authorize_url and connection")
	}

	fmt.Println("Open this URL to sign in:")
	fmt.Println()
	fmt.Println("  " + started.AuthorizeURL)
	fmt.Println()
	openBrowserFn(started.AuthorizeURL)

	timeout := time.Duration(started.ExpiresIn) * time.Second
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	deadline := nowFn().Add(timeout)
	Info("Waiting for you to finish signing in (Ctrl-C to cancel)...")
	for {
		resp, err := client.GET("/monitors/mcp/oauth/connections/"+url.PathEscape(started.Connection), nil)
		if err != nil {
			return "", fmt.Errorf("Failed to check sign-in status: %s", err)
		}
		if !resp.IsSuccess() {
			return "", fmt.Errorf("API Error (%d): %s", resp.StatusCode, resp.ParseError())
		}
		var st struct {
			Status string `json:"status"`
			Error  string `json:"error"`
			Issuer string `json:"issuer"`
		}
		json.Unmarshal(resp.Body, &st)
		switch st.Status {
		case "ready":
			if st.Issuer != "" {
				Success("Signed in to " + st.Issuer)
			} else {
				Success("Signed in")
			}
			return started.Connection, nil
		case "failed":
			return "", fmt.Errorf("%s", mcpOAuthErrorMessage(st.Error))
		case "expired":
			return "", fmt.Errorf("%s", mcpOAuthErrorMessage("expired"))
		}
		sleepForPoll(2*time.Second, deadline)
		if !nowFn().Before(deadline) {
			return "", fmt.Errorf("Timed out waiting for the OAuth sign-in. Run the command again to retry.")
		}
	}
}

var mcpOAuthErrors = map[string]string{
	"expired":       "The sign-in expired. Run the command again to retry.",
	"access_denied": "Sign-in was denied or cancelled.",
	"wrong_issuer":  "The sign-in response did not come from the expected authorization server.",
	"failed":        "Sign-in could not be completed. Please try again.",
}

var mcpOAuthCode = regexp.MustCompile(`^[a-z_]{1,32}$`)

func mcpOAuthErrorMessage(code string) string {
	if msg, ok := mcpOAuthErrors[code]; ok {
		return msg
	}
	if mcpOAuthCode.MatchString(code) {
		return "Sign-in was not completed: " + code
	}
	return mcpOAuthErrors["failed"]
}

// mcpErrorText prints the messages of a validation error such as
// {"mcp": ["Auth failed: server returned 401"]} or
// {"mcp": {"oauth_connection": "..."}} without the field names.
func mcpErrorText(resp *lib.APIResponse) string {
	var body map[string]interface{}
	if json.Unmarshal(resp.Body, &body) != nil || body["mcp"] == nil {
		return resp.ParseError()
	}
	var messages []string
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch node := v.(type) {
		case string:
			messages = append(messages, node)
		case []interface{}:
			for _, child := range node {
				walk(child)
			}
		case map[string]interface{}:
			keys := make([]string, 0, len(node))
			for k := range node {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				walk(node[k])
			}
		}
	}
	walk(body["mcp"])
	if len(messages) == 0 {
		return resp.ParseError()
	}
	return strings.Join(messages, "; ")
}

func printMcpSummary(m mcpMonitor) {
	auth := m.Mcp.Auth
	switch {
	case auth == "headers":
		auth += " (" + strings.Join(m.Mcp.Headers, ", ") + ")"
	case auth == "oauth" && m.Mcp.Oauth != nil && m.Mcp.Oauth.Issuer != "":
		auth += " (" + m.Mcp.Oauth.Issuer + ")"
	}
	watched := "all"
	if m.Mcp.Tools != nil {
		watched = strings.Join(m.Mcp.Tools, ", ")
	}
	baseline := strings.Join(m.Mcp.BaselineTools, ", ")
	if baseline == "" {
		baseline = "none"
	}
	fmt.Printf("  Protocol:  %s\n", m.Mcp.ProtocolVersion)
	fmt.Printf("  Auth:      %s\n", auth)
	fmt.Printf("  Watching:  %s\n", watched)
	fmt.Printf("  Baseline:  %s\n", baseline)
}
