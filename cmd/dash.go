package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/time/rate"
)

var webAssets embed.FS

func SetWebAssets(assets embed.FS) {
	webAssets = assets
}

// IP Filter for restricting access by IP address
type IPFilter struct {
	allowedNetworks []*net.IPNet
	mutex           sync.RWMutex
}

// NewIPFilter creates a new IP filter instance
func NewIPFilter() *IPFilter {
	return &IPFilter{
		allowedNetworks: make([]*net.IPNet, 0),
	}
}

// LoadAllowedIPs parses the CRONITOR_ALLOWED_IPS environment variable
func (ipf *IPFilter) LoadAllowedIPs() error {
	ipf.mutex.Lock()
	defer ipf.mutex.Unlock()

	allowedIPsEnv := viper.GetString(varAllowedIPs)

	if allowedIPsEnv == "" {
		// No IP filtering if environment variable is empty
		ipf.allowedNetworks = make([]*net.IPNet, 0)
		return nil
	}

	var networks []*net.IPNet
	ipRanges := strings.Split(allowedIPsEnv, ",")

	for _, ipRange := range ipRanges {
		ipRange = strings.TrimSpace(ipRange)
		if ipRange == "" {
			continue
		}

		// Check if it's a single IP (add /32 for IPv4 or /128 for IPv6)
		if !strings.Contains(ipRange, "/") {
			ip := net.ParseIP(ipRange)
			if ip == nil {
				return fmt.Errorf("invalid IP address: %s", ipRange)
			}
			if ip.To4() != nil {
				ipRange += "/32" // IPv4
			} else {
				ipRange += "/128" // IPv6
			}
		}

		_, network, err := net.ParseCIDR(ipRange)
		if err != nil {
			return fmt.Errorf("invalid CIDR notation: %s - %v", ipRange, err)
		}

		networks = append(networks, network)
	}

	ipf.allowedNetworks = networks
	if len(networks) > 0 {
		log(fmt.Sprintf("IP filtering enabled with %d allowed networks", len(networks)))
	}
	return nil
}

// IsAllowed checks if an IP address is allowed
func (ipf *IPFilter) IsAllowed(clientIP string) bool {
	ipf.mutex.RLock()
	defer ipf.mutex.RUnlock()

	// If no IP restrictions are configured, allow all
	if len(ipf.allowedNetworks) == 0 {
		return true
	}

	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}

	// Handle IPv4-mapped IPv6 addresses
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}

	// Check if IP is in any of the allowed networks
	for _, network := range ipf.allowedNetworks {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// createIPFilterMiddleware creates middleware that checks client IP against whitelist
func createIPFilterMiddleware(ipFilter *IPFilter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := getClientIP(r)

			if !ipFilter.IsAllowed(clientIP) {
				log(fmt.Sprintf("IP access denied: %s", clientIP))
				// Close connection silently without any response to make it appear like no service is running
				if hijacker, ok := w.(http.Hijacker); ok {
					conn, _, err := hijacker.Hijack()
					if err == nil {
						conn.Close() // Close connection immediately - appears like connection refused/timeout
						return
					}
				}
				// Fallback: if hijacking fails, just return nothing (no headers, no body)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Global IP filter instance
var ipFilter = NewIPFilter()

// CORS (Cross-Origin Resource Sharing) management
type CORSManager struct {
	allowedOrigins map[string]bool
	mutex          sync.RWMutex
}

// NewCORSManager creates a new CORS manager instance
func NewCORSManager() *CORSManager {
	cm := &CORSManager{
		allowedOrigins: make(map[string]bool),
	}
	return cm
}

// LoadAllowedOrigins parses the CRONITOR_CORS_ALLOWED_ORIGINS environment variable
func (cm *CORSManager) LoadAllowedOrigins() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	allowedOriginsEnv := viper.GetString("CRONITOR_CORS_ALLOWED_ORIGINS")

	if allowedOriginsEnv == "" {
		// Default: only same-origin requests allowed (empty map means strict same-origin policy)
		cm.allowedOrigins = make(map[string]bool)
		return nil
	}

	origins := make(map[string]bool)
	originList := strings.Split(allowedOriginsEnv, ",")

	for _, origin := range originList {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}

		// Validate origin format
		if origin != "*" && !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
			return fmt.Errorf("invalid origin format: %s (must be http:// or https:// or '*')", origin)
		}

		origins[origin] = true
	}

	cm.allowedOrigins = origins
	if len(origins) > 0 {
		log(fmt.Sprintf("CORS enabled with %d allowed origins", len(origins)))
	}
	return nil
}

// IsOriginAllowed checks if an origin is allowed for CORS requests
func (cm *CORSManager) IsOriginAllowed(origin string) bool {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	// If no origins are configured, use strict same-origin policy (no CORS)
	if len(cm.allowedOrigins) == 0 {
		return false
	}

	// Check for wildcard (allow all origins - not recommended for production)
	if cm.allowedOrigins["*"] {
		return true
	}

	// Check for exact origin match
	return cm.allowedOrigins[origin]
}

// createCORSMiddleware creates middleware that handles CORS policies
func createCORSMiddleware(corsManager *CORSManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Handle preflight requests (OPTIONS method)
			if r.Method == "OPTIONS" {
				// Set secure CORS headers for preflight
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token")
				w.Header().Set("Access-Control-Allow-Credentials", "false") // Secure default
				w.Header().Set("Access-Control-Max-Age", "3600")            // Cache preflight for 1 hour

				// Only set Access-Control-Allow-Origin if origin is allowed
				if origin != "" && corsManager.IsOriginAllowed(origin) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

			// For non-preflight requests, set CORS headers if origin is allowed
			if origin != "" && corsManager.IsOriginAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "false") // Secure default
			}

			// Add security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			next.ServeHTTP(w, r)
		})
	}
}

// Global CORS manager instance
var corsManager = NewCORSManager()

// Request size limiting management
type RequestSizeLimiter struct {
	maxBodySize   int64 // Default maximum body size (1MB)
	jsonLimit     int64 // JSON content type limit (100KB)
	formLimit     int64 // Form data limit (10KB)
	maxHeaderSize int64 // Maximum total header size (8KB)
}

// NewRequestSizeLimiter creates a new request size limiter instance
func NewRequestSizeLimiter() *RequestSizeLimiter {
	return &RequestSizeLimiter{
		maxBodySize:   1024 * 1024, // 1MB default
		jsonLimit:     100 * 1024,  // 100KB for JSON
		formLimit:     10 * 1024,   // 10KB for form data
		maxHeaderSize: 8 * 1024,    // 8KB for headers
	}
}

// getContentTypeLimit returns the appropriate size limit based on content type
func (rsl *RequestSizeLimiter) getContentTypeLimit(contentType string) int64 {
	contentType = strings.ToLower(strings.TrimSpace(contentType))

	// Remove charset and boundary information
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	switch contentType {
	case "application/json":
		return rsl.jsonLimit
	case "application/x-www-form-urlencoded":
		return rsl.formLimit
	case "multipart/form-data":
		return rsl.formLimit
	default:
		return rsl.maxBodySize
	}
}

// createRequestSizeLimitMiddleware creates middleware that enforces request size limits
func createRequestSizeLimitMiddleware(limiter *RequestSizeLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// First, check header size limits
			totalHeaderSize := int64(0)
			for name, values := range r.Header {
				totalHeaderSize += int64(len(name))
				for _, value := range values {
					totalHeaderSize += int64(len(value))
				}
			}

			if totalHeaderSize > limiter.maxHeaderSize {
				http.Error(w, fmt.Sprintf("Request headers too large (%d bytes, maximum %d bytes)",
					totalHeaderSize, limiter.maxHeaderSize), http.StatusRequestHeaderFieldsTooLarge)
				return
			}

			// For requests with body, apply size limits
			if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
				contentType := r.Header.Get("Content-Type")
				limit := limiter.getContentTypeLimit(contentType)

				// Check Content-Length header if present
				if contentLengthStr := r.Header.Get("Content-Length"); contentLengthStr != "" {
					if contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64); err == nil {
						if contentLength > limit {
							http.Error(w, fmt.Sprintf("Request body too large (%d bytes, maximum %d bytes for %s)",
								contentLength, limit, contentType), http.StatusRequestEntityTooLarge)
							return
						}
					}
				}

				// Wrap the request body with MaxBytesReader to enforce the limit
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Global request size limiter instance
var requestSizeLimiter = NewRequestSizeLimiter()

// CSRF token management
type CSRFManager struct {
	tokens map[string]time.Time // token -> expiry time
	mutex  sync.RWMutex
}

// NewCSRFManager creates a new CSRF manager instance
func NewCSRFManager() *CSRFManager {
	cm := &CSRFManager{
		tokens: make(map[string]time.Time),
	}

	// Start cleanup goroutine to remove expired tokens every 10 minutes
	go cm.cleanupRoutine()

	return cm
}

// GenerateToken generates a new CSRF token
func (cm *CSRFManager) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	token := hex.EncodeToString(bytes)

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Set token to expire in 1 hour
	cm.tokens[token] = time.Now().Add(1 * time.Hour)

	return token, nil
}

// ValidateToken validates a CSRF token and removes it (one-time use)
func (cm *CSRFManager) ValidateToken(token string) bool {
	if token == "" {
		return false
	}

	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	expiry, exists := cm.tokens[token]
	if !exists {
		return false
	}

	// Check if token has expired
	if time.Now().After(expiry) {
		delete(cm.tokens, token)
		return false
	}

	// Token is valid - remove it (one-time use)
	delete(cm.tokens, token)
	return true
}

// cleanupRoutine removes expired tokens every 10 minutes
func (cm *CSRFManager) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cm.cleanup()
		}
	}
}

// cleanup removes expired tokens
func (cm *CSRFManager) cleanup() {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	now := time.Now()
	for token, expiry := range cm.tokens {
		if now.After(expiry) {
			delete(cm.tokens, token)
		}
	}
}

// Global CSRF manager instance
var csrfManager = NewCSRFManager()

type CommandHistory struct {
	history map[string][]string
}

func NewCommandHistory() *CommandHistory {
	return &CommandHistory{
		history: make(map[string][]string),
	}
}

