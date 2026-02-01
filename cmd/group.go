package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
)

var (
	groupPage       int
	groupPageSize   int
	groupEnv        string
	groupFormat     string
	groupOutput     string
	groupWithStatus bool
	groupFetchAll   bool
	groupSort       string
	groupData       string
)

var groupCmd = &cobra.Command{
	Use:   "group",
	Short: "Manage monitor groups",
	Long: `Create, list, update, and delete monitor groups.

For full API documentation, see https://cronitor.io/docs/groups-api.md`,
}

func init() {
	RootCmd.AddCommand(groupCmd)

	// Add subcommands
	groupCmd.AddCommand(groupListCmd)
	groupCmd.AddCommand(groupGetCmd)
	groupCmd.AddCommand(groupCreateCmd)
	groupCmd.AddCommand(groupUpdateCmd)
	groupCmd.AddCommand(groupDeleteCmd)
	groupCmd.AddCommand(groupPauseCmd)
	groupCmd.AddCommand(groupResumeCmd)

	// Persistent flags for all group subcommands
	groupCmd.PersistentFlags().IntVar(&groupPage, "page", 1, "Page number for paginated results")
	groupCmd.PersistentFlags().StringVar(&groupEnv, "env", "", "Filter by environment")
	groupCmd.PersistentFlags().StringVar(&groupFormat, "format", "", "Output format: json, table")
	groupCmd.PersistentFlags().StringVarP(&groupOutput, "output", "o", "", "Write output to file")
}

