package lib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/user"
	"regexp"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/viper"
)

// MCPInstance represents a Cronitor dashboard instance configuration
type MCPInstance struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// CronitorMCPHandler handles MCP requests for Cronitor
type CronitorMCPHandler struct {
	instanceName string
	apiURL       string
	username     string
	password     string
	systemPrompt string
}

// NewCronitorMCPHandler creates a new MCP handler for Cronitor
func NewCronitorMCPHandler(instanceName string) *CronitorMCPHandler {
	handler := &CronitorMCPHandler{
		instanceName: instanceName,
	}

	// Load instance configuration
	handler.loadInstanceConfig(instanceName)

	// Set system prompt based on instance
	handler.setSystemPrompt()

	return handler
}

func (h *CronitorMCPHandler) loadInstanceConfig(instanceName string) {
	// Try to load from instances configuration first
	instanceKey := fmt.Sprintf("mcp_instances.%s", instanceName)

	// Check if we have instance-specific configuration
	if viper.IsSet(instanceKey) {
		// Get the nested configuration
		instanceConfig := viper.GetStringMapString(instanceKey)
		h.apiURL = instanceConfig["url"]
		h.username = instanceConfig["username"]
		h.password = instanceConfig["password"]

		// Set default URL if not specified
		if h.apiURL == "" {
			h.apiURL = "http://localhost:9000"
		}
		return
	}

	// Fall back to default configuration for "default" instance
	if instanceName == "default" || instanceName == "" {
		// When "default" is selected, read the username and password directly
		// from the main config (already loaded by viper)
		h.username = viper.GetString("CRONITOR_DASH_USER")
		h.password = viper.GetString("CRONITOR_DASH_PASS")

		// Default URL is localhost:9000
		h.apiURL = "http://localhost:9000"

		// Check environment for custom URL
		if url := os.Getenv("CRONITOR_DASH_URL"); url != "" {
			h.apiURL = url
		}
		return
	}

	// No configuration found - use defaults
	h.apiURL = "http://localhost:9000"
	h.username = viper.GetString("CRONITOR_DASH_USER")
	h.password = viper.GetString("CRONITOR_DASH_PASS")
}

// setSystemPrompt sets instance-specific system prompts
func (h *CronitorMCPHandler) setSystemPrompt() {
	h.systemPrompt = `# Cronitor MCP Rules

When using Cronitor MCP tools:

## Default Behavior
- Always create jobs in the user's personal crontab unless explicitly specified otherwise
- Use descriptive job names that clearly indicate the purpose, avoid underscores and use other special characters sparingly 
- CRITICAL: Do NOT set monitored:true when creating jobs - let it default to false
- NEVER explicitly set the monitored parameter unless the user specifically requests monitoring
- After job creation, mention that monitoring is available and suggest enabling it if desired
- When referencing the cronitor executable, by default it is installed systemwide and should just be called "cronitor" in commands. 

## Job Creation Guidelines
- After creating a job, ASK the user if they want to test it - do NOT automatically run it
- NEVER call run_cronjob_now without explicit user confirmation
- Suggest testing with something like: "Would you like to test this job now?"
- Do NOT include "monitored: true" in create_cronjob calls - omit the parameter entirely
- Only set monitored:true if the user explicitly says "create a monitored job" or "enable monitoring"
- Prefer explicit paths over relying on PATH environment variable, except for "cronitor" When cronitor executable is used, it is installed systemwide and should just be called "cronitor" in commands.

## Command Creation Best Practices
- When possible, create commands that run executables directly with appropriate arguments
- For web projects, use curl to invoke specific endpoints (e.g., curl -X POST http://localhost:3000/api/cron/cleanup)
- For scripts, call the interpreter with the script path (e.g., /usr/bin/python3 /path/to/script.py)
- For system commands, use full paths when possible (e.g., /usr/bin/find instead of just find)
- Include appropriate error handling and logging where applicable

## Remote Host Considerations
- IMPORTANT: If creating a cron job on a remote host, verify the code/script exists there first
- If the user wants to run a local script on a remote server, ASK: "This script needs to be deployed to the remote server first. Would you like help with that?"
- Suggest deployment options like scp, rsync, or git pull depending on the situation
- Never assume local files exist on remote hosts
- For remote instances (production, staging), always confirm deployment status before creating jobs 

## Scheduling Best Practices
- Avoid scheduling multiple heavy jobs at the same time
- For daily jobs, prefer running between 2-4 AM local time
- Use random minutes (not :00 or :30) to avoid load spikes

## Instance Selection
- Users will often have multiple connected Cronitor MCP servers
- Use "default" instance for local development
- Always confirm which instance before making changes
- List existing jobs before creating new ones to avoid duplicates

## REMINDERS
- Users will often have multiple connected Cronitor MCP servers
- Use "default" instance for local development
- Always confirm which instance before making changes
- List existing jobs before creating new ones to avoid duplicates
- Do NOT delete jobs without explicit confirmation from the user
- Do NOT automatically run jobs after creation - always ASK first
- IMPORTANT: When referencing jobs for operations (run, update, delete), always use the job KEY, not the job name
- The job key is returned when creating a job and shown when listing jobs
- For remote hosts: ALWAYS verify scripts/code exist there before creating jobs - offer deployment help if needed`
}

