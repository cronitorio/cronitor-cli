package cmd

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

//go:embed web/dist
var webAssets embed.FS

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}
}

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Start the Cronitor web dashboard",
	Long: `Start the Cronitor web dashboard server.
The dashboard provides a web interface for managing your Cronitor monitors and crontabs.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetInt("port")
		if port == 0 {
			port = 9000
		}

		// Basic auth middleware
		authMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username := viper.GetString(varDashUsername)
				password := viper.GetString(varDashPassword)

				if username == "" || password == "" {
					http.Error(w, "Dashboard credentials not configured", http.StatusInternalServerError)
					return
				}

				auth := r.Header.Get("Authorization")
				if auth == "" {
					w.Header().Set("WWW-Authenticate", `Basic realm="Cronitor Dashboard"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				payload, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
				pair := strings.SplitN(string(payload), ":", 2)

				if len(pair) != 2 || pair[0] != username || pair[1] != password {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				next.ServeHTTP(w, r)
			})
		}

		// Get the embedded filesystem
		fsys, err := fs.Sub(webAssets, "web/dist")
		if err != nil {
			fatal(err.Error(), 1)
		}

		// Create a custom file server that serves index.html for all routes
		fileServer := http.FileServer(http.FS(fsys))
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Don't serve index.html for API routes or static assets
			if strings.HasPrefix(r.URL.Path, "/api/") ||
				strings.HasPrefix(r.URL.Path, "/assets/") ||
				strings.Contains(r.URL.Path, ".") {
				fileServer.ServeHTTP(w, r)
				return
			}

			// For all other routes, serve index.html
			index, err := fsys.Open("index.html")
			if err != nil {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			defer index.Close()
			http.ServeContent(w, r, "index.html", time.Now(), index.(io.ReadSeeker))
		})

		// Apply auth middleware to all routes
		http.Handle("/", authMiddleware(handler))

		// Add settings API endpoints
		http.Handle("/api/settings", authMiddleware(http.HandlerFunc(handleSettings)))

		// Add jobs endpoint
		http.Handle("/api/jobs", authMiddleware(http.HandlerFunc(handleJobs)))

		// Add kill jobs endpoint
		http.Handle("/api/jobs/kill", authMiddleware(http.HandlerFunc(handleKillJobs)))

		// Start the server in a goroutine
		go func() {
			fmt.Printf("Starting Cronitor dashboard on port %d...\n", port)
			if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
				fatal(err.Error(), 1)
			}
		}()

		// Wait a moment for the server to start
		time.Sleep(500 * time.Millisecond)

		// Open the browser
		url := fmt.Sprintf("http://localhost:%d", port)
		fmt.Printf("Opening browser to %s...\n", url)
		openBrowser(url)

		// Keep the main goroutine running
		select {}
	},
}

func init() {
	RootCmd.AddCommand(dashCmd)
	dashCmd.Flags().Int("port", 9000, "Port to run the dashboard on")
}

type SettingsResponse struct {
	ConfigFile
	EnvVars        map[string]bool `json:"env_vars"`
	ConfigFilePath string          `json:"config_file_path"`
	Version        string          `json:"version"`
}