func (ch *CommandHistory) MoveHistory(oldKey, newKey, oldCommand string) {
	if history, exists := ch.history[oldKey]; exists {
		// Create new history slice with old command first
		newHistory := make([]string, 0, 50)
		newHistory = append(newHistory, oldCommand) // Add the old command to history

		// Add existing history, keeping only the last 49 entries
		startIdx := len(history) - 49
		if startIdx < 0 {
			startIdx = 0
		}
		newHistory = append(newHistory, history[startIdx:]...)

		// Update the history map
		ch.history[newKey] = newHistory
		delete(ch.history, oldKey)
	} else {
		// No existing history, just create new entry with old command
		ch.history[newKey] = []string{oldCommand}
	}

	// Clean up old entries if we have too many keys
	if len(ch.history) > 1000 {
		// Remove half of the oldest entries
		keys := make([]string, 0, len(ch.history))
		for k := range ch.history {
			keys = append(keys, k)
		}
		// Remove first 500 keys (oldest)
		for i := 0; i < 500 && i < len(keys); i++ {
			delete(ch.history, keys[i])
		}
	}
}

func (ch *CommandHistory) GetCommands(key, currentCommand string) []string {
	commands := make([]string, 0)
	commands = append(commands, currentCommand) // Always include current command first

	if history, exists := ch.history[key]; exists {
		commands = append(commands, history...)
	}

	return commands
}

var commandHistory = NewCommandHistory()

var isSafeModeEnabled bool

// Process cache for debouncing expensive ps commands
type processCache struct {
	lines     []string // Parsed ps output lines
	timestamp time.Time
	mutex     sync.RWMutex
}

var psCache = &processCache{}

// getCachedProcessList returns cached ps output or refreshes if older than 5 seconds
func getCachedProcessList() []string {
	psCache.mutex.RLock()
	cacheAge := time.Since(psCache.timestamp)
	hasValidCache := len(psCache.lines) > 0 && cacheAge < 5*time.Second
	if hasValidCache {
		cachedLines := make([]string, len(psCache.lines))
		copy(cachedLines, psCache.lines)
		psCache.mutex.RUnlock()
		return cachedLines
	}
	psCache.mutex.RUnlock()

	// Cache miss or expired - refresh the cache
	psCache.mutex.Lock()
	defer psCache.mutex.Unlock()

	// Double-check in case another goroutine refreshed while we were waiting
	cacheAge = time.Since(psCache.timestamp)
	if len(psCache.lines) > 0 && cacheAge < 5*time.Second {
		cachedLines := make([]string, len(psCache.lines))
		copy(cachedLines, psCache.lines)
		return cachedLines
	}

	// Execute ps command to refresh cache
	cmd := exec.Command("ps", "-eo", "pgid,lstart,args")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		// Return empty cache on error, don't update timestamp so next call will retry
		return []string{}
	}

	// Parse and cache the output
	output := out.String()
	psCache.lines = strings.Split(output, "\n")
	psCache.timestamp = time.Now()

	// Return a copy to avoid external modifications
	cachedLines := make([]string, len(psCache.lines))
	copy(cachedLines, psCache.lines)
	return cachedLines
}

// Simple cache to prevent re-parsing unchanged crontab files
type crontabCache struct {
	data      []*lib.Crontab
	timestamp time.Time
	mutex     sync.RWMutex
}

var cronCache = &crontabCache{}

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