// GetSystemPrompt returns the system prompt for this instance
func (h *CronitorMCPHandler) GetSystemPrompt() string {
	return h.systemPrompt
}

// RegisterTools registers all MCP tools with the server
func (h *CronitorMCPHandler) RegisterTools(s *server.MCPServer) error {
	// Create Cronjob Tool
	tool := mcp.NewTool(
		"create_cronjob",
		mcp.WithDescription("Create a new cron job"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the cron job"),
		),
		mcp.WithString("command",
			mcp.Required(),
			mcp.Description("Command to execute"),
		),
		mcp.WithString("schedule",
			mcp.Required(),
			mcp.Description("Cron expression or human-readable schedule (e.g., 'every 15 minutes')"),
		),
		mcp.WithString("crontab_file",
			mcp.Description("Target crontab file"),
		),
		mcp.WithBoolean("monitored",
			mcp.Description("Enable Cronitor monitoring"),
		),
		mcp.WithString("run_as_user",
			mcp.Description("User to run the job as"),
		),
	)
	s.AddTool(tool, h.handleCreateCronjob)

	// List Cronjobs Tool
	tool = mcp.NewTool(
		"list_cronjobs",
		mcp.WithDescription("List all cron jobs"),
		mcp.WithString("filter",
			mcp.Description("Filter by name or command"),
		),
	)
	s.AddTool(tool, h.handleListCronjobs)

	// Update Cronjob Tool
	tool = mcp.NewTool(
		"update_cronjob",
		mcp.WithDescription("Update an existing cron job"),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Job key or identifier"),
		),
		mcp.WithString("name",
			mcp.Description("New name"),
		),
		mcp.WithString("command",
			mcp.Description("New command"),
		),
		mcp.WithString("schedule",
			mcp.Description("New schedule"),
		),
		mcp.WithBoolean("suspended",
			mcp.Description("Suspend/resume the job"),
		),
		mcp.WithBoolean("monitored",
			mcp.Description("Enable/disable monitoring"),
		),
	)
	s.AddTool(tool, h.handleUpdateCronjob)

	// Delete Cronjob Tool
	tool = mcp.NewTool(
		"delete_cronjob",
		mcp.WithDescription("Delete a cron job"),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Job key or identifier"),
		),
	)
	s.AddTool(tool, h.handleDeleteCronjob)

	// Run Job Now Tool
	tool = mcp.NewTool(
		"run_cronjob_now",
		mcp.WithDescription("Execute a cron job immediately"),
		mcp.WithString("key",
			mcp.Required(),
			mcp.Description("Job key or identifier"),
		),
	)
	s.AddTool(tool, h.handleRunCronjobNow)

	// Get Current Instance Tool
	tool = mcp.NewTool(
		"get_cronitor_instance",
		mcp.WithDescription("Get information about the current Cronitor instance"),
	)
	s.AddTool(tool, h.handleGetInstance)

	return nil
}