// handleSettings handles GET and POST requests for settings
func handleSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		// Read the current config file
		configPath := configFilePath()
		data, err := ioutil.ReadFile(configPath)
		if err != nil {
			// If file doesn't exist, return empty config
			data = []byte("{}")
		}

		var configData ConfigFile
		if err := json.Unmarshal(data, &configData); err != nil {
			http.Error(w, "Failed to parse config file", http.StatusInternalServerError)
			return
		}

		// Create response with env var information
		response := SettingsResponse{
			ConfigFile:     configData,
			ConfigFilePath: configPath,
			Version:        Version,
			EnvVars: map[string]bool{
				"CRONITOR_API_KEY":      os.Getenv(varApiKey) != "",
				"CRONITOR_PING_API_KEY": os.Getenv(varPingApiKey) != "",
				"CRONITOR_EXCLUDE_TEXT": os.Getenv(varExcludeText) != "",
				"CRONITOR_HOSTNAME":     os.Getenv(varHostname) != "",
				"CRONITOR_LOG":          os.Getenv(varLog) != "",
				"CRONITOR_ENV":          os.Getenv(varEnv) != "",
				"CRONITOR_DASH_USER":    os.Getenv(varDashUsername) != "",
				"CRONITOR_DASH_PASS":    os.Getenv(varDashPassword) != "",
			},
		}

		// Override config values with environment variables if they exist
		if os.Getenv(varDashUsername) != "" {
			response.DashUsername = os.Getenv(varDashUsername)
		}
		if os.Getenv(varDashPassword) != "" {
			response.DashPassword = os.Getenv(varDashPassword)
		}

		responseData, err := json.Marshal(response)
		if err != nil {
			http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
			return
		}

		w.Write(responseData)

	case "POST":
		// Read the request body
		var configData ConfigFile
		if err := json.NewDecoder(r.Body).Decode(&configData); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Only update viper values that aren't set by environment variables
		if !viper.IsSet(varApiKey) {
			viper.Set(varApiKey, configData.ApiKey)
		}
		if !viper.IsSet(varPingApiKey) {
			viper.Set(varPingApiKey, configData.PingApiAuthKey)
		}
		if !viper.IsSet(varExcludeText) {
			viper.Set(varExcludeText, configData.ExcludeText)
		}
		if !viper.IsSet(varHostname) {
			viper.Set(varHostname, configData.Hostname)
		}
		if !viper.IsSet(varLog) {
			viper.Set(varLog, configData.Log)
		}
		if !viper.IsSet(varEnv) {
			viper.Set(varEnv, configData.Env)
		}
		if !viper.IsSet(varDashUsername) {
			viper.Set(varDashUsername, configData.DashUsername)
		}
		if !viper.IsSet(varDashPassword) {
			viper.Set(varDashPassword, configData.DashPassword)
		}

		// Marshal the config data
		b, err := json.MarshalIndent(configData, "", "    ")
		if err != nil {
			http.Error(w, "Failed to marshal config data", http.StatusInternalServerError)
			return
		}

		// Write to config file
		configPath := configFilePath()
		if err := os.MkdirAll(defaultConfigFileDirectory(), os.ModePerm); err != nil {
			http.Error(w, "Failed to create config directory", http.StatusInternalServerError)
			return
		}

		if err := ioutil.WriteFile(configPath, b, 0644); err != nil {
			http.Error(w, "Failed to write config file", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write(b)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

type JobInstance struct {
	PID     string `json:"pid"`
	Started string `json:"started"`
}

type Job struct {
	Name        string        `json:"name"`
	DefaultName string        `json:"default_name"`
	Command     string        `json:"command"`
	Expression  string        `json:"expression"`
	RunAsUser   string        `json:"run_as_user"`
	CronFile    string        `json:"cron_file"`
	LineNumber  int           `json:"line_number"`
	IsMonitored bool          `json:"is_monitored"`
	Timezone    string        `json:"timezone"`
	Passing     bool          `json:"passing"`
	Disabled    bool          `json:"disabled"`
	Paused      bool          `json:"paused"`
	Initialized bool          `json:"initialized"`
	Code        string        `json:"code"`
	Key         string        `json:"key"`
	Instances   []JobInstance `json:"instances"`
}

// handleJobs handles GET and PUT requests for jobs
func handleJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		handleGetJobs(w, r)
	case "PUT":
		handlePutJob(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetJobs(w http.ResponseWriter, r *http.Request) {
	var err error
	existingMonitors.Monitors, err = getCronitorApi().GetMonitors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var username string
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	var jobs []Job
	var crontabs []*lib.Crontab

	// Read user crontab
	crontabs = lib.ReadCrontabFromFile(username, "", crontabs)

	// Read system crontab if it exists
	if systemCrontab := lib.CrontabFactory(username, lib.SYSTEM_CRONTAB); systemCrontab.Exists() {
		crontabs = lib.ReadCrontabFromFile(username, lib.SYSTEM_CRONTAB, crontabs)
	}

	// Read crontabs from drop-in directory
	crontabs = lib.ReadCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)

	// Process each crontab
	for _, crontab := range crontabs {
		hasChanges := false
		for i := range crontab.Lines {
			line := crontab.Lines[i]
			if !line.IsMonitorable() {
				continue
			}

			// If we know this monitor exists already, return the name
			line.Mon = existingMonitors.Get(line.Key(crontab.CanonicalName()), line.Code)
			if line.Mon.Name != "" && line.Mon.Name != line.Name {
				line.Name = line.Mon.Name
				hasChanges = true
			}

			timezone := effectiveTimezoneLocationName().Name
			if crontab.TimezoneLocationName != nil {
				timezone = crontab.TimezoneLocationName.Name
			}

			runAsUser := line.RunAs
			if runAsUser == "" {
				runAsUser = crontab.User
			}

			job := Job{
				Name:        line.Name,
				DefaultName: createDefaultName(line, crontab, "", []string{}, map[string]bool{}),
				Command:     line.CommandToRun,
				Expression:  line.CronExpression,
				RunAsUser:   runAsUser,
				CronFile:    crontab.DisplayName(),
				LineNumber:  line.LineNumber + 1,
				IsMonitored: len(line.Code) > 0,
				Timezone:    timezone,
				Passing:     line.Mon.Passing,
				Disabled:    line.Mon.Disabled,
				Paused:      line.Mon.Paused,
				Initialized: line.Mon.Initialized,
				Code:        line.Code,
				Key:         line.Key(crontab.CanonicalName()),
				Instances:   find_instances(line.CommandToRun),
			}

			jobs = append(jobs, job)
		}
		if hasChanges {
			crontab.Save(crontab.Write())
		}
	}

	responseData, err := json.Marshal(jobs)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Write(responseData)
}

func handlePutJob(w http.ResponseWriter, r *http.Request) {
	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var username string
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	var crontabs []*lib.Crontab
	var foundLine *lib.Line
	var foundCrontab *lib.Crontab

	// Read user crontab
	crontabs = lib.ReadCrontabFromFile(username, "", crontabs)

	// Read system crontab if it exists
	if systemCrontab := lib.CrontabFactory(username, lib.SYSTEM_CRONTAB); systemCrontab.Exists() {
		crontabs = lib.ReadCrontabFromFile(username, lib.SYSTEM_CRONTAB, crontabs)
	}

	// Read crontabs from drop-in directory
	crontabs = lib.ReadCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)

	// Find the matching line
	for _, crontab := range crontabs {
		for _, line := range crontab.Lines {
			if (job.Code != "" && line.Code == job.Code) || (job.Key != "" && line.Key(crontab.CanonicalName()) == job.Key) {
				foundLine = line
				foundCrontab = crontab
				break
			}
		}
		if foundLine != nil {
			break
		}
	}

	if foundLine == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Update the line
	hasChanges := false

	// Handle name update
	if job.Name != foundLine.Name {
		foundLine.Name = job.Name
		hasChanges = true

		// If monitor exists, update its name
		if foundLine.Code != "" {
			monitor := lib.Monitor{
				Name:        job.Name,
				DefaultName: createDefaultName(foundLine, foundCrontab, "", []string{}, map[string]bool{}),
				Schedule:    job.Expression,
				Type:        "job",
				Platform:    lib.CRON,
				Timezone:    job.Timezone,
				Code:        foundLine.Code,
			}

			monitors := map[string]*lib.Monitor{
				monitor.Key: &monitor,
			}

			updatedMonitors, err := getCronitorApi().PutMonitors(monitors)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if updatedMonitor, exists := updatedMonitors[monitor.Key]; exists {
				foundLine.Mon = *updatedMonitor
				hasChanges = true
			}
		}
	}

	if job.RunAsUser != "" && foundLine.RunAs != job.RunAsUser {
		foundLine.RunAs = job.RunAsUser
		hasChanges = true
	}
	if job.Expression != "" && foundLine.CronExpression != job.Expression {
		foundLine.CronExpression = job.Expression
		hasChanges = true
	}
	if job.Timezone != "" && foundCrontab.TimezoneLocationName != nil && foundCrontab.TimezoneLocationName.Name != job.Timezone {
		foundCrontab.TimezoneLocationName = &lib.TimezoneLocationName{Name: job.Timezone}
		hasChanges = true
	}

	if !job.IsMonitored {
		if foundLine.Code != "" {
			foundLine.Code = ""
			hasChanges = true
		}
	} else {
		// Create monitor if not exists
		if foundLine.Code == "" {
			monitor := lib.Monitor{
				Name:        job.Name,
				DefaultName: createDefaultName(foundLine, foundCrontab, "", []string{}, map[string]bool{}),
				Key:         foundLine.Key(foundCrontab.CanonicalName()),
				Schedule:    job.Expression,
				Type:        "job",
				Platform:    lib.CRON,
				Timezone:    job.Timezone,
			}

			monitors := map[string]*lib.Monitor{
				monitor.Key: &monitor,
			}

			updatedMonitors, err := getCronitorApi().PutMonitors(monitors)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if updatedMonitor, exists := updatedMonitors[monitor.Key]; exists {
				foundLine.Mon = *updatedMonitor
				foundLine.Code = updatedMonitor.Code
				hasChanges = true
			}
		}
	}

	// Save changes if any
	if hasChanges {
		if err := foundCrontab.Save(foundCrontab.Write()); err != nil {
			http.Error(w, "Failed to save crontab", http.StatusInternalServerError)
			return
		}
	}

	// Update the job with the latest values
	job.Name = foundLine.Name
	job.Code = foundLine.Code
	job.IsMonitored = len(foundLine.Code) > 0

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func find_instances(commandString string) []JobInstance {
	cmd := exec.Command("ps", "-eo", "pid,lstart,args")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return []JobInstance{}
	}

	lines := strings.Split(out.String(), "\n")
	instances := make([]JobInstance, 0)

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		pid := fields[0]
		// The start time is fields[1:6] (day month date time year)
		started := strings.Join(fields[1:6], " ")
		args := strings.Join(fields[6:], " ")

		if strings.Contains(args, commandString) {
			instances = append(instances, JobInstance{
				PID:     pid,
				Started: started,
			})
		}
	}

	return instances
}

// handleKillJobs handles POST requests to kill processes
func handleKillJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		PIDs []string `json:"pids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	type KillError struct {
		PID   string `json:"pid"`
		Error string `json:"error"`
	}

	var errors []KillError

	for _, pid := range request.PIDs {
		cmd := exec.Command("kill", "-9", pid)
		if err := cmd.Run(); err != nil {
			// Check if the process has already exited
			if strings.Contains(err.Error(), "No such process") {
				continue
			}
			// If it's a permission error, add it to our error list
			if strings.Contains(err.Error(), "Operation not permitted") {
				errors = append(errors, KillError{
					PID:   pid,
					Error: "Insufficient privileges to kill process",
				})
			}
		}
	}

	if len(errors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": errors,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
}