// Add new configuration variables
const (
	varMCPEnabled  = "CRONITOR_MCP_ENABLED"
	varMCPMode     = "CRONITOR_MCP_MODE"
	varMCPInstance = "CRONITOR_MCP_INSTANCE"
)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Start the web dashboard",
	Long: `Start the Crontab Guru web dashboard.
The dashboard provides a web interface for managing your cron jobs and crontab files.

MCP Mode:
When --mcp-instance flag is used, the dashboard runs in MCP (Model Context Protocol) mode
for integration with Cursor IDE or other LLM applications. In this mode, it does
not start a web server but instead communicates via stdio protocol.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get MCP instance name
		mcpInstance, _ := cmd.Flags().GetString("mcp-instance")

		// Check if MCP mode is enabled
		mcpEnabled := false

		// If mcp-instance is provided, automatically enable MCP mode
		if mcpInstance != "" {
			mcpEnabled = true
		} else {
			// Check explicit --mcp flag for backward compatibility
			mcpEnabled, _ = cmd.Flags().GetBool("mcp")
			if !mcpEnabled {
				// Check environment variable
				mcpEnabled = viper.GetBool(varMCPEnabled)
			}

			// If MCP is enabled but no instance specified, use default
			if mcpEnabled && mcpInstance == "" {
				mcpInstance = viper.GetString(varMCPInstance)
				if mcpInstance == "" {
					mcpInstance = "default"
				}
			}
		}

		// If MCP mode is enabled, run MCP server instead of web dashboard
		if mcpEnabled {
			runMCPServer(mcpInstance)
			return
		}

		// Check for dashboard credentials before starting the server
		username := viper.GetString(varDashUsername)
		password := viper.GetString(varDashPassword)

		if username == "" || password == "" {
			fmt.Println("Dashboard credentials are not configured.")
			fmt.Println("Please set a username and password for dashboard access.")
			fmt.Println("These credentials will be saved to your configuration file and can be updated later from the Settings screen.")
			fmt.Println()

			// Prompt for username
			fmt.Print("Enter dashboard username: ")
			reader := bufio.NewReader(os.Stdin)
			usernameInput, err := reader.ReadString('\n')
			if err != nil {
				fatal(fmt.Sprintf("Failed to read username: %v", err), 1)
			}
			usernameInput = strings.TrimSpace(usernameInput)
			if usernameInput == "" {
				fatal("Username cannot be empty", 1)
			}

			// Prompt for password
			fmt.Print("Enter dashboard password: ")
			passwordInput, err := reader.ReadString('\n')
			if err != nil {
				fatal(fmt.Sprintf("Failed to read password: %v", err), 1)
			}
			passwordInput = strings.TrimSpace(passwordInput)
			if passwordInput == "" {
				fatal("Password cannot be empty", 1)
			}

			// Read the current config file
			configPath := configFilePath()
			var configData ConfigFile
			if data, err := ioutil.ReadFile(configPath); err == nil {
				// File exists, parse it
				if err := json.Unmarshal(data, &configData); err != nil {
					// If parsing fails, start with empty config
					configData = ConfigFile{}
				}
			}

			// Update credentials in config data
			configData.DashUsername = usernameInput
			configData.DashPassword = passwordInput

			// Preserve other existing config values
			if configData.ApiKey == "" {
				configData.ApiKey = viper.GetString(varApiKey)
			}
			if configData.PingApiAuthKey == "" {
				configData.PingApiAuthKey = viper.GetString(varPingApiKey)
			}
			if len(configData.ExcludeText) == 0 {
				configData.ExcludeText = viper.GetStringSlice(varExcludeText)
			}
			if configData.Hostname == "" {
				configData.Hostname = viper.GetString(varHostname)
			}
			if configData.Log == "" {
				configData.Log = viper.GetString(varLog)
			}
			if configData.Env == "" {
				configData.Env = viper.GetString(varEnv)
			}
			if configData.AllowedIPs == "" {
				configData.AllowedIPs = viper.GetString(varAllowedIPs)
			}
			if configData.CorsAllowedOrigins == "" {
				configData.CorsAllowedOrigins = viper.GetString("CRONITOR_CORS_ALLOWED_ORIGINS")
			}

			// Marshal the config data
			b, err := json.MarshalIndent(configData, "", "    ")
			if err != nil {
				fatal(fmt.Sprintf("Failed to marshal config data: %v", err), 1)
			}

			// Write to config file
			if err := os.MkdirAll(defaultConfigFileDirectory(), os.ModePerm); err != nil {
				fatal(fmt.Sprintf("Failed to create config directory: %v", err), 1)
			}

			if err := ioutil.WriteFile(configPath, b, 0644); err != nil {
				fatal(fmt.Sprintf("Failed to write config file: %v", err), 1)
			}

			fmt.Printf("\nCredentials saved to: %s\n", configPath)
			fmt.Println("Please run 'cronitor dash' again to start the dashboard.")
			os.Exit(0)
		}

		port, _ := cmd.Flags().GetInt("port")
		if port == 0 {
			port = 9000
		}

		safeMode, _ := cmd.Flags().GetBool("safe-mode")
		isSafeModeEnabled = safeMode

		// Load IP filtering configuration
		if err := ipFilter.LoadAllowedIPs(); err != nil {
			fatal(fmt.Sprintf("Failed to load IP filtering configuration: %v", err), 1)
		}

		// Load CORS configuration
		if err := corsManager.LoadAllowedOrigins(); err != nil {
			fatal(fmt.Sprintf("Failed to load CORS configuration: %v", err), 1)
		}

		// Create middleware
		requestSizeLimitMiddleware := createRequestSizeLimitMiddleware(requestSizeLimiter)
		ipFilterMiddleware := createIPFilterMiddleware(ipFilter)
		corsMiddleware := createCORSMiddleware(corsManager)

		// Helper function to chain middleware
		chainMiddleware := func(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
			// Apply middleware in reverse order so they execute in the order specified
			for i := len(middlewares) - 1; i >= 0; i-- {
				handler = middlewares[i](handler)
			}
			return handler
		}

		// CSRF middleware
		csrfMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// For GET requests, generate a new CSRF token and set it in a cookie
				if r.Method == "GET" {
					token, err := csrfManager.GenerateToken()
					if err != nil {
						http.Error(w, "Failed to generate CSRF token", http.StatusInternalServerError)
						return
					}

					// Set CSRF token in secure HTTP-only cookie with SameSite=Strict
					cookie := &http.Cookie{
						Name:     "csrf_token",
						Value:    token,
						Path:     "/",
						HttpOnly: true,
						SameSite: http.SameSiteStrictMode,
						Secure:   false, // Set to true when HTTPS is implemented
						MaxAge:   3600,  // 1 hour
					}
					http.SetCookie(w, cookie)

					// Also send token in response header for AJAX requests
					w.Header().Set("X-CSRF-Token", token)
				} else if r.Method == "POST" || r.Method == "PUT" || r.Method == "DELETE" {
					// For state-changing requests, validate CSRF token
					var token string

					// Check for CSRF token in X-CSRF-Token header first (for AJAX requests)
					token = r.Header.Get("X-CSRF-Token")

					// If not found in header, check in form data
					if token == "" {
						token = r.FormValue("csrf_token")
					}

					// DO NOT accept CSRF tokens from cookies - this would defeat CSRF protection
					// since cookies are automatically sent by browsers, including in CSRF attacks

					// Validate the token
					if !csrfManager.ValidateToken(token) {
						http.Error(w, "CSRF token validation failed", http.StatusForbidden)
						return
					}
				}

				next.ServeHTTP(w, r)
			})
		}

		// Basic auth middleware with rate limiting
		authMiddleware := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username := viper.GetString(varDashUsername)
				password := viper.GetString(varDashPassword)

				// Get client IP for rate limiting
				clientIP := getClientIP(r)

				auth := r.Header.Get("Authorization")
				if auth == "" {
					w.Header().Set("WWW-Authenticate", `Basic realm="Crontab Guru Dashboard"`)
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				payload, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
				pair := strings.SplitN(string(payload), ":", 2)

				// Check for authentication failure
				if len(pair) != 2 || pair[0] != username || pair[1] != password {
					// Get rate limiter for this IP
					limiter := authRateLimiter.GetLimiter(clientIP)

					// Check if rate limit exceeded
					if !limiter.Allow() {
						// Rate limit exceeded - return 429 with Retry-After header
						retryAfter := int(limiter.Reserve().Delay().Seconds())
						if retryAfter <= 0 {
							retryAfter = 12 // Default to 12 seconds based on our rate (every 12 seconds)
						}

						w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
						http.Error(w, "Too Many Requests - Rate limit exceeded", http.StatusTooManyRequests)
						return
					}

					// Not rate limited, but still unauthorized
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}

				// Authentication successful - regenerate CSRF token to prevent session fixation
				if token, err := csrfManager.GenerateToken(); err == nil {
					cookie := &http.Cookie{
						Name:     "csrf_token",
						Value:    token,
						Path:     "/",
						HttpOnly: true,
						SameSite: http.SameSiteStrictMode,
						Secure:   false, // Set to true when HTTPS is implemented
						MaxAge:   3600,  // 1 hour
					}
					http.SetCookie(w, cookie)
					w.Header().Set("X-CSRF-Token", token)
				}

				// Authentication successful - proceed to next handler
				next.ServeHTTP(w, r)
			})
		}

		// Get the embedded filesystem
		fsys, err := fs.Sub(webAssets, "web/static")
		if err != nil {
			fatal(err.Error(), 1)
		}

		// Create a custom file server that serves index.html for all routes
		fileServer := http.FileServer(http.FS(fsys))
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Don't serve index.html for API routes
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}

			// For static assets in the /static/ directory
			if strings.HasPrefix(r.URL.Path, "/static/") {
				// The files are already in the root of fsys, so we need to remove /static prefix
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/static")
				fileServer.ServeHTTP(w, r)
				return
			}

			// List of known static files that should be served from root
			staticFiles := []string{
				"/manifest.json",
				"/favicon.ico",
				"/favicon.png",
				"/logo192.png",
				"/logo512.png",
				"/asset-manifest.json",
			}

			// Check if this is a known static file
			for _, staticFile := range staticFiles {
				if r.URL.Path == staticFile {
					// Serve the file directly without modifying the path
					fileServer.ServeHTTP(w, r)
					return
				}
			}

			// For all other routes (including /), serve index.html for client-side routing
			index, err := fsys.Open("index.html")
			if err != nil {
				http.Error(w, "Not Found", http.StatusNotFound)
				return
			}
			defer index.Close()
			http.ServeContent(w, r, "index.html", time.Now(), index.(io.ReadSeeker))
		})

		// Create a separate handler for public static files (no auth required)
		publicFiles := []string{
			"/manifest.json",
			"/favicon.ico",
			"/favicon.png",
			"/logo192.png",
			"/logo512.png",
		}

		publicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if this is a public file
			for _, publicFile := range publicFiles {
				if r.URL.Path == publicFile {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			// Not a public file, pass to main handler with auth
			authMiddleware(csrfMiddleware(handler)).ServeHTTP(w, r)
		})

		// Define common middleware chains
		baseMiddleware := []func(http.Handler) http.Handler{
			requestSizeLimitMiddleware,
			ipFilterMiddleware,
			corsMiddleware,
		}

		apiMiddleware := append(baseMiddleware, authMiddleware, csrfMiddleware)

		// Apply request size limits, IP filter, and CORS to all routes, but auth only where needed
		http.Handle("/", chainMiddleware(publicHandler, baseMiddleware...))

		// Register API endpoints with middleware
		apiRoutes := map[string]http.HandlerFunc{
			"/api/settings":       handleSettings,
			"/api/jobs":           handleJobs,
			"/api/crontabs":       handleCrontabs,
			"/api/crontabs/":      handleCrontab,
			"/api/users":          handleUsers,
			"/api/jobs/kill":      handleKillInstances,
			"/api/jobs/run":       handleRunJob,
			"/api/monitors":       handleGetMonitors,
			"/api/signup":         handleSignup,
			"/api/update/check":   handleUpdateCheck,
			"/api/update/perform": handleUpdatePerform,
		}

		for path, handler := range apiRoutes {
			http.Handle(path, chainMiddleware(http.HandlerFunc(handler), apiMiddleware...))
		}

		// Create HTTP server with proper configuration
		server := &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      nil, // Use default ServeMux
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}

		// Start the server in a goroutine
		go func() {
			fmt.Printf("Starting Cronitor dashboard on port %d...\n", port)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fatal(err.Error(), 1)
			}
		}()

		// Wait a moment for the server to start
		time.Sleep(500 * time.Millisecond)

		// Open the browser
		url := fmt.Sprintf("http://localhost:%d", port)
		fmt.Printf("Opening browser to %s...\n", url)
		openBrowser(url)

		// Wait for interrupt signal to gracefully shutdown
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		fmt.Printf("Dashboard running on %s (Press Ctrl+C to stop)\n", url)
		<-c

		fmt.Println("\nShutting down dashboard...")

		// Create a context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			fmt.Printf("Server forced to shutdown: %v\n", err)
		} else {
			fmt.Println("Dashboard stopped gracefully")
		}
	},
}

// runMCPServer starts the MCP server for Cursor integration
func runMCPServer(instanceName string) {
	// Create MCP server
	s := server.NewMCPServer(
		fmt.Sprintf("Cronitor Dashboard (%s)", instanceName),
		Version,
		server.WithToolCapabilities(false),
	)

	// Create and register the Cronitor MCP handler
	handler := lib.NewCronitorMCPHandler(instanceName)

	// Register tools
	if err := handler.RegisterTools(s); err != nil {
		fatal(fmt.Sprintf("Failed to register MCP tools: %v", err), 1)
	}

	// Register resources
	if err := handler.RegisterResources(s); err != nil {
		fatal(fmt.Sprintf("Failed to register MCP resources: %v", err), 1)
	}

	// Start the stdio server
	if err := server.ServeStdio(s); err != nil {
		fatal(fmt.Sprintf("MCP server error: %v", err), 1)
	}
}

// Helper function to slugify a string for filenames
func slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)
	// Replace spaces with hyphens
	s = strings.ReplaceAll(s, " ", "-")
	// Remove any non-alphanumeric characters
	s = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(s, "")
	return s
}

// Helper function to add a line to a crontab file
func addLineToCrontab(file string, line string) error {
	// If the file doesn't exist and it's in /etc/cron.d, create it
	if strings.HasPrefix(file, "/etc/cron.d") {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			// Create the file with proper permissions
			if err := os.WriteFile(file, []byte(line+"\n"), 0644); err != nil {
				return err
			}
			return nil
		}
	}

	// Otherwise append to existing file
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(line + "\n"); err != nil {
		return err
	}

	return nil
}

func init() {
	RootCmd.AddCommand(dashCmd)
	dashCmd.Flags().Int("port", 9000, "Port to run the dashboard on")
	dashCmd.Flags().Bool("safe-mode", false, "Limit the ability to edit jobs, crontabs, and settings")
	dashCmd.Flags().Bool("mcp", false, "Enable MCP server mode for Cursor integration (deprecated: use --mcp-instance instead)")
	dashCmd.Flags().String("mcp-instance", "", "MCP instance name to connect to (automatically enables MCP mode)")

	// Bind MCP flags to viper
	viper.BindPFlag(varMCPEnabled, dashCmd.Flags().Lookup("mcp"))
	viper.BindPFlag(varMCPInstance, dashCmd.Flags().Lookup("mcp-instance"))
}

type SettingsResponse struct {
	ConfigFile
	EnvVars            map[string]bool              `json:"env_vars"`
	ConfigFilePath     string                       `json:"config_file_path"`
	Version            string                       `json:"version"`
	Hostname           string                       `json:"hostname"`
	Timezone           string                       `json:"timezone"`
	Timezones          []string                     `json:"timezones"`
	OS                 string                       `json:"os"`
	SafeMode           bool                         `json:"safe_mode"`
	UpdateStatus       *UpdateStatus                `json:"update_status,omitempty"`
	AllowedIPs         string                       `json:"allowed_ips"`
	CorsAllowedOrigins string                       `json:"cors_allowed_origins"`
	ClientIP           string                       `json:"client_ip"`
	MCPEnabled         bool                         `json:"mcp_enabled"`
	MCPInstances       map[string]MCPInstanceConfig `json:"mcp_instances,omitempty"`
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

		// Get list of timezones
		timezones := []string{}

		// Get the system's timezone first
		systemTZ := effectiveTimezoneLocationName().Name

		// Try to read from system timezone database
		zoneDirs := []string{
			"/usr/share/zoneinfo",
			"/usr/lib/zoneinfo",
			"/usr/share/lib/zoneinfo",
			"/etc/zoneinfo",
			"/var/db/timezone/zoneinfo", // macOS location
		}

		var zoneDir string
		for _, dir := range zoneDirs {
			if _, err := os.Stat(dir); err == nil {
				// Follow symlinks to get the actual directory
				realPath, err := filepath.EvalSymlinks(dir)
				if err == nil {
					zoneDir = realPath
					break
				}
			}
		}

		if zoneDir != "" {
			err := filepath.Walk(zoneDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				// Skip the root directory itself
				if path == zoneDir {
					return nil
				}
				if !info.IsDir() {
					// Convert path to timezone name by removing the zoneDir prefix
					tz := strings.TrimPrefix(path, zoneDir+"/")
					// Skip any files that don't look like timezone files
					if strings.HasPrefix(tz, ".") {
						return nil
					}
					if _, err := time.LoadLocation(tz); err == nil {
						// Skip the system timezone as we'll add it at the top
						if tz != systemTZ {
							timezones = append(timezones, tz)
						}
					}
				}
				return nil
			})
			if err != nil {
				fmt.Printf("Error walking timezone directory: %v\n", err)
			}
		}

		// Sort the timezones alphabetically
		sort.Strings(timezones)

		// Add system timezone at the top
		timezones = append([]string{systemTZ}, timezones...)

		// Check for updates with a reasonable timeout to avoid slowing down the response
		var updateStatus *UpdateStatus
		updateDone := make(chan *UpdateStatus, 1)
		go func() {
			if status, err := checkForUpdate(); err == nil {
				updateDone <- status
			} else {
				updateDone <- nil
			}
		}()

		// Wait for update check with timeout
		select {
		case status := <-updateDone:
			updateStatus = status
		case <-time.After(5 * time.Second):
			// Timeout after 5 seconds - proceed without update status
		}

		// Create response with env var information
		response := SettingsResponse{
			ConfigFile:     configData,
			ConfigFilePath: configPath,
			Version:        Version,
			Hostname:       effectiveHostname(),
			Timezone:       effectiveTimezoneLocationName().Name,
			Timezones:      timezones,
			EnvVars: map[string]bool{
				"CRONITOR_API_KEY":              os.Getenv(varApiKey) != "",
				"CRONITOR_PING_API_KEY":         os.Getenv(varPingApiKey) != "",
				"CRONITOR_EXCLUDE_TEXT":         os.Getenv(varExcludeText) != "",
				"CRONITOR_HOSTNAME":             os.Getenv(varHostname) != "",
				"CRONITOR_LOG":                  os.Getenv(varLog) != "",
				"CRONITOR_ENV":                  os.Getenv(varEnv) != "",
				"CRONITOR_DASH_USER":            os.Getenv(varDashUsername) != "",
				"CRONITOR_DASH_PASS":            os.Getenv(varDashPassword) != "",
				"CRONITOR_ALLOWED_IPS":          os.Getenv(varAllowedIPs) != "",
				"CRONITOR_CORS_ALLOWED_ORIGINS": os.Getenv("CRONITOR_CORS_ALLOWED_ORIGINS") != "",
				"CRONITOR_USERS":                os.Getenv(varUsers) != "",
				"CRONITOR_MCP_ENABLED":          os.Getenv(varMCPEnabled) != "",
			},
			OS:                 runtime.GOOS,
			SafeMode:           isSafeModeEnabled,
			UpdateStatus:       updateStatus,
			AllowedIPs:         os.Getenv(varAllowedIPs),
			CorsAllowedOrigins: os.Getenv("CRONITOR_CORS_ALLOWED_ORIGINS"),
			ClientIP:           getClientIP(r),
			MCPEnabled:         configData.MCPEnabled,
			MCPInstances:       configData.MCPInstances,
		}

		// Override config values with environment variables if they exist
		if os.Getenv(varDashUsername) != "" {
			response.DashUsername = os.Getenv(varDashUsername)
		}
		if os.Getenv(varDashPassword) != "" {
			response.DashPassword = os.Getenv(varDashPassword)
		}
		if os.Getenv(varAllowedIPs) != "" {
			response.AllowedIPs = os.Getenv(varAllowedIPs)
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
		if !viper.IsSet(varUsers) {
			viper.Set(varUsers, configData.Users)
		}

		// Handle AllowedIPs specially - always update if not overridden by environment variable
		if os.Getenv(varAllowedIPs) == "" {
			viper.Set(varAllowedIPs, configData.AllowedIPs)
			// ALWAYS reload IP filter configuration when AllowedIPs changes through settings
			if err := ipFilter.LoadAllowedIPs(); err != nil {
				http.Error(w, fmt.Sprintf("Invalid IP configuration: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Handle CORS allowed origins - always update if not overridden by environment variable
		if os.Getenv("CRONITOR_CORS_ALLOWED_ORIGINS") == "" {
			viper.Set("CRONITOR_CORS_ALLOWED_ORIGINS", configData.CorsAllowedOrigins)
			// ALWAYS reload CORS configuration when CorsAllowedOrigins changes through settings
			if err := corsManager.LoadAllowedOrigins(); err != nil {
				http.Error(w, fmt.Sprintf("Invalid CORS configuration: %v", err), http.StatusBadRequest)
				return
			}
		}

		// Handle MCP settings
		if os.Getenv(varMCPEnabled) == "" {
			viper.Set(varMCPEnabled, configData.MCPEnabled)
		}

		// Always update MCP instances configuration
		if configData.MCPInstances != nil {
			viper.Set("mcp_instances", configData.MCPInstances)
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

		// Force viper to reload the configuration from the file to pick up changes immediately
		// This ensures API key changes take effect without requiring a server restart
		if err := viper.ReadInConfig(); err != nil {
			log("Warning: Failed to reload config file after settings update: " + err.Error())
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
	Name               string        `json:"name"`
	DefaultName        string        `json:"default_name"`
	Command            string        `json:"command"`
	Expression         string        `json:"expression"`
	RunAsUser          string        `json:"run_as_user"`
	CrontabDisplayName string        `json:"crontab_display_name"`
	CrontabFilename    string        `json:"crontab_filename"`
	LineNumber         int           `json:"line_number"`
	Monitored          bool          `json:"monitored"`
	Timezone           string        `json:"timezone"`
	Passing            bool          `json:"passing"`
	Disabled           bool          `json:"disabled"`
	Paused             bool          `json:"paused"`
	Initialized        bool          `json:"initialized"`
	Code               string        `json:"code"`
	Key                string        `json:"key"`
	Instances          []JobInstance `json:"instances"`
	Suspended          *bool         `json:"suspended"`
	PauseHours         string        `json:"pause_hours"`
	IsMetaCronJob      bool          `json:"is_meta_cron_job"`
	Ignored            bool          `json:"ignored"`
}

// handleJobs handles requests for jobs
func handleJobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		handleGetJobs(w, r)
	case "PUT":
		handlePutJob(w, r)
	case "DELETE":
		handleDeleteJob(w, r)
	case "POST":
		handlePostJob(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetJobs(w http.ResponseWriter, r *http.Request) {
	// Parse crontabs
	crontabs, err := lib.GetAllCrontabs(parseUsers())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if this request is from the Crontabs view (has a 'key' query parameter)
	includeIgnoredJobs := r.URL.Query().Get("key") != ""

	var jobs []Job

	// Process each crontab
	for _, crontab := range crontabs {
		for i := range crontab.Lines {
			line := crontab.Lines[i]
			if !line.IsJob {
				continue
			}

			// Skip ignored jobs only if not requested from Crontabs view
			if line.Ignored && !includeIgnoredJobs {
				continue
			}

			timezone := effectiveTimezoneLocationName().Name
			if crontab.TimezoneLocationName != nil {
				timezone = crontab.TimezoneLocationName.Name
			}

			runAsUser := line.RunAs
			if runAsUser == "" {
				runAsUser = crontab.User
			}

			jobKey := line.Key(crontab.CanonicalName())

			// Basic exclusions for cleaner names
			excludeFromName := []string{
				"2>&1",
				"/bin/bash -l -c",
				"/bin/bash -lc",
				"/bin/bash -c -l",
				"/bin/bash -cl",
				"/dev/null",
				"'",
				"\"",
				"\\",
			}
			allNameCandidates := make(map[string]bool)

			job := Job{
				Name:               line.Name,
				DefaultName:        createDefaultName(line, crontab, "", excludeFromName, allNameCandidates),
				Command:            line.CommandToRun,
				Expression:         line.CronExpression,
				RunAsUser:          runAsUser,
				CrontabDisplayName: crontab.DisplayName(),
				CrontabFilename:    crontab.Filename,
				LineNumber:         line.LineNumber,
				Monitored:          len(line.Code) > 0,
				Timezone:           timezone,
				Passing:            false,
				Disabled:           false,
				Paused:             false,
				Initialized:        false,
				Code:               line.Code,
				Key:                jobKey,
				Instances:          []JobInstance{}, // Will be populated below if conditions are met
				Suspended:          &line.IsComment,
				IsMetaCronJob:      line.IsMetaCronJob(),
				Ignored:            line.Ignored,
			}

			jobs = append(jobs, job)

			// Prevent memory issues by limiting the number of jobs returned
			if len(jobs) > 1000 { // Reasonable limit
				log("Warning: Too many jobs found, truncating response")
				break
			}
		}

	}

	// Populate instances for jobs using command history, but limit to first 50 jobs to prevent performance issues
	if len(jobs) > 0 {
		maxJobsForInstances := 50
		if len(jobs) < maxJobsForInstances {
			maxJobsForInstances = len(jobs)
		}

		jobsWithInstances := 0

		for i := 0; i < maxJobsForInstances; i++ {
			job := &jobs[i]
			// Skip if job is suspended/commented out or ignored
			if (job.Suspended != nil && *job.Suspended) || job.Ignored {
				continue
			}

			if job.Command != "" {
				// Get full command history for this job (current + historical commands)
				commands := commandHistory.GetCommands(job.Key, job.Command)

				// Find instances using the full command history
				matchingInstances := findInstances(commands)

				if len(matchingInstances) > 0 {
					job.Instances = matchingInstances
					jobsWithInstances++
				}
			}
		}
	}

	responseData, err := json.Marshal(jobs)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Write(responseData)
}

// handleGetMonitors handles GET requests for monitor data
func handleGetMonitors(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	monitors, err := getCronitorApi().GetMonitors()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Sync monitor names with crontab job names
	go func() {
		// Get all crontabs for name syncing
		crontabs, err := lib.GetAllCrontabs(parseUsers())
		if err != nil {
			log(fmt.Sprintf("Warning: Failed to get crontabs for name syncing: %v", err))
			return
		}

		var crontabsToSave []*lib.Crontab

		// Process each crontab to sync monitor names
		for _, crontab := range crontabs {
			crontabModified := false

			for i := range crontab.Lines {
				line := crontab.Lines[i]
				if !line.IsJob || line.Ignored {
					continue
				}

				// Check if this job has a monitor and if the name needs updating
				jobKey := line.Key(crontab.CanonicalName())
				if len(line.Code) > 0 {
					for _, monitor := range monitors {
						// Match by code or key
						if (monitor.Attributes.Code == line.Code) || monitor.Key == jobKey {
							// If monitor name differs from crontab line name, update the crontab
							if monitor.Name != "" && monitor.Name != line.Name {
								line.Name = monitor.Name
								crontabModified = true
							}
							break
						}
					}
				}
			}

			// If this crontab was modified, mark it for saving
			if crontabModified {
				crontabsToSave = append(crontabsToSave, crontab)
			}
		}

		// Save any modified crontabs
		for _, crontab := range crontabsToSave {
			if err := crontab.Save(crontab.Write()); err != nil {
				log(fmt.Sprintf("Warning: Failed to save crontab %s after syncing monitor names: %v", crontab.Filename, err))
			}
		}

		if len(crontabsToSave) > 0 {
			log(fmt.Sprintf("Synced monitor names for %d crontab(s)", len(crontabsToSave)))
		}
	}()

	responseData, err := json.Marshal(monitors)
	if err != nil {
		http.Error(w, "Failed to marshal response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(responseData)
}

// handleSignup handles POST requests to sign up for a new Cronitor account
func handleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if request.Name == "" || request.Email == "" || request.Password == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	if !strings.Contains(request.Email, "@") || len(request.Email) < 5 {
		http.Error(w, "Please enter a valid email address", http.StatusBadRequest)
		return
	}

	if len(request.Password) < 8 {
		http.Error(w, "Password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	// Call the Cronitor API to sign up
	api := lib.CronitorApi{
		UserAgent: "cronitor-cli",
	}

	resp, err := api.Signup(request.Name, request.Email, request.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save the API keys to config
	viper.Set(varApiKey, resp.ApiKey)
	viper.Set(varPingApiKey, resp.PingApiKey)

	// Write config to file
	if err := viper.WriteConfig(); err != nil {
		// Try to create config directory if it doesn't exist
		if err := os.MkdirAll(defaultConfigFileDirectory(), os.ModePerm); err == nil {
			viper.WriteConfig()
		}
	}

	// Return the API keys
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"api_key":      resp.ApiKey,
		"ping_api_key": resp.PingApiKey,
	})
}

// findInstances finds running instances of commands using cached ps output
func findInstances(commandStrings []string) []JobInstance {
	if len(commandStrings) == 0 {
		return []JobInstance{}
	}

	lines := getCachedProcessList()
	instances := make([]JobInstance, 0)
	seenPGIDs := make(map[string]bool) // To avoid duplicate entries

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		pgid := fields[0] // Process group ID
		// The start time is fields[1:6] (day month date time year)
		started := strings.Join(fields[1:6], " ")
		args := strings.Join(fields[6:], " ")

		// Skip if this is a cronitor exec command
		if strings.Contains(args, "cronitor exec") {
			continue
		}

		// Check if any of the command strings match
		for _, cmdStr := range commandStrings {
			if strings.Contains(args, cmdStr) && !seenPGIDs[pgid] {
				instances = append(instances, JobInstance{
					PID:     pgid,
					Started: started,
				})
				seenPGIDs[pgid] = true
				break
			}
		}
	}

	return instances
}

// handleRunJob handles POST requests to run a job
func handleRunJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Command         string `json:"command"`
		CrontabFilename string `json:"crontab_filename"`
		Key             string `json:"key"`
		WithMonitoring  bool   `json:"with_monitoring"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Command == "" {
		http.Error(w, "Command parameter is required", http.StatusBadRequest)
		return
	}

	shell := "/bin/sh"     // Default shell
	var monitorCode string // For monitoring

	// If crontab filename and key are provided, use them to find the specific job
	if request.CrontabFilename != "" && request.Key != "" {
		crontab, err := lib.GetCrontab(request.CrontabFilename)
		if err != nil {
			http.Error(w, "Crontab not found", http.StatusForbidden)
			return
		} else {
			// Use the shell from this crontab
			if crontab.Shell != "" {
				shell = crontab.Shell
			}

			// Find the specific job by key
			var foundLine *lib.Line
			for _, line := range crontab.Lines {
				if line.IsJob && line.Key(crontab.CanonicalName()) == request.Key {
					foundLine = line
					break
				}
			}

			if foundLine != nil {
				// Validate that the command matches what's in the crontab
				if foundLine.CommandToRun != request.Command && isSafeModeEnabled {
					http.Error(w, "Command does not match the job in the crontab", http.StatusForbidden)
					return
				}
				// Get monitor code if monitoring is requested
				if request.WithMonitoring && foundLine.Code != "" {
					monitorCode = foundLine.Code
				}
			} else if isSafeModeEnabled {
				http.Error(w, "Job not found in crontab", http.StatusForbidden)
				return
			}
		}
	} else if isSafeModeEnabled {
		// In safe mode, crontab filename and key are required
		http.Error(w, "Crontab filename and key are required in safe mode", http.StatusForbidden)
		return
	}

	// Create a temporary file for the output
	tempFile, err := ioutil.TempFile("", "cronitor-job-*.log")
	if err != nil {
		http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Set up SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Create a channel to signal when the command is done
	done := make(chan struct{})

	// Track the last position we read from the file
	lastPos := int64(0)
	const maxOutputSize = 10 * 1024 * 1024 // 10MB limit
	const maxChunkSize = 8192              // 8KB chunks
	var totalSent int64 = 0

	// Create context with timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// For monitoring support
	var series string
	var startTimestamp float64

	if request.WithMonitoring && monitorCode != "" {
		startTimestamp = makeStamp()
		series = formatStamp(startTimestamp)
		// Send run ping without waitgroup
		go sendPing("run", monitorCode, request.Command, series, startTimestamp, nil, nil, nil, "", nil)
	}

	// Start the command in a goroutine
	go func() {
		defer close(done)

		startTime := time.Now()
		cmd := exec.CommandContext(ctx, shell, "-c", request.Command)
		cmd.Env = makeCronLikeEnv()
		cmd.Stdout = tempFile
		cmd.Stderr = tempFile

		// Create a new process group for each "run now" command
		// This ensures each invocation gets its own PGID for proper tracking
		cmd.SysProcAttr = getPlatformSysProcAttrForDash()

		err := cmd.Start()
		if err != nil {
			errorData, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("Error starting command: %v", err)})
			fmt.Fprintf(w, "data: %s\n\n", errorData)
			w.(http.Flusher).Flush()
			return
		}

		// Send the PID back to the client
		pidData, _ := json.Marshal(map[string]int{"pid": cmd.Process.Pid})
		fmt.Fprintf(w, "data: %s\n\n", pidData)
		w.(http.Flusher).Flush()

		// Invalidate server-side caches so new instances show up immediately
		invalidateCrontabCache()
		// Also invalidate process cache since a new process was started
		invalidateProcessCache()

		err = cmd.Wait()
		duration := time.Since(startTime)
		exitCode := 0

		// Ensure temp file is synced before reading final output
		tempFile.Sync()

		// Handle monitoring if enabled
		if request.WithMonitoring && monitorCode != "" {
			endTimestamp := makeStamp()
			durationSeconds := endTimestamp - startTimestamp

			// Gather output for monitoring (truncated for ping)
			outputForPing := gatherOutputForDash(tempFile, true)
			outputStr := string(outputForPing)

			// Calculate metrics
			fileSize, _ := getFileSize(tempFile)
			metrics := map[string]int{
				"length": int(fileSize),
			}

			if err != nil {
				// Command failed
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = 1
				}

				message := strings.TrimSpace(fmt.Sprintf("%s [%s]", outputStr, err.Error()))
				go sendPing("fail", monitorCode, message, series, endTimestamp, &durationSeconds, &exitCode, metrics, "", nil)
			} else {
				// Command succeeded
				go sendPing("complete", monitorCode, outputStr, series, endTimestamp, &durationSeconds, &exitCode, metrics, "", nil)
			}

			// Ship full log data
			go shipLogDataForDash(tempFile, monitorCode, series)
		}

		// Determine exit status and reason
		var statusMsg string
		if ctx.Err() == context.DeadlineExceeded {
			statusMsg = fmt.Sprintf("Timed out after %.2f seconds", duration.Seconds())
		} else if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				// Check for kill signals and common termination codes
				if exitCode == -1 || exitCode == 130 || exitCode == 137 || exitCode == 143 || exitCode == 9 {
					// Common kill/interrupt exit codes:
					// 130 = Ctrl+C (SIGINT), 137 = SIGKILL, 143 = SIGTERM, 9 = SIGKILL, -1 = killed
					statusMsg = fmt.Sprintf("Killed after %.2f seconds [Exit code %d]", duration.Seconds(), exitCode)
				} else {
					statusMsg = fmt.Sprintf("Done in %.2f seconds [Exit code %d]", duration.Seconds(), exitCode)
				}
			} else {
				// Check if the error indicates the process was killed
				errStr := err.Error()
				if strings.Contains(errStr, "killed") || strings.Contains(errStr, "signal") {
					statusMsg = fmt.Sprintf("Killed after %.2f seconds: %v", duration.Seconds(), err)
				} else {
					statusMsg = fmt.Sprintf("Failed after %.2f seconds: %v", duration.Seconds(), err)
				}
			}
		} else {
			statusMsg = fmt.Sprintf("Done in %.2f seconds [Exit code %d]", duration.Seconds(), exitCode)
		}

		// Give a brief moment for output to be flushed to the temp file
		time.Sleep(200 * time.Millisecond)

		// Send completion message - retry a few times if needed
		completionData, _ := json.Marshal(map[string]string{
			"completion": statusMsg,
		})

		// Try to send completion message multiple times to ensure delivery
		for attempts := 0; attempts < 3; attempts++ {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", completionData); err != nil {
				log(fmt.Sprintf("Failed to send completion message (attempt %d): %v", attempts+1, err))
				time.Sleep(100 * time.Millisecond)
				continue
			}

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			break
		}
	}()

	// Stream the log file contents with memory limits
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // More frequent polling for better responsiveness
		defer ticker.Stop()

		for {
			select {
			case <-done:
				// Give extra time for output to be written to file after command completion
				time.Sleep(300 * time.Millisecond)
				// Read any final output multiple times to ensure we get everything
				for i := 0; i < 3; i++ {
					streamRemainingOutput(tempFile, &lastPos, &totalSent, maxOutputSize, maxChunkSize, w)
					time.Sleep(50 * time.Millisecond)
				}
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				streamRemainingOutput(tempFile, &lastPos, &totalSent, maxOutputSize, maxChunkSize, w)
			}
		}
	}()

	// Wait for the command to complete or timeout
	select {
	case <-done:
	case <-ctx.Done():
		fmt.Fprintf(w, "data: %s\n\n", `{"error":"Command timed out"}`)
		w.(http.Flusher).Flush()
	}
}