// RegisterResources registers all MCP resources with the server
func (h *CronitorMCPHandler) RegisterResources(s *server.MCPServer) error {
	// Register crontabs resource
	resource := mcp.NewResource(
		"cronitor://crontabs",
		"All Crontabs",
		mcp.WithResourceDescription("List of all crontab files"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(resource, h.handleCrontabsResource)

	// Register jobs resource
	resource = mcp.NewResource(
		"cronitor://jobs",
		"All Cron Jobs",
		mcp.WithResourceDescription("List of all cron jobs"),
		mcp.WithMIMEType("application/json"),
	)
	s.AddResource(resource, h.handleJobsResource)

	return nil
}

// Tool Handlers

func (h *CronitorMCPHandler) handleCreateCronjob(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract arguments using the helper methods
	name, err := req.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid name: %v", err)), nil
	}

	command, err := req.RequireString("command")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid command: %v", err)), nil
	}

	schedule, err := req.RequireString("schedule")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid schedule: %v", err)), nil
	}

	// Get current user for defaults
	currentUser := ""
	if u, err := user.Current(); err == nil {
		currentUser = u.Username
	}
	// Fallback to environment variable
	if currentUser == "" {
		currentUser = os.Getenv("USER")
	}
	if currentUser == "" {
		currentUser = os.Getenv("USERNAME") // Windows
	}

	// Optional arguments with better defaults
	crontabFile := req.GetString("crontab_file", "")
	if crontabFile == "" {
		// Default to user crontab for current user
		crontabFile = fmt.Sprintf("user:%s", currentUser)
	}

	monitored := req.GetBool("monitored", false) // Disabled by default per system prompt
	runAsUser := req.GetString("run_as_user", "")

	// For user crontabs, we don't need run_as_user
	// For system crontabs (/etc/crontab or /etc/cron.d/*), we do need it
	if !strings.HasPrefix(crontabFile, "user:") && runAsUser == "" {
		runAsUser = currentUser
	}

	// Parse schedule
	cronExpr := ParseSchedule(schedule)

	// Create job via dashboard API
	jobData := map[string]interface{}{
		"name":             name,
		"expression":       cronExpr,
		"command":          command,
		"crontab_filename": crontabFile,
		"monitored":        monitored,
	}

	// Only add run_as_user for system crontabs
	if runAsUser != "" && !strings.HasPrefix(crontabFile, "user:") {
		jobData["run_as_user"] = runAsUser
	}

	// Make API call to dashboard
	resp, err := h.makeAuthenticatedRequest("POST", h.apiURL+"/api/jobs", jobData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to create job: %v", err)), nil
	}

	// Parse the response to get the job key
	var createdJob struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(resp, &createdJob); err != nil {
		// If we can't parse the response, still return success but without the key
		successMsg := fmt.Sprintf("Successfully created cron job '%s' with schedule '%s' on instance '%s'",
			name, cronExpr, h.instanceName)
		if !monitored {
			successMsg += "\n\nNote: Monitoring is disabled for this job. To enable monitoring and get alerts for failures, you can update the job with monitoring enabled."
		}
		return mcp.NewToolResultText(successMsg), nil
	}

	// Build success message with monitoring reminder and key
	successMsg := fmt.Sprintf("Successfully created cron job '%s' with schedule '%s' on instance '%s'\nJob key: %s",
		name, cronExpr, h.instanceName, createdJob.Key)

	if !monitored {
		successMsg += "\n\nNote: Monitoring is disabled for this job. To enable monitoring and get alerts for failures, you can update the job with monitoring enabled."
	}

	return mcp.NewToolResultText(successMsg), nil
}

