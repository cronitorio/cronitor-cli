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
	GroupID: GroupAPI,
	Use:     "statuspage",
	Short: "Manage status pages",
	Long: `Manage Cronitor status pages and their components.

Status pages display the health of your monitors to your users.
Components are individual items on a status page (linked to monitors or groups).

Examples:
  cronitor statuspage list
  cronitor statuspage list --with-status
  cronitor statuspage get my-page --with-components
  cronitor statuspage create "My Status Page" --subdomain my-status
  cronitor statuspage delete <key>

  cronitor statuspage component list --statuspage my-page
  cronitor statuspage component create --statuspage my-page --monitor api-health
  cronitor statuspage component update <key> --data '{"name":"New Name"}'
  cronitor statuspage component delete <component-key>

For full API documentation:
  Humans: https://cronitor.io/docs/statuspages-api
  Agents: https://cronitor.io/docs/statuspages-api.md`,
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
	statuspagePage           int
	statuspageFormat         string
	statuspageOutput         string
	statuspageData           string
	statuspageWithStatus     bool
	statuspageWithComponents bool
	// Component flags
	componentStatuspage string
	componentData       string
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
	Long: `List all status pages.

Examples:
  cronitor statuspage list
  cronitor statuspage list --with-status
  cronitor statuspage list --with-components`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)
		if statuspagePage > 1 {
			params["page"] = fmt.Sprintf("%d", statuspagePage)
		}
		if statuspageWithStatus {
			params["withStatus"] = "true"
		}
		if statuspageWithComponents {
			params["withComponents"] = "true"
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
				Subdomain string `json:"hosted_subdomain"`
				Status    string `json:"status"`
			} `json:"data"`
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
			Headers: []string{"NAME", "KEY", "SUBDOMAIN", "STATUS"},
		}

		for _, sp := range result.StatusPages {
			status := successStyle.Render(sp.Status)
			if sp.Status != "operational" {
				status = warningStyle.Render(sp.Status)
			}
			table.Rows = append(table.Rows, []string{sp.Name, sp.Key, sp.Subdomain, status})
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
	Long: `Create a new status page.

Examples:
  cronitor statuspage create --data '{"name":"My Status Page","subdomain":"my-status"}'
  cronitor statuspage create --data '{"name":"Internal Status","subdomain":"internal","access":"private"}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if statuspageData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(statuspageData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/statuspages", []byte(statuspageData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create status page: %s", err))
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
			Success(fmt.Sprintf("Created status page: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Status page created")
		}

		statuspageOutputToTarget(FormatJSON(resp.Body))
	},
}

// --- UPDATE ---
var statuspageUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a status page",
	Long: `Update an existing status page.

Examples:
  cronitor statuspage update my-page --data '{"name":"New Name"}'
  cronitor statuspage update my-page --data '{"access":"private"}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if statuspageData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(statuspageData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/statuspages/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update status page: %s", err))
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
	statuspageCmd.AddCommand(componentCmd)

	// List flags
	statuspageListCmd.Flags().BoolVar(&statuspageWithStatus, "with-status", false, "Include current status")
	statuspageListCmd.Flags().BoolVar(&statuspageWithComponents, "with-components", false, "Include component details")

	// Get flags
	statuspageGetCmd.Flags().BoolVar(&statuspageWithStatus, "with-status", false, "Include current status")
	statuspageGetCmd.Flags().BoolVar(&statuspageWithComponents, "with-components", false, "Include component details")

	// Create flags
	statuspageCreateCmd.Flags().StringVarP(&statuspageData, "data", "d", "", "JSON payload")

	// Update flags
	statuspageUpdateCmd.Flags().StringVarP(&statuspageData, "data", "d", "", "JSON payload")
}

// --- COMPONENT COMMANDS ---
var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Manage status page components",
	Long: `Manage components on status pages.

Components represent individual services/monitors displayed on a status page.

Examples:
  cronitor statuspage component list --statuspage my-page
  cronitor statuspage component create --statuspage my-page --monitor api-health
  cronitor statuspage component update <key> --data '{"name":"New Name"}'
  cronitor statuspage component delete <component-key>`,
}

var componentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List components",
	Long: `List status page components.

Examples:
  cronitor statuspage component list --statuspage my-page
  cronitor statuspage component list --statuspage my-page --with-status`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if componentStatuspage != "" {
			params["statuspage"] = componentStatuspage
		}
		if statuspageWithStatus {
			params["withStatus"] = "true"
		}

		resp, err := client.GET("/statuspage_components", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list components: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if statuspageFormat == "json" {
			statuspageOutputToTarget(FormatJSON(resp.Body))
			return
		}

		var result struct {
			Components []struct {
				Key        string `json:"key"`
				Name       string `json:"name"`
				Type       string `json:"type"`
				Statuspage string `json:"statuspage"`
				Autopub    bool   `json:"autopublish"`
			} `json:"data"`
		}
		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		table := &UITable{
			Headers: []string{"NAME", "KEY", "TYPE", "STATUSPAGE", "AUTOPUBLISH"},
		}

		for _, c := range result.Components {
			autopub := "no"
			if c.Autopub {
				autopub = "yes"
			}
			table.Rows = append(table.Rows, []string{c.Name, c.Key, c.Type, c.Statuspage, autopub})
		}

		statuspageOutputToTarget(table.Render())
	},
}

var componentCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a component",
	Long: `Create a new status page component.

Examples:
  cronitor statuspage component create --data '{"statuspage":"my-page","monitor":"api-health"}'
  cronitor statuspage component create --data '{"statuspage":"my-page","group":"production","name":"Production"}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if componentData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(componentData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/statuspage_components", []byte(componentData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create component: %s", err))
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
			Success(fmt.Sprintf("Created component: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Component created")
		}
	},
}

var componentUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update a component",
	Long: `Update an existing status page component.

Updatable fields: name, description, autopublish.

Examples:
  cronitor statuspage component update my-comp --data '{"name":"New Name"}'
  cronitor statuspage component update my-comp --data '{"autopublish":false}'
  cronitor statuspage component update my-comp --data '{"description":"Updated description"}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if componentData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(componentData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/statuspage_components/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update component: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Component '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Component '%s' updated", key))
		statuspageOutputToTarget(FormatJSON(resp.Body))
	},
}

var componentDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a component",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/statuspage_components/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete component: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Component '%s' not found", key))
			os.Exit(1)
		}

		if resp.IsSuccess() {
			Success(fmt.Sprintf("Component '%s' deleted", key))
		} else {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}
	},
}

func init() {
	componentCmd.AddCommand(componentListCmd)
	componentCmd.AddCommand(componentCreateCmd)
	componentCmd.AddCommand(componentUpdateCmd)
	componentCmd.AddCommand(componentDeleteCmd)

	// Component list flags
	componentListCmd.Flags().StringVar(&componentStatuspage, "statuspage", "", "Filter by status page key")
	componentListCmd.Flags().BoolVar(&statuspageWithStatus, "with-status", false, "Include status information")

	// Component create flags
	componentCreateCmd.Flags().StringVarP(&componentData, "data", "d", "", "JSON payload")

	// Component update flags
	componentUpdateCmd.Flags().StringVarP(&componentData, "data", "d", "", "JSON payload")
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
