package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ConfigFile struct {
	ApiKey             string                       `json:"CRONITOR_API_KEY"`
	PingApiAuthKey     string                       `json:"CRONITOR_PING_API_KEY"`
	ExcludeText        []string                     `json:"CRONITOR_EXCLUDE_TEXT,omitempty"`
	Hostname           string                       `json:"CRONITOR_HOSTNAME"`
	Log                string                       `json:"CRONITOR_LOG"`
	Env                string                       `json:"CRONITOR_ENV"`
	DashUsername       string                       `json:"CRONITOR_DASH_USER"`
	DashPassword       string                       `json:"CRONITOR_DASH_PASS"`
	AllowedIPs         string                       `json:"CRONITOR_ALLOWED_IPS"`
	CorsAllowedOrigins string                       `json:"CRONITOR_CORS_ALLOWED_ORIGINS"`
	Users              string                       `json:"CRONITOR_USERS"`
	ApiVersion         string                       `json:"CRONITOR_API_VERSION,omitempty"`
	MCPEnabled         bool                         `json:"CRONITOR_MCP_ENABLED,omitempty"`
	MCPInstances       map[string]MCPInstanceConfig `json:"mcp_instances,omitempty"`
}

type MCPInstanceConfig struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// configureCmd represents the configure command
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Save configuration variables to the config file",
	Long: `
Optionally write configuration options to a JSON file.

By default, configuration files are system-wide for ease of use in cron jobs and scripts. Default configuration file location varies by platform:
  Linux        /etc/cronitor/cronitor.json
  MacOS        /etc/cronitor/cronitor.json
  Windows      %SystemDrive%\ProgramData\Cronitor\cronitor.json

Credential-bearing config files are written with mode 0600 (owner-only). Existing files remain readable at their current mode (including 0644) until someone saves. The next configure/signup/dash save rewrites the file as 0600 and prints a warning that other users will lose read access.

Preferred: inject CRONITOR_API_KEY and CRONITOR_PING_API_KEY in the crontab or service environment instead of storing keys in the JSON file.
If you need a shared file after a save, chmod 0640 and chgrp a dedicated group yourself (the next Cronitor save will set 0600 again).
Per-user: --config or CRONITOR_CONFIG pointing at a user-owned 0600 file.

CronitorCLI configuration can be supplied from a file, environment variables, or command line flags.
You can use a default config file for some things and environment variables or command line arguments for others -- the goal is flexibility.

WARNING: --api-key, --ping-api-key, and --dash-password appear in shell history and process lists. Prefer environment variables set outside the transcript (export CRONITOR_API_KEY=... in your profile, crontab, or service unit).

Environment variables that are read:
  CRONITOR_API_KEY
  CRONITOR_PING_API_KEY
  CRONITOR_CONFIG
  CRONITOR_EXCLUDE_TEXT
  CRONITOR_HOSTNAME
  CRONITOR_LOG
  CRONITOR_ENV
  CRONITOR_DASH_USER
  CRONITOR_DASH_PASS
  CRONITOR_ALLOWED_IPS
  CRONITOR_USERS
  CRONITOR_API_VERSION
  CRONITOR_CORS_ALLOWED_ORIGINS
  CRONITOR_MCP_ENABLED

Example setting your API key (preferred):
  $ export CRONITOR_API_KEY
  $ cronitor configure

Example setting common exclude text for use with 'cronitor discover':
  $ cronitor configure -e "/var/app/code/path/" -e "/var/app/bin/" -e "> /dev/null"`,
	Run: func(cmd *cobra.Command, args []string) {

		configData := configFromViper()

		fmt.Println("\nConfiguration File:")
		fmt.Println(configFilePath())

		fmt.Println("\nVersion:")
		fmt.Println(Version)

		fmt.Println("\nAPI Key:")
		fmt.Println(secretPresenceLabel(configData.ApiKey))

		fmt.Println("\nPing API Key:")
		fmt.Println(secretPresenceLabel(configData.PingApiAuthKey))

		fmt.Println("\nEnvironment:")
		if configData.Env == "" {
			fmt.Println("Not Set")
		} else {
			fmt.Println(configData.Env)
		}

		fmt.Println("\nHostname:")
		fmt.Println(effectiveHostname())

		fmt.Println("\nTimezone Location:")
		fmt.Println(effectiveTimezoneLocationName())

		fmt.Println("\nDebug Log:")
		if viper.GetString(varLog) == "" {
			fmt.Println("Off")
		} else {
			fmt.Println(viper.GetString(varLog))
		}

		fmt.Println("\nDashboard Username:")
		if configData.DashUsername == "" {
			fmt.Println("Not Set")
		} else {
			fmt.Println(configData.DashUsername)
		}

		fmt.Println("\nDashboard Password:")
		if configData.DashPassword == "" {
			fmt.Println("Not Set")
		} else {
			fmt.Println("********")
		}

		fmt.Println("\nAllowed IP Addresses:")
		if configData.AllowedIPs == "" {
			fmt.Println("All IPs allowed (no restrictions)")
		} else {
			fmt.Println(configData.AllowedIPs)
		}

		fmt.Println("\nUsers:")
		if configData.Users == "" {
			fmt.Println("Current user only")
		} else {
			fmt.Println(configData.Users)
		}

		fmt.Println("\nAPI Version:")
		if configData.ApiVersion == "" {
			fmt.Println("Not Set (API default)")
		} else {
			fmt.Println(configData.ApiVersion)
		}

		fmt.Println("\nMCP Enabled:")
		fmt.Println(configData.MCPEnabled)

		if len(configData.MCPInstances) > 0 {
			fmt.Println("\nMCP Instances:")
			for name, instance := range configData.MCPInstances {
				// Print name and URL only; never print MCP instance passwords.
				fmt.Printf("  %s: %s\n", name, instance.URL)
			}
		}

		if verbose {
			printCronitorEnvSources()
		}

		b, err := json.MarshalIndent(configData, "", "    ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		configPath := configFilePath()
		if err := persistConfigFile(configPath, b); err != nil {
			fmt.Fprintf(os.Stderr, "\nERROR: %v\n\n", err)
			os.Exit(126)
		}
	},
}