func (h *CronitorMCPHandler) handleListCronjobs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Optional filter argument
	filter := req.GetString("filter", "")

	// Get jobs from dashboard API
	resp, err := h.makeAuthenticatedRequest("GET", h.apiURL+"/api/jobs", nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list jobs: %v", err)), nil
	}

	// Parse the Job structure from dash.go
	type Job struct {
		Name       string `json:"name"`
		Command    string `json:"command"`
		Expression string `json:"expression"`
		Monitored  bool   `json:"monitored"`
		Suspended  *bool  `json:"suspended"`
		Key        string `json:"key"`
	}

	var jobs []Job
	if err := json.Unmarshal(resp, &jobs); err != nil {
		return mcp.NewToolResultError("Failed to parse jobs"), nil
	}

	// Apply filter
	var filtered []Job
	for _, job := range jobs {
		if filter == "" ||
			strings.Contains(strings.ToLower(job.Name), strings.ToLower(filter)) ||
			strings.Contains(strings.ToLower(job.Command), strings.ToLower(filter)) {
			filtered = append(filtered, job)
		}
	}

	// Format output
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d jobs on instance '%s':\n\n", len(filtered), h.instanceName))

	for _, job := range filtered {
		status := "active"
		if job.Suspended != nil && *job.Suspended {
			status = "suspended"
		}
		monitoring := "not monitored"
		if job.Monitored {
			monitoring = "monitored"
		}

		output.WriteString(fmt.Sprintf("- %s (%s): %s [%s, %s, key: %s]\n",
			job.Name, job.Expression, job.Command, status, monitoring, job.Key))
	}

	return mcp.NewToolResultText(output.String()), nil
}

func (h *CronitorMCPHandler) handleUpdateCronjob(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid key: %v", err)), nil
	}

	// Build update payload
	updateData := make(map[string]interface{})
	updateData["key"] = key

	// Check for optional fields
	args := req.GetArguments()

	if name := req.GetString("name", ""); name != "" {
		updateData["name"] = name
	}
	if command := req.GetString("command", ""); command != "" {
		updateData["command"] = command
	}
	if schedule := req.GetString("schedule", ""); schedule != "" {
		updateData["expression"] = ParseSchedule(schedule)
	}

	// For boolean values, we need to check if they were actually provided
	if _, hasSuspended := args["suspended"]; hasSuspended {
		updateData["suspended"] = req.GetBool("suspended", false)
	}
	if _, hasMonitored := args["monitored"]; hasMonitored {
		updateData["monitored"] = req.GetBool("monitored", false)
	}

	// Make API call to dashboard
	_, err = h.makeAuthenticatedRequest("PUT", h.apiURL+"/api/jobs", updateData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to update job: %v", err)), nil
	}

	return mcp.NewToolResultText(
		fmt.Sprintf("Successfully updated cron job with key '%s' on instance '%s'", key, h.instanceName),
	), nil
}

func (h *CronitorMCPHandler) handleDeleteCronjob(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid key: %v", err)), nil
	}

	// Build delete payload
	deleteData := map[string]interface{}{
		"key": key,
	}

	// Make API call to dashboard
	_, err = h.makeAuthenticatedRequest("DELETE", h.apiURL+"/api/jobs", deleteData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to delete job: %v", err)), nil
	}

	return mcp.NewToolResultText(
		fmt.Sprintf("Successfully deleted cron job with key '%s' from instance '%s'", key, h.instanceName),
	), nil
}