// --- LIST ---
var groupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all groups",
	Long: `List all monitor groups with optional filtering.

Examples:
  cronitor group list
  cronitor group list --page 2
  cronitor group list --page-size 50
  cronitor group list --with-status
  cronitor group list --env production`,
	Run: func(cmd *cobra.Command, args []string) {
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if groupPage > 1 {
			params["page"] = fmt.Sprintf("%d", groupPage)
		}
		if groupPageSize > 0 {
			params["pageSize"] = fmt.Sprintf("%d", groupPageSize)
		}
		if groupEnv != "" {
			params["env"] = groupEnv
		}
		if groupWithStatus {
			params["withStatus"] = "true"
		}

		if groupFetchAll {
			bodies, err := FetchAllPages(client, "/groups", params, "groups")
			if err != nil {
				Error(fmt.Sprintf("Failed to list groups: %s", err))
				os.Exit(1)
			}
			if groupFormat == "json" || groupFormat == "" {
				outputGroupToTarget(FormatJSON(MergePagedJSON(bodies, "groups")))
				return
			}
			// Table: accumulate rows from all pages
			table := &UITable{
				Headers: []string{"NAME", "KEY", "MONITORS", "CREATED"},
			}
			for _, body := range bodies {
				var result struct {
					Groups []struct {
						Key      string   `json:"key"`
						Name     string   `json:"name"`
						Monitors []string `json:"monitors"`
						Created  string   `json:"created"`
					} `json:"groups"`
				}
				json.Unmarshal(body, &result)
				for _, g := range result.Groups {
					monitorCount := fmt.Sprintf("%d", len(g.Monitors))
					created := ""
					if g.Created != "" {
						created = g.Created[:10]
					}
					table.Rows = append(table.Rows, []string{g.Name, g.Key, monitorCount, created})
				}
			}
			outputGroupToTarget(table.Render())
			return
		}

		resp, err := client.GET("/groups", params)
		if err != nil {
			Error(fmt.Sprintf("Failed to list groups: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if groupFormat == "json" || groupFormat == "" {
			outputGroupToTarget(FormatJSON(resp.Body))
			return
		}

		// Parse for table output
		var result struct {
			Groups []struct {
				Key      string   `json:"key"`
				Name     string   `json:"name"`
				Monitors []string `json:"monitors"`
				Created  string   `json:"created"`
			} `json:"groups"`
			PageSize   int `json:"page_size"`
			Page       int `json:"page"`
			TotalCount int `json:"total_count"`
		}

		if err := json.Unmarshal(resp.Body, &result); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		if len(result.Groups) == 0 {
			outputGroupToTarget(mutedStyle.Render("No groups found"))
			return
		}

		// Table output
		table := &UITable{
			Headers: []string{"NAME", "KEY", "MONITORS", "CREATED"},
		}
		for _, g := range result.Groups {
			monitorCount := fmt.Sprintf("%d", len(g.Monitors))
			created := ""
			if g.Created != "" {
				created = g.Created[:10] // Just the date part
			}
			table.Rows = append(table.Rows, []string{g.Name, g.Key, monitorCount, created})
		}
		outputGroupToTarget(table.Render())

		if result.TotalCount > result.PageSize {
			fmt.Printf("\nPage %d of %d (total: %d groups)\n",
				result.Page, (result.TotalCount+result.PageSize-1)/result.PageSize, result.TotalCount)
		}
	},
}

// --- GET ---
var groupGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a specific group",
	Long: `Retrieve details for a specific group by key.

Examples:
  cronitor group get my-group
  cronitor group get my-group --with-status
  cronitor group get my-group --format json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)
		params := make(map[string]string)

		if groupEnv != "" {
			params["env"] = groupEnv
		}
		if groupWithStatus {
			params["withStatus"] = "true"
		}
		if groupSort != "" {
			params["sort"] = groupSort
		}

		resp, err := client.GET(fmt.Sprintf("/groups/%s", key), params)
		if err != nil {
			Error(fmt.Sprintf("Failed to get group: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Group '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		if groupFormat == "json" || groupFormat == "" {
			outputGroupToTarget(FormatJSON(resp.Body))
			return
		}

		// Parse for detailed output
		var group struct {
			Key         string   `json:"key"`
			Name        string   `json:"name"`
			Monitors    []string `json:"monitors"`
			Created     string   `json:"created"`
			LatestEvent struct {
				Stamp string `json:"stamp"`
				State string `json:"state"`
			} `json:"latest_event"`
		}

		if err := json.Unmarshal(resp.Body, &group); err != nil {
			Error(fmt.Sprintf("Failed to parse response: %s", err))
			os.Exit(1)
		}

		// Table output for single group
		fmt.Printf("Group: %s\n", boldStyle.Render(group.Name))
		fmt.Printf("Key: %s\n", group.Key)
		if group.Created != "" {
			fmt.Printf("Created: %s\n", group.Created)
		}
		if group.LatestEvent.Stamp != "" {
			fmt.Printf("Latest Event: %s (%s)\n", group.LatestEvent.Stamp, group.LatestEvent.State)
		}
		if len(group.Monitors) > 0 {
			fmt.Printf("\nMonitors (%d):\n", len(group.Monitors))
			for _, m := range group.Monitors {
				fmt.Printf("  - %s\n", m)
			}
		}
	},
}

// --- CREATE ---
var groupCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new group",
	Long: `Create a new monitor group.

Examples:
  cronitor group create --data '{"name":"Production Jobs"}'
  cronitor group create --data '{"name":"Production Jobs","key":"prod-jobs","monitors":["job1","job2"]}'`,
	Run: func(cmd *cobra.Command, args []string) {
		if groupData == "" {
			Error("Create data required. Use --data '{...}'")
			os.Exit(1)
		}

		var js json.RawMessage
		if err := json.Unmarshal([]byte(groupData), &js); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}

		client := lib.NewAPIClient(dev, log)
		resp, err := client.POST("/groups", []byte(groupData), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to create group: %s", err))
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
			Success(fmt.Sprintf("Created group: %s (key: %s)", result.Name, result.Key))
		} else {
			Success("Group created successfully")
		}

		if groupFormat == "json" {
			outputGroupToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- UPDATE ---
var groupUpdateCmd = &cobra.Command{
	Use:   "update <key>",
	Short: "Update an existing group",
	Long: `Update an existing monitor group.

Examples:
  cronitor group update my-group --data '{"name":"New Name"}'
  cronitor group update my-group --data '{"monitors":["job1","job2","job3"]}'`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		if groupData == "" {
			Error("Update data required. Use --data '{...}'")
			os.Exit(1)
		}

		var bodyMap map[string]interface{}
		if err := json.Unmarshal([]byte(groupData), &bodyMap); err != nil {
			Error(fmt.Sprintf("Invalid JSON: %s", err))
			os.Exit(1)
		}
		bodyMap["key"] = key
		body, _ := json.Marshal(bodyMap)

		client := lib.NewAPIClient(dev, log)
		resp, err := client.PUT(fmt.Sprintf("/groups/%s", key), body, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to update group: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Updated group: %s", key))

		if groupFormat == "json" {
			outputGroupToTarget(FormatJSON(resp.Body))
		}
	},
}

// --- DELETE ---
var groupDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a group",
	Long: `Delete a monitor group. This does not delete the monitors in the group.

Examples:
  cronitor group delete my-group`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.DELETE(fmt.Sprintf("/groups/%s", key), nil, nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to delete group: %s", err))
			os.Exit(1)
		}

		if resp.IsNotFound() {
			Error(fmt.Sprintf("Group '%s' not found", key))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Deleted group: %s", key))
	},
}

// --- PAUSE ---
var groupPauseCmd = &cobra.Command{
	Use:   "pause <key> <hours>",
	Short: "Pause all monitors in a group",
	Long: `Pause all monitors in a group for the specified number of hours.

Examples:
  cronitor group pause my-group 1      # Pause for 1 hour
  cronitor group pause my-group 24     # Pause for 24 hours
  cronitor group pause my-group 168    # Pause for 1 week`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		hours := args[1]
		client := lib.NewAPIClient(dev, log)

		resp, err := client.GET(fmt.Sprintf("/groups/%s/pause/%s", key, hours), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to pause group: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Paused all monitors in group '%s' for %s hours", key, hours))
	},
}