func init() {
	RootCmd.AddCommand(configureCmd)
	configureCmd.Flags().StringSliceP("exclude-from-name", "e", []string{}, "Substring to always exclude from generated monitor name e.g. $ cronitor configure -e '> /dev/null' -e '/path/to/app'")
	configureCmd.Flags().String("dash-username", "", "Username for the dashboard authentication")
	configureCmd.Flags().String("dash-password", "", "Password for the dashboard authentication (appears in shell history and process lists; prefer CRONITOR_DASH_PASS)")
	configureCmd.Flags().String("allowed-ips", "", "Comma-separated list of allowed IP addresses/CIDR ranges (e.g. 192.168.1.0/24,10.0.0.1)")
	configureCmd.Flags().String("ping-api-key", "", "Your Cronitor Ping API key (appears in shell history and process lists; prefer CRONITOR_PING_API_KEY)")
	configureCmd.Flags().String("log", "", "Path to debug log file")
	configureCmd.Flags().String("env", "", "Environment name (e.g. staging, production)")
	configureCmd.Flags().String("users", "", "Comma-separated list of users whose crontabs to include")
	configureCmd.Flags().Bool(varMCPEnabled, false, "Enable MCP instances")

	viper.BindPFlag(varExcludeText, configureCmd.Flags().Lookup("exclude-from-name"))
	viper.BindPFlag(varDashUsername, configureCmd.Flags().Lookup("dash-username"))
	viper.BindPFlag(varDashPassword, configureCmd.Flags().Lookup("dash-password"))
	viper.BindPFlag(varAllowedIPs, configureCmd.Flags().Lookup("allowed-ips"))
	viper.BindPFlag(varPingApiKey, configureCmd.Flags().Lookup("ping-api-key"))
	viper.BindPFlag(varLog, configureCmd.Flags().Lookup("log"))
	viper.BindPFlag(varEnv, configureCmd.Flags().Lookup("env"))
	viper.BindPFlag(varUsers, configureCmd.Flags().Lookup("users"))
	viper.BindPFlag(varMCPEnabled, configureCmd.Flags().Lookup(varMCPEnabled))
}