func (h *CronitorMCPHandler) handleRunCronjobNow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key, err := req.RequireString("key")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid key: %v", err)), nil
	}

	// First, get the job details to find the command
	jobsResp, err := h.makeAuthenticatedRequest("GET", h.apiURL+"/api/jobs", nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get job details: %v", err)), nil
	}

	type Job struct {
		Key             string `json:"key"`
		Command         string `json:"command"`
		CrontabFilename string `json:"crontab_filename"`
	}

	var jobs []Job
	if err := json.Unmarshal(jobsResp, &jobs); err != nil {
		return mcp.NewToolResultError("Failed to parse jobs"), nil
	}

	// Find the job
	var targetJob *Job
	for _, job := range jobs {
		if job.Key == key {
			targetJob = &job
			break
		}
	}

	if targetJob == nil {
		return mcp.NewToolResultError(fmt.Sprintf("Job with key '%s' not found", key)), nil
	}

	// Build run payload
	runData := map[string]interface{}{
		"command":          targetJob.Command,
		"crontab_filename": targetJob.CrontabFilename,
		"key":              targetJob.Key,
	}

	// Make API call to run the job
	_, err = h.makeAuthenticatedRequest("POST", h.apiURL+"/api/jobs/run", runData)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to run job: %v", err)), nil
	}

	return mcp.NewToolResultText(
		fmt.Sprintf("Successfully triggered job with key '%s' on instance '%s'", key, h.instanceName),
	), nil
}

func (h *CronitorMCPHandler) handleGetInstance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	info := fmt.Sprintf("Connected to Cronitor instance: %s\nAPI URL: %s\n\n%s",
		h.instanceName, h.apiURL, h.systemPrompt)
	return mcp.NewToolResultText(info), nil
}

// Resource Handlers

func (h *CronitorMCPHandler) handleCrontabsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	resp, err := h.makeAuthenticatedRequest("GET", h.apiURL+"/api/crontabs", nil)
	if err != nil {
		return nil, err
	}

	// Pretty print the JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, resp, "", "  "); err != nil {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(resp),
			},
		}, nil
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     prettyJSON.String(),
		},
	}, nil
}

func (h *CronitorMCPHandler) handleJobsResource(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	resp, err := h.makeAuthenticatedRequest("GET", h.apiURL+"/api/jobs", nil)
	if err != nil {
		return nil, err
	}

	// Pretty print the JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, resp, "", "  "); err != nil {
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(resp),
			},
		}, nil
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      req.Params.URI,
			MIMEType: "application/json",
			Text:     prettyJSON.String(),
		},
	}, nil
}

// Helper Methods

// Create a shared HTTP client with cookie jar
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
	Jar: func() http.CookieJar {
		jar, _ := cookiejar.New(nil)
		return jar
	}(),
}

