package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

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

CronitorCLI configuration can be supplied from a file, environment variables, or command line flags.
You can use a default config file for some things and environment variables or command line arguments for others -- the goal is flexibility.

Environment variables that are read:
  CRONITOR_API_KEY
  CRONITOR_CONFIG
  CRONITOR_EXCLUDE_TEXT
  CRONITOR_HOSTNAME
  CRONITOR_LOG
  CRONITOR_PING_API_KEY
  CRONITOR_USERS

Example setting your API Key:
  $ cronitor configure --api-key 4319e94e890a013dbaca57c2df2ff60c2

Example setting common exclude text for use with 'cronitor discover':
  $ cronitor configure -e "/var/app/code/path/" -e "/var/app/bin/" -e "> /dev/null"`,
	Run: func(cmd *cobra.Command, args []string) {

		configData := ConfigFile{}
		configData.ApiKey = viper.GetString(varApiKey)
		configData.PingApiAuthKey = viper.GetString(varPingApiKey)
		configData.ExcludeText = viper.GetStringSlice(varExcludeText)
		configData.Hostname = viper.GetString(varHostname)
		configData.Log = viper.GetString(varLog)
		configData.Env = viper.GetString(varEnv)
		configData.DashUsername = viper.GetString(varDashUsername)
		configData.DashPassword = viper.GetString(varDashPassword)
		configData.AllowedIPs = viper.GetString(varAllowedIPs)
		configData.CorsAllowedOrigins = viper.GetString("CRONITOR_CORS_ALLOWED_ORIGINS")
		configData.Users = viper.GetString(varUsers)
		configData.MCPEnabled = viper.GetBool(varMCPEnabled)

		// Load MCP instances if configured
		if viper.IsSet("mcp_instances") {
			// Get the raw config and manually convert
			rawInstances := viper.GetStringMap("mcp_instances")
			configData.MCPInstances = make(map[string]MCPInstanceConfig)

			for name, rawConfig := range rawInstances {
				if configMap, ok := rawConfig.(map[string]interface{}); ok {
					instance := MCPInstanceConfig{}
					if url, ok := configMap["url"].(string); ok {
						instance.URL = url
					}
					if username, ok := configMap["username"].(string); ok {
						instance.Username = username
					}
					if password, ok := configMap["password"].(string); ok {
						instance.Password = password
					}
					configData.MCPInstances[name] = instance
				}
			}
		}

		fmt.Println("\nConfiguration File:")
		fmt.Println(configFilePath())

		fmt.Println("\nVersion:")
		fmt.Println(Version)

		fmt.Println("\nAPI Key:")
		if configData.ApiKey == "" {
			fmt.Println("Not Set")
		} else {
			fmt.Println(configData.ApiKey)
		}

		fmt.Println("\nPing API Key:")
		if configData.PingApiAuthKey == "" {
			fmt.Println("Not Set")
		} else {
			fmt.Println(configData.PingApiAuthKey)
		}

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

		fmt.Println("\nLocalDash Username:")
		if configData.DashUsername == "" {
			fmt.Println("Not Set")
		} else {
			fmt.Println(configData.DashUsername)
		}

		fmt.Println("\nLocalDash Password:")
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

		fmt.Println("\nMCP Enabled:")
		fmt.Println(configData.MCPEnabled)

		if len(configData.MCPInstances) > 0 {
			fmt.Println("\nMCP Instances:")
			for name, instance := range configData.MCPInstances {
				fmt.Printf("  %s: %s\n", name, instance.URL)
			}
		}

		if verbose {
			fmt.Println("\nEnviornment Variables:")
			for _, pair := range os.Environ() {
				fmt.Println(pair)
			}
		}

		b, err := json.MarshalIndent(configData, "", "    ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		configPath := configFilePath()
		configDir := filepath.Dir(configPath)

		// Create the directory for the config file
		if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
			fmt.Fprintf(os.Stderr,
				"\nERROR: The configuration directory %s could not be created; check permissions and try again.\n\n",
				configDir)
			os.Exit(126)
		}

		if err := ioutil.WriteFile(configPath, b, 0644); err != nil {
			fmt.Fprintf(os.Stderr,
				"\nERROR: The configuration file %s could not be written; check permissions and try again. "+
					"\n       By default, configuration files are system-wide for ease of use in cron jobs and scripts. Specify an alternate config file using the --config argument or CRONITOR_CONFIG environment variable.\n\n",
				configPath)
			os.Exit(126)
		}
	},
}

func init() {
	RootCmd.AddCommand(configureCmd)
	configureCmd.Flags().StringSliceP("exclude-from-name", "e", []string{}, "Substring to always exclude from generated monitor name e.g. $ cronitor configure -e '> /dev/null' -e '/path/to/app'")
	configureCmd.Flags().String("dash-username", "", "Username for the dashboard authentication")
	configureCmd.Flags().String("dash-password", "", "Password for the dashboard authentication")
	configureCmd.Flags().String("allowed-ips", "", "Comma-separated list of allowed IP addresses/CIDR ranges (e.g. 192.168.1.0/24,10.0.0.1)")
	configureCmd.Flags().String("ping-api-key", "", "Your Cronitor Ping API key")
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
