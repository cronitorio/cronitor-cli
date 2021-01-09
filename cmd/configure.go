package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io/ioutil"
	"os"
)

type ConfigFile struct {
	ApiKey         string   `json:"CRONITOR_API_KEY"`
	PingApiAuthKey string   `json:"CRONITOR_PING_API_KEY"`
	ExcludeText    []string `json:"CRONITOR_EXCLUDE_TEXT,omitempty"`
	Hostname       string   `json:"CRONITOR_HOSTNAME"`
	Log            string   `json:"CRONITOR_LOG"`
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

		if verbose {
			fmt.Println("\nVersion:")
			fmt.Println(Version)
			fmt.Println("\nAPI Key:")
			fmt.Println(configData.ApiKey)
			fmt.Println("\nPing API Key:")
			fmt.Println(configData.PingApiAuthKey)
			fmt.Println("\nHostname:")
			fmt.Println(effectiveHostname())
			fmt.Println("\nTimezone Location:")
			fmt.Println(effectiveTimezoneLocationName())
			fmt.Println("\nDebug Log:")
			fmt.Println(viper.GetString(varLog))
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

		os.MkdirAll(defaultConfigFileDirectory(), os.ModePerm)
		if ioutil.WriteFile(configFilePath(), b, 0644) != nil {
			fmt.Fprintf(os.Stderr,
				"\nERROR: The configuration file %s could not be written; check permissions and try again. "+
					"\n\nBy default, configuration files are system-wide for ease of use in cron jobs and scripts. Specify an alternate config file using the --config argument or CRONITOR_CONFIG environment variable.\n\n", configFilePath())
			os.Exit(126)
		}
	},
}

func configFilePath() string {
	viperConfig := viper.ConfigFileUsed()
	if len(viperConfig) > 0 {
		return viperConfig
	}

	return fmt.Sprintf("%s/cronitor.json", defaultConfigFileDirectory())
}

func init() {
	RootCmd.AddCommand(configureCmd)
	configureCmd.Flags().StringSliceP("exclude-from-name", "e", []string{}, "Substring to always exclude from generated monitor name e.g. $ cronitor configure -e '> /dev/null' -e '/path/to/app'")
	viper.BindPFlag(varExcludeText, configureCmd.Flags().Lookup("exclude-from-name"))
}