func (h *CronitorMCPHandler) makeAuthenticatedRequest(method, apiURL string, body interface{}) ([]byte, error) {
	// For state-changing requests, we need to get a CSRF token first
	if method == "POST" || method == "PUT" || method == "DELETE" {
		// First, make a GET request to get a CSRF token
		getReq, err := http.NewRequest("GET", h.apiURL+"/api/settings", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create CSRF token request: %v", err)
		}

		// Set auth header for the GET request
		if h.username != "" && h.password != "" {
			getReq.SetBasicAuth(h.username, h.password)
		}

		getResp, err := httpClient.Do(getReq)
		if err != nil {
			return nil, fmt.Errorf("failed to get CSRF token: %v", err)
		}
		defer getResp.Body.Close()

		// Check for authentication error
		if getResp.StatusCode == 401 {
			return nil, fmt.Errorf("authentication failed - check username and password for instance '%s'", h.instanceName)
		}

		// Read the response body to ensure cookies are set
		io.ReadAll(getResp.Body)

		// Extract CSRF token from response header
		csrfToken := getResp.Header.Get("X-CSRF-Token")

		// If no token in header, try to extract from cookies
		if csrfToken == "" {
			parsedURL, _ := url.Parse(h.apiURL)
			for _, cookie := range httpClient.Jar.Cookies(parsedURL) {
				if cookie.Name == "csrf_token" {
					csrfToken = cookie.Value
					break
				}
			}
		}

		if csrfToken == "" {
			return nil, fmt.Errorf("failed to obtain CSRF token")
		}

		// Now make the actual request with the CSRF token
		var reqBody io.Reader
		if body != nil {
			jsonBody, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			reqBody = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequest(method, apiURL, reqBody)
		if err != nil {
			return nil, err
		}

		// Set auth header
		if h.username != "" && h.password != "" {
			req.SetBasicAuth(h.username, h.password)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		// Always add CSRF token to header for state-changing requests
		req.Header.Set("X-CSRF-Token", csrfToken)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		// Read response
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
		}

		return respBody, nil
	}

	// For non-state-changing requests (GET), proceed normally
	req, err := http.NewRequest(method, apiURL, nil)
	if err != nil {
		return nil, err
	}

	// Set auth header
	if h.username != "" && h.password != "" {
		req.SetBasicAuth(h.username, h.password)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// ParseSchedule converts human-readable schedules to cron expressions
func ParseSchedule(input string) string {
	input = strings.TrimSpace(input)

	// Common patterns
	patterns := map[string]string{
		"every minute": "* * * * *",
		"every hour":   "0 * * * *",
		"every day":    "0 0 * * *",
		"every week":   "0 0 * * 0",
		"every month":  "0 0 1 * *",
		"hourly":       "0 * * * *",
		"daily":        "0 0 * * *",
		"weekly":       "0 0 * * 0",
		"monthly":      "0 0 1 * *",
		"yearly":       "0 0 1 1 *",
		"annually":     "0 0 1 1 *",
		"midnight":     "0 0 * * *",
		"noon":         "0 12 * * *",
	}

	// Check exact matches first
	if cron, ok := patterns[strings.ToLower(input)]; ok {
		return cron
	}

	// Handle "every N minutes/hours/days"
	if match := regexp.MustCompile(`every (\d+) minutes?`).FindStringSubmatch(input); match != nil {
		n := match[1]
		return fmt.Sprintf("*/%s * * * *", n)
	}

	if match := regexp.MustCompile(`every (\d+) hours?`).FindStringSubmatch(input); match != nil {
		n := match[1]
		return fmt.Sprintf("0 */%s * * *", n)
	}

	if match := regexp.MustCompile(`every (\d+) days?`).FindStringSubmatch(input); match != nil {
		n := match[1]
		return fmt.Sprintf("0 0 */%s * *", n)
	}

	// Handle "at HH:MM"
	if match := regexp.MustCompile(`at (\d{1,2}):(\d{2})`).FindStringSubmatch(input); match != nil {
		hour := match[1]
		minute := match[2]
		return fmt.Sprintf("%s %s * * *", minute, hour)
	}

	// Handle "on Xday at HH:MM" (e.g., "on monday at 10:30")
	dayPattern := regexp.MustCompile(`on (monday|tuesday|wednesday|thursday|friday|saturday|sunday) at (\d{1,2}):(\d{2})`)
	if match := dayPattern.FindStringSubmatch(strings.ToLower(input)); match != nil {
		dayMap := map[string]string{
			"sunday": "0", "monday": "1", "tuesday": "2", "wednesday": "3",
			"thursday": "4", "friday": "5", "saturday": "6",
		}
		day := dayMap[match[1]]
		hour := match[2]
		minute := match[3]
		return fmt.Sprintf("%s %s * * %s", minute, hour, day)
	}

	// If it looks like a cron expression already, return as-is
	if isValidCron(input) {
		return input
	}

	// Default: return the input unchanged
	return input
}

func isValidCron(expr string) bool {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return false
	}

	for _, part := range parts {
		if !regexp.MustCompile(`^(\*|[0-9,\-*/]+|[A-Z]{3})$`).MatchString(part) {
			return false
		}
	}

	return true
}