// gatherOutputForDash is similar to gatherOutput in exec.go but adapted for dashboard
func gatherOutputForDash(tempFile *os.File, truncateForPingOutput bool) []byte {
	var outputBytes []byte
	const outputForPingMaxLen int64 = 1000
	const outputForLogUploadMaxLen int64 = 100000000

	if size, err := getFileSize(tempFile); err == nil {
		// In all cases, if we have to truncate, we want to read the END
		// of the log file, because it is more informative.
		if truncateForPingOutput && size > outputForPingMaxLen {
			outputBytes = make([]byte, outputForPingMaxLen)
			tempFile.Seek(outputForPingMaxLen*-1, 2)
		} else if !truncateForPingOutput && size > outputForLogUploadMaxLen {
			outputBytes = make([]byte, outputForLogUploadMaxLen)
			tempFile.Seek(outputForLogUploadMaxLen*-1, 2)
		} else {
			outputBytes = make([]byte, size)
			tempFile.Seek(0, 0)
		}
		tempFile.Read(outputBytes)
	}

	return outputBytes
}

// shipLogDataForDash sends full log data to Cronitor, similar to shipLogData in exec.go but without waitgroup
func shipLogDataForDash(tempFile *os.File, monitorCode string, series string) {
	outputForLogs := gatherOutputForDash(tempFile, false)
	_, err := lib.SendLogData(viper.GetString(varApiKey), monitorCode, series, string(outputForLogs))
	if err != nil {
		log(fmt.Sprintf("Failed to ship log data: %v", err))
	}
}