// --- RESUME ---
var groupResumeCmd = &cobra.Command{
	Use:   "resume <key>",
	Short: "Resume all monitors in a group",
	Long: `Resume all paused monitors in a group.

Examples:
  cronitor group resume my-group`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		client := lib.NewAPIClient(dev, log)

		// Resume is just pause with 0 hours
		resp, err := client.GET(fmt.Sprintf("/groups/%s/pause/0", key), nil)
		if err != nil {
			Error(fmt.Sprintf("Failed to resume group: %s", err))
			os.Exit(1)
		}

		if !resp.IsSuccess() {
			Error(fmt.Sprintf("API Error (%d): %s", resp.StatusCode, resp.ParseError()))
			os.Exit(1)
		}

		Success(fmt.Sprintf("Resumed all monitors in group '%s'", key))
	},
}

func init() {
	// List command flags
	groupListCmd.Flags().IntVar(&groupPageSize, "page-size", 0, "Number of results per page")
	groupListCmd.Flags().BoolVar(&groupWithStatus, "with-status", false, "Include status information")
	groupListCmd.Flags().BoolVar(&groupFetchAll, "all", false, "Fetch all pages of results")

	// Get command flags
	groupGetCmd.Flags().BoolVar(&groupWithStatus, "with-status", false, "Include status information")
	groupGetCmd.Flags().StringVar(&groupSort, "sort", "", "Sort order for monitors")

	// Create command flags
	groupCreateCmd.Flags().StringVarP(&groupData, "data", "d", "", "JSON payload")

	// Update command flags
	groupUpdateCmd.Flags().StringVarP(&groupData, "data", "d", "", "JSON payload")
}

func outputGroupToTarget(content string) {
	if groupOutput != "" {
		if err := os.WriteFile(groupOutput, []byte(content+"\n"), 0644); err != nil {
			Error(fmt.Sprintf("Failed to write to file: %s", err))
			os.Exit(1)
		}
		Info(fmt.Sprintf("Output written to %s", groupOutput))
	} else {
		fmt.Println(content)
	}
}