// Helper function to stream output with memory limits
func streamRemainingOutput(tempFile *os.File, lastPos *int64, totalSent *int64, maxOutputSize int64, maxChunkSize int64, w http.ResponseWriter) {
	if *totalSent >= maxOutputSize {
		return // Stop streaming if we've hit the limit
	}

	fileInfo, err := tempFile.Stat()
	if err != nil {
		return
	}

	if fileInfo.Size() > *lastPos {
		remainingAllowed := maxOutputSize - *totalSent
		toRead := fileInfo.Size() - *lastPos

		if toRead > remainingAllowed {
			toRead = remainingAllowed
		}
		if toRead > maxChunkSize {
			toRead = maxChunkSize
		}

		// Read only the new portion in a small chunk
		buffer := make([]byte, toRead)
		tempFile.Seek(*lastPos, 0)
		n, err := tempFile.Read(buffer)
		if err != nil && err != io.EOF {
			return
		}

		if n > 0 {
			// Truncate to actual bytes read
			newContent := string(buffer[:n])
			outputData, _ := json.Marshal(map[string]string{"output": newContent})
			fmt.Fprintf(w, "data: %s\n\n", outputData)
			w.(http.Flusher).Flush()
			*lastPos += int64(n)
			*totalSent += int64(n)
		}

		if *totalSent >= maxOutputSize {
			// Send truncation warning
			warningData, _ := json.Marshal(map[string]string{
				"output": "\n[OUTPUT TRUNCATED - Limit of 10MB reached]\n",
			})
			fmt.Fprintf(w, "data: %s\n\n", warningData)
			w.(http.Flusher).Flush()
		}
	}
}

// handleKillInstances handles POST requests to kill processes
func handleKillInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		PIDs []int `json:"pids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	type KillError struct {
		PID   int    `json:"pid"`
		Error string `json:"error"`
	}

	var errors []KillError
	var killed []int

	// Create process validator for comprehensive PID validation
	processValidator := lib.NewProcessValidator()

	for _, pid := range request.PIDs {
		// Perform comprehensive PID validation including ownership checks
		if err := processValidator.ValidatePIDWithOwnership(pid); err != nil {
			errors = append(errors, KillError{
				PID:   pid,
				Error: err.Error(),
			})
			continue
		}

		// PID validation passed, proceed with killing the process
		cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
		output, err := cmd.CombinedOutput()

		if err != nil {
			outputStr := strings.ToLower(string(output))
			errorStr := strings.ToLower(err.Error())

			// Check if the process doesn't exist (count as success)
			if strings.Contains(outputStr, "no such process") ||
				strings.Contains(errorStr, "no such process") ||
				strings.Contains(outputStr, "no process found") ||
				strings.Contains(errorStr, "no process found") {
				// Process already gone - count as successful
				killed = append(killed, pid)
				continue
			}

			// Check for permission errors
			if strings.Contains(outputStr, "operation not permitted") ||
				strings.Contains(errorStr, "operation not permitted") ||
				strings.Contains(outputStr, "permission denied") ||
				strings.Contains(errorStr, "permission denied") {
				errors = append(errors, KillError{
					PID:   pid,
					Error: "Insufficient privileges to kill process",
				})
			} else {
				// For other errors, try to provide more context
				errorMessage := err.Error()
				if len(output) > 0 {
					errorMessage = fmt.Sprintf("%s: %s", errorMessage, strings.TrimSpace(string(output)))
				}

				// Check if this might be a "process not found" case with exit status 1
				if strings.Contains(errorMessage, "exit status 1") {
					// Double-check by trying to see if process exists with ps
					checkCmd := exec.Command("ps", "-p", fmt.Sprintf("%d", pid))
					if checkErr := checkCmd.Run(); checkErr != nil {
						// ps failed to find the process, so it doesn't exist
						killed = append(killed, pid)
						continue
					}
				}

				errors = append(errors, KillError{
					PID:   pid,
					Error: errorMessage,
				})
			}
		} else {
			// Successfully killed
			killed = append(killed, pid)
		}
	}

	// Always set content type and return JSON response
	w.Header().Set("Content-Type", "application/json")

	// Invalidate caches if any processes were killed (even partial success)
	if len(killed) > 0 {
		invalidateCrontabCache()
		// Also invalidate process cache since the process list has changed
		invalidateProcessCache()
	}

	if len(errors) > 0 {
		w.WriteHeader(http.StatusPartialContent) // 206 for partial success
		json.NewEncoder(w).Encode(map[string]interface{}{
			"killed": killed,
			"errors": errors,
		})
		return
	}

	// Complete success
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"killed":  killed,
		"message": fmt.Sprintf("Successfully killed %d process(es)", len(killed)),
	})
}

// handleDeleteJob handles DELETE requests to delete a job
func handleDeleteJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "DELETE" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	crontab, err := lib.GetCrontab(job.CrontabFilename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var foundLine *lib.Line
	var foundLineIndex int

	// Find the matching line
	for i, line := range crontab.Lines {
		if (job.Code != "" && line.Code == job.Code) || (job.Key != "" && line.Key(crontab.CanonicalName()) == job.Key) {
			foundLine = line
			foundLineIndex = i
			break
		}
	}

	if foundLine == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// If the job is monitored, pause it indefinitely
	if job.Monitored {
		if err := getCronitorApi().PauseMonitor(job.Code, ""); err != nil {
			http.Error(w, fmt.Sprintf("Failed to pause monitor: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Remove the line from the crontab
	crontab.Lines = append(crontab.Lines[:foundLineIndex], crontab.Lines[foundLineIndex+1:]...)

	// Save the crontab
	if err := crontab.Save(crontab.Write()); err != nil {
		http.Error(w, "Failed to save crontab", http.StatusInternalServerError)
		return
	}

	// Invalidate cache since we modified a crontab
	invalidateCrontabCache()

	w.WriteHeader(http.StatusOK)
}

// handlePostJob handles POST requests to create a new job
func handlePostJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if isSafeModeEnabled {
		http.Error(w, "Job creation is disabled in safe mode", http.StatusForbidden)
		return
	}

	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if job.Name == "" || job.Expression == "" || job.Command == "" || job.CrontabFilename == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(job.CrontabFilename, "user:") && job.RunAsUser == "" {
		http.Error(w, "Missing required field: run_as_user", http.StatusBadRequest)
		return
	}

	// If cron.d is selected, create a new file
	if job.CrontabFilename == "/etc/cron.d" {
		// Slugify the job name for the filename
		filename := slugify(job.Name) + ".cron"
		job.CrontabFilename = filepath.Join("/etc/cron.d", filename)
	}

	// Get the crontab
	crontab, err := lib.GetCrontab(job.CrontabFilename)
	if err != nil {
		// If the file doesn't exist, create it (for both /etc/crontab and files in /etc/cron.d)
		if os.IsNotExist(err) && (job.CrontabFilename == "/etc/crontab" || strings.HasPrefix(job.CrontabFilename, "/etc/cron.d")) {
			// Create an empty file
			if err := os.WriteFile(job.CrontabFilename, []byte{}, 0644); err != nil {
				http.Error(w, fmt.Sprintf("Failed to create crontab file: %v", err), http.StatusInternalServerError)
				return
			}
			crontab, err = lib.GetCrontab(job.CrontabFilename)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Add the line to the crontab
	line := &lib.Line{
		IsJob:          true,
		Name:           job.Name,
		CronExpression: job.Expression,
		CommandToRun:   job.Command,
		RunAs:          job.RunAsUser,
		Crontab:        *crontab,
	}

	crontab.Lines = append(crontab.Lines, line)

	// Save the crontab
	if err := crontab.Save(crontab.Write()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate cache since we modified a crontab
	invalidateCrontabCache()

	// If monitoring is enabled, create a new monitor
	if job.Monitored {
		monitor := &lib.Monitor{
			Name:        job.Name,
			DefaultName: createDefaultName(line, crontab, "", []string{}, map[string]bool{}),
			Schedule:    job.Expression,
			Type:        "job",
			Platform:    lib.CRON,
			Timezone:    job.Timezone,
			Key:         line.Key(crontab.CanonicalName()),
		}

		monitors := map[string]*lib.Monitor{
			monitor.Key: monitor,
		}

		updatedMonitors, err := getCronitorApi().PutMonitors(monitors)
		if err != nil {
			// Log the error but don't fail the request
			log(fmt.Sprintf("Failed to create monitor: %v", err))
		} else if updatedMonitor, exists := updatedMonitors[monitor.Key]; exists {
			line.Code = updatedMonitor.Attributes.Code
			line.Mon = *updatedMonitor
			// Save the crontab again to update the code
			if err := crontab.Save(crontab.Write()); err != nil {
				log(fmt.Sprintf("Failed to save crontab with monitor code: %v", err))
			}
		}
	}

	w.WriteHeader(http.StatusCreated)
}

// handleGetCrontabs handles GET requests for crontabs
func handleGetCrontabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read all crontabs
	var crontabs []*lib.Crontab
	crontabs, err := lib.GetAllCrontabs(parseUsers())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse each crontab to ensure lines are loaded
	for _, crontab := range crontabs {
		if len(crontab.Lines) == 0 && crontab.Exists() {
			crontab.Parse(true)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(crontabs)
}

// handlePostCrontabs handles POST requests to create a new crontab
func handlePostCrontabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if isSafeModeEnabled {
		http.Error(w, "Crontab creation is disabled in safe mode", http.StatusForbidden)
		return
	}

	// Define a custom struct to capture all fields including comments
	type CrontabRequest struct {
		Filename             string                    `json:"filename"`
		TimezoneLocationName *lib.TimezoneLocationName `json:"TimezoneLocationName"`
		Comments             string                    `json:"comments"`
	}

	var request CrontabRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Filename == "" {
		http.Error(w, "Filename is required", http.StatusBadRequest)
		return
	}

	// If creating in /etc/cron.d, build the full path
	if !strings.Contains(request.Filename, "/") && request.Filename != "/etc/crontab" && !strings.HasPrefix(request.Filename, "user:") {
		request.Filename = filepath.Join("/etc/cron.d", request.Filename)
	}

	// Try to load the crontab first to check if it exists
	existingCrontab, err := lib.GetCrontab(request.Filename)
	if err == nil {
		// Parse it to ensure lines are loaded
		if len(existingCrontab.Lines) == 0 && existingCrontab.Exists() {
			existingCrontab.Parse(true)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(existingCrontab)
		return
	}

	// Create a new crontab
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	newCrontab := lib.CrontabFactory(username, request.Filename)

	// Build content with timezone and comments
	content := ""

	// Add timezone if provided
	if request.TimezoneLocationName != nil && request.TimezoneLocationName.Name != "" {
		content = fmt.Sprintf("CRON_TZ=%s\nTZ=%s\n", request.TimezoneLocationName.Name, request.TimezoneLocationName.Name)
	}

	// Add comments if provided
	if request.Comments != "" {
		// Add each comment line with proper formatting
		commentLines := strings.Split(request.Comments, "\n")
		for _, line := range commentLines {
			line = strings.TrimSpace(line)
			if line != "" {
				// Always add # prefix for comments
				if !strings.HasPrefix(line, "#") {
					content += "# " + line + "\n"
				} else {
					content += line + "\n"
				}
			}
		}
	}

	if err := newCrontab.Save(content); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create crontab file: %v", err), http.StatusInternalServerError)
		return
	}

	// Parse the new crontab to populate lines
	newCrontab.Parse(true)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newCrontab)
}

// handleUsers handles GET requests for system users
func handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var users []string

	// On Unix-like systems, we can use the 'id' command to get a list of users
	if runtime.GOOS != "windows" {
		cmd := exec.Command("id", "-u", "-n")
		output, err := cmd.Output()
		if err == nil {
			// Add the current user
			currentUser := strings.TrimSpace(string(output))
			users = append(users, currentUser)
		}

		// Try to get additional users from /etc/passwd if available
		if passwdFile, err := os.Open("/etc/passwd"); err == nil {
			defer passwdFile.Close()
			scanner := bufio.NewScanner(passwdFile)
			for scanner.Scan() {
				line := scanner.Text()
				fields := strings.Split(line, ":")
				if len(fields) > 2 {
					username := fields[0]
					// Include root and regular users (UID >= 1000)
					if uid, err := strconv.Atoi(fields[2]); err == nil && (uid == 0 || uid >= 1000) && username != "nobody" {
						// Check if user already exists in the list
						found := false
						for _, u := range users {
							if u == username {
								found = true
								break
							}
						}
						if !found {
							users = append(users, username)
						}
					}
				}
			}
		}
	} else {
		// On Windows, just return the current user
		if currentUser, err := user.Current(); err == nil {
			users = append(users, currentUser.Username)
		}
	}

	// Sort the users alphabetically
	sort.Strings(users)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// handleCrontabs handles requests for crontabs
func handleCrontabs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		handleGetCrontabs(w, r)
	case "POST":
		handlePostCrontabs(w, r)
	case "PUT":
		handlePutCrontabs(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePutCrontabs handles PUT requests to update a crontab
func handlePutCrontabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "PUT" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if isSafeModeEnabled {
		http.Error(w, "Crontab editing is disabled in safe mode", http.StatusForbidden)
		return
	}

	// Get the crontab filename from the URL path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid crontab path", http.StatusBadRequest)
		return
	}
	// Join all parts after "/api/crontabs/" to handle nested directory paths
	filename := strings.Join(parts[3:], "/")
	// Add leading slash for file paths, but not for user crontabs (user:username)
	if !strings.HasPrefix(filename, "user:") {
		filename = "/" + filename
	}

	// Parse the request body
	var request struct {
		Lines []struct {
			LineText string `json:"line_text"`
			Name     string `json:"name,omitempty"`
		} `json:"lines"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get the crontab
	crontab, err := lib.GetCrontab(filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert the lines to the format expected by the crontab
	var newLines []*lib.Line
	for _, line := range request.Lines {
		newLine := &lib.Line{
			FullLine: line.LineText,
			Name:     line.Name,
			Crontab:  *crontab,
		}

		// Set line types and parse content based on line type
		if strings.HasPrefix(line.LineText, "#") {
			newLine.IsComment = true
		} else if strings.Contains(line.LineText, "=") {
			// Environment variable - just use FullLine
		} else if line.LineText != "" {
			// This is a job line - we need to parse it properly
			newLine.IsJob = true

			// Parse the job line to extract cron expression and command
			splitLine := strings.Fields(line.LineText)
			if len(splitLine) >= 6 {
				// Check if it's a 6-field expression
				var cronExpression string
				var command []string

				// Handle special @keywords
				if len(splitLine) > 0 && strings.HasPrefix(splitLine[0], "@") {
					cronExpression = splitLine[0]
					command = splitLine[1:]
				} else {
					// Standard 5 or 6 field cron expression
					cronExpression = strings.Join(splitLine[0:5], " ")
					command = splitLine[5:]

					// Handle run-as user for system crontabs
					if !crontab.IsUserCrontab && len(command) > 0 {
						// First word after cron expression might be the user
						if runtime.GOOS != "windows" {
							if _, err := exec.Command("id", "-u", command[0]).CombinedOutput(); err == nil {
								newLine.RunAs = command[0]
								command = command[1:]
							}
						}
					}
				}

				newLine.CronExpression = cronExpression
				newLine.CommandToRun = strings.Join(command, " ")
			}
		}

		newLines = append(newLines, newLine)
	}

	// Update the crontab's lines
	crontab.Lines = newLines

	// Save the crontab
	if err := crontab.Save(crontab.Write()); err != nil {
		http.Error(w, "Failed to save crontab", http.StatusInternalServerError)
		return
	}

	// Invalidate cache since we modified a crontab
	invalidateCrontabCache()

	// Parse the updated crontab to ensure lines are loaded
	if len(crontab.Lines) == 0 && crontab.Exists() {
		crontab.Parse(true)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(crontab)
}

// handleCrontab handles requests for individual crontabs
func handleCrontab(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "PUT":
		handlePutCrontabs(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Helper function to check if a slice contains a string
func contains(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func handlePutJob(w http.ResponseWriter, r *http.Request) {
	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if job.CrontabFilename == "" {
		http.Error(w, "Crontab filename is required", http.StatusBadRequest)
		return
	}

	crontab, err := lib.GetCrontab(job.CrontabFilename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var foundLine *lib.Line
	var foundLineIndex int

	// Find the matching line
	for i, line := range crontab.Lines {
		if (job.Code != "" && line.Code == job.Code) || (job.Key != "" && line.Key(crontab.CanonicalName()) == job.Key) {
			foundLine = line
			foundLineIndex = i
			break
		}
	}

	if foundLine == nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	// Update the line
	hasChanges := false

	// Handle suspended status
	if job.Suspended != nil && *job.Suspended != foundLine.IsComment {
		foundLine.IsComment = *job.Suspended
		hasChanges = true

		// If the job is monitored, handle pause/unpause
		if job.Monitored {
			if *job.Suspended {
				// Pause the monitor when suspending
				if err := getCronitorApi().PauseMonitor(job.Code, job.PauseHours); err != nil {
					http.Error(w, fmt.Sprintf("Failed to pause monitor: %v", err), http.StatusInternalServerError)
					return
				}
			} else {
				// Unpause the monitor when unsuspending
				if err := getCronitorApi().PauseMonitor(job.Code, "0"); err != nil {
					http.Error(w, fmt.Sprintf("Failed to unpause monitor: %v", err), http.StatusInternalServerError)
					return
				}
			}
		}
	}

	// Collect all monitor updates
	var monitor *lib.Monitor
	if job.Monitored {
		monitor = &lib.Monitor{
			Name:        job.Name,
			DefaultName: createDefaultName(foundLine, crontab, "", []string{}, map[string]bool{}),
			Schedule:    job.Expression,
			Type:        "job",
			Platform:    lib.CRON,
			Timezone:    job.Timezone,
			Key:         foundLine.Code,
		}

		// If we're enabling monitoring for the first time, we won't have a code yet, use the key instead
		// Ensure monitor is unpaused -- important if they have previously disabled monitoring and then re-enabled it
		if foundLine.Code == "" {
			monitor.Key = foundLine.Key(crontab.CanonicalName())
			paused := false
			monitor.Paused = &paused
		}

	} else if foundLine.Code != "" {
		if err := getCronitorApi().PauseMonitor(foundLine.Code, ""); err != nil {
			http.Error(w, fmt.Sprintf("Failed to pause monitor: %v", err), http.StatusInternalServerError)
			return
		}

		crontab.Lines[foundLineIndex].Code = ""
		hasChanges = true
	}

	// Handle name update
	if job.Name != foundLine.Name {
		crontab.Lines[foundLineIndex].Name = job.Name
		hasChanges = true
	}

	// Handle command update
	if job.Command != "" && job.Command != foundLine.CommandToRun {
		if isSafeModeEnabled {
			http.Error(w, "Command editing is disabled in safe mode", http.StatusForbidden)
			return
		}

		// Get the old key before updating the command
		oldKey := foundLine.Key(crontab.CanonicalName())
		oldCommand := foundLine.CommandToRun // Store the old command

		// Update the command
		crontab.Lines[foundLineIndex].CommandToRun = job.Command
		hasChanges = true

		// Get the new key after updating the command
		newKey := foundLine.Key(crontab.CanonicalName())

		// Move history to new key
		commandHistory.MoveHistory(oldKey, newKey, oldCommand)
	}

	// Handle schedule update
	if job.Expression != "" && foundLine.CronExpression != job.Expression {
		crontab.Lines[foundLineIndex].CronExpression = job.Expression
		hasChanges = true
	}

	// Handle timezone update
	if job.Timezone != "" && crontab.TimezoneLocationName != nil && crontab.TimezoneLocationName.Name != job.Timezone {
		crontab.TimezoneLocationName = &lib.TimezoneLocationName{Name: job.Timezone}
		hasChanges = true
	}

	// Handle ignored status update
	if job.Ignored != foundLine.Ignored {
		crontab.Lines[foundLineIndex].Ignored = job.Ignored
		hasChanges = true
	}

	// If monitor exists or needs to be created, update it with all changes
	if monitor != nil {
		monitors := map[string]*lib.Monitor{
			monitor.Key: monitor,
		}

		updatedMonitors, err := getCronitorApi().PutMonitors(monitors)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if updatedMonitor, exists := updatedMonitors[monitor.Key]; exists {
			crontab.Lines[foundLineIndex].Mon = *updatedMonitor
			// Only update the code if we get one back (for new monitors)
			// For existing monitors, preserve the existing code
			if updatedMonitor.Attributes.Code != "" {
				crontab.Lines[foundLineIndex].Code = updatedMonitor.Attributes.Code
				hasChanges = true
			}
		}
	}

	// Save changes if any
	if hasChanges {
		if err := crontab.Save(crontab.Write()); err != nil {
			http.Error(w, fmt.Sprintf("Failed to save crontab: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Invalidate cache since we modified a crontab
	invalidateCrontabCache()

	// Update the job with the latest values
	job.Name = crontab.Lines[foundLineIndex].Name
	job.Code = crontab.Lines[foundLineIndex].Code

	// If a monitor was created/updated in this request, use the requested monitored status
	// Otherwise, determine monitored status based on whether the job has a code
	if monitor != nil {
		// Monitor was created or updated, so use the requested monitored status
		job.Monitored = true
	} else {
		// No monitor operation was performed, determine status based on code presence
		job.Monitored = len(crontab.Lines[foundLineIndex].Code) > 0
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

// Helper function to invalidate crontab cache
func invalidateCrontabCache() {
	// No-op: Crontab file cache has been removed
}

// Helper function to invalidate process cache
func invalidateProcessCache() {
	psCache.mutex.Lock()
	psCache.lines = nil
	psCache.timestamp = time.Time{}
	psCache.mutex.Unlock()
}

// handleUpdateCheck handles GET requests to check for updates
func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status, err := checkForUpdate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleUpdatePerform handles POST requests to perform an update
func handleUpdatePerform(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if isSafeModeEnabled {
		http.Error(w, "Updates are disabled in safe mode", http.StatusForbidden)
		return
	}

	// Check for update first
	status, err := checkForUpdate()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !status.HasUpdate {
		http.Error(w, "No update available", http.StatusBadRequest)
		return
	}

	// Set up SSE headers for streaming progress
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Function to send progress updates
	sendProgress := func(message string) {
		defer func() {
			if r := recover(); r != nil {
				// Connection was likely closed during update, silently ignore
				log(fmt.Sprintf("Progress update failed (connection closed): %v", r))
			}
		}()

		// Check if w is nil
		if w == nil {
			return
		}

		progressData, _ := json.Marshal(map[string]string{"progress": message})
		if _, err := fmt.Fprintf(w, "data: %s\n\n", progressData); err != nil {
			return
		}

		// Simple flush without nested defer
		if flusher, ok := w.(http.Flusher); ok && flusher != nil {
			flusher.Flush() // This will be caught by the main defer/recover if it panics
		}
	}

	sendProgress("Starting update process...")

	// Perform the update in a goroutine
	go func() {
		if err := performUpdateWithRestart(status, sendProgress); err != nil {
			defer func() {
				if r := recover(); r != nil {
					// Connection was likely closed during update, ignore the panic
					log(fmt.Sprintf("Error response failed (connection closed): %v", r))
				}
			}()

			// Check if w is nil before writing error
			if w == nil {
				return
			}

			errorData, _ := json.Marshal(map[string]string{"error": err.Error()})
			if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", errorData); writeErr != nil {
				return // Write failed, connection likely closed
			}

			// Simple flush without nested defer
			if flusher, ok := w.(http.Flusher); ok && flusher != nil {
				flusher.Flush() // This will be caught by the main defer/recover if it panics
			}
		}
	}()

	// Keep the connection alive briefly to ensure messages are sent
	time.Sleep(1 * time.Second)
}

// performUpdateWithRestart performs the update and handles the restart
func performUpdateWithRestart(status *UpdateStatus, sendProgress func(string)) error {
	sendProgress("Downloading new version...")

	// Perform the update (download and replace executable)
	if err := performUpdate(status); err != nil {
		return fmt.Errorf("failed to download update: %v", err)
	}

	sendProgress("Update downloaded successfully. Restarting server...")

	// Give the UI a moment to receive the message
	time.Sleep(1 * time.Second)

	// Check if we're running under systemd
	if isRunningUnderSystemd() {
		sendProgress("Detected systemd service, requesting service restart...")

		// When running under systemd, we should let systemd handle the restart
		// Signal systemd to restart the service
		if err := requestSystemdRestart(); err != nil {
			// If systemd restart fails, fall back to manual restart
			log(fmt.Sprintf("Failed to request systemd restart, falling back to manual restart: %v", err))
			return performManualRestart(sendProgress)
		}

		sendProgress("Update complete! Service restarting via systemd...")

		// Give a brief moment for the response to be sent, then exit gracefully
		go func() {
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		}()

		return nil
	}

	// For non-systemd environments, use manual restart
	return performManualRestart(sendProgress)
}

// isRunningUnderSystemd checks if the current process is running under systemd
func isRunningUnderSystemd() bool {
	// Check if NOTIFY_SOCKET environment variable is set (systemd sets this)
	if os.Getenv("NOTIFY_SOCKET") != "" {
		return true
	}

	// Check if we're running as PID 1's direct child and systemd is PID 1
	ppid := os.Getppid()
	if ppid == 1 {
		// Check if PID 1 is systemd
		if cmdline, err := os.ReadFile("/proc/1/cmdline"); err == nil {
			return strings.Contains(string(cmdline), "systemd")
		}
	}

	// Check if systemd journal is available
	if _, err := os.Stat("/run/systemd/journal"); err == nil {
		// Also check if we have systemd process manager
		if _, err := os.Stat("/run/systemd/system"); err == nil {
			return true
		}
	}

	return false
}

// requestSystemdRestart attempts to restart the service via systemd
func requestSystemdRestart() error {
	// Instead of trying to run systemctl (which requires sudo), we'll just exit gracefully
	// and let systemd restart us automatically (assuming Restart=always or Restart=on-failure)
	//
	// NOTE: Your systemd service file should have one of these restart policies:
	//   Restart=always       - Always restart the service when it exits
	//   Restart=on-failure   - Restart only on non-zero exit codes
	//   Restart=on-success   - Restart only on zero exit codes
	//
	// Example systemd service file:
	//   [Service]
	//   Type=simple
	//   Restart=always
	//   RestartSec=5
	//   ExecStart=/usr/local/bin/cronitor dash

	// Try to determine the service name for logging purposes
	serviceName := getSystemdServiceName()
	if serviceName != "" {
		log(fmt.Sprintf("Exiting to trigger systemd restart of service: %s", serviceName))
	} else {
		log("Exiting to trigger systemd restart")
	}

	// Exit with success code to trigger systemd restart
	// Most systemd services are configured with Restart=always or Restart=on-failure
	return nil
}

// getSystemdServiceName attempts to determine the systemd service name
func getSystemdServiceName() string {
	// Try to get service name from systemd environment
	if serviceName := os.Getenv("SYSTEMD_SERVICE"); serviceName != "" {
		return serviceName
	}

	// Try common service names based on executable name
	executable, err := os.Executable()
	if err != nil {
		return ""
	}

	baseName := strings.TrimSuffix(filepath.Base(executable), filepath.Ext(executable))

	// Common patterns for systemd service names
	possibleNames := []string{
		baseName + ".service",
		baseName + "-dashboard.service",
		"crontab-guru-dashboard.service", // User mentioned this specific name
		"cronitor-cli.service",
		"cronitor.service",
	}

	// Check which service exists and is active
	for _, name := range possibleNames {
		cmd := exec.Command("systemctl", "is-active", "--quiet", name)
		if err := cmd.Run(); err == nil {
			return name
		}
	}

	return ""
}

// performManualRestart performs a manual restart (non-systemd)
func performManualRestart(sendProgress func(string)) error {
	// Get current executable path and arguments
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %v", err)
	}

	sendProgress("Starting new server instance...")

	// Start new process with same arguments
	cmd := exec.Command(executable, os.Args[1:]...)
	cmd.Env = os.Environ()

	// Start the new process but don't wait for it
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start new process: %v", err)
	}

	sendProgress("Update complete! Server restarting...")

	// Give a brief moment for the response to be sent, then exit
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()

	return nil
}

// RateLimiter manages rate limiting per IP address
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mutex    sync.RWMutex
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
	}

	// Start cleanup goroutine to remove stale entries every 10 minutes
	go rl.cleanupRoutine()

	return rl
}

// GetLimiter returns a rate limiter for the given IP address
func (rl *RateLimiter) GetLimiter(ip string) *rate.Limiter {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	limiter, exists := rl.limiters[ip]
	if !exists {
		// 5 failed attempts per minute: rate.Every(12*time.Second) = 1 request every 12 seconds = 5 per minute
		limiter = rate.NewLimiter(rate.Every(12*time.Second), 5)
		rl.limiters[ip] = limiter
	}

	return limiter
}

// cleanupRoutine removes stale entries from the rate limiter map every 10 minutes
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		}
	}
}

// cleanup removes entries that haven't been used recently
func (rl *RateLimiter) cleanup() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	// Remove entries that have full tokens (indicating no recent activity)
	for ip, limiter := range rl.limiters {
		// If the limiter has all its tokens, it hasn't been used recently
		if limiter.Tokens() == float64(limiter.Burst()) {
			delete(rl.limiters, ip)
		}
	}
}

// getClientIP extracts the client IP from the request, handling proxy headers
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for reverse proxies)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// Take the first IP in the chain
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		ip := strings.TrimSpace(xri)
		if net.ParseIP(ip) != nil {
			return ip
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Global rate limiter instance
var authRateLimiter = NewRateLimiter()
