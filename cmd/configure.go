package cmd

import (
	"fmt"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"io/ioutil"
	"github.com/spf13/viper"
	"encoding/json"
	"os"
)

type ConfigFile struct {
	ApiKey         string   `json:"CRONITOR_API_KEY"`
	PingApiAuthKey string   `json:"CRONITOR_PING_API_KEY"`
	ExcludeText    []string `json:"CRONITOR_EXCLUDE_TEXT,omitempty"`
	Hostname       string   `json:"CRONITOR_HOSTNAME"`
	Log       		string   `json:"CRONITOR_LOG"`
}

// configureCmd represents the configure command
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Save configuration variables to the config file",
	Long:  `
Optionally write configuration options to a JSON file, by default ~/.cronitor.json.

Cronitor Cli configuration can be supplied from a file, environment variables, or command line flags.
You can use a default config file for some things and environment variables or command line flags for others -- the goal is flexibility.

Environment variables that are read:
  CRONITOR_API_KEY
  CRONITOR_PING_API_KEY
  CRONITOR_EXCLUDE_TEXT
  CRONITOR_HOSTNAME
  CRONITOR_LOG

Example setting your API Key:
  $ cronitor configure --api-key 4319e94e890a013dbaca57c2df2ff60c2

Example setting common exclude text for use with 'cronitor discover':
  $ cronitor configure -e "/var/app/code/path/" -e "/var/app/bin/" -e "> /dev/null"`,
	Run: func(cmd *cobra.Command, args []string) {

		if verbose {
			fmt.Println("\nHostname:")
			fmt.Println(effectiveHostname())
			fmt.Println("\nLocation:")
			fmt.Println(effectiveTimezoneLocationName())
		}

		configData := ConfigFile{}
		configData.ApiKey = viper.GetString(varApiKey)
		configData.PingApiAuthKey = viper.GetString(varPingApiKey)
		configData.ExcludeText = viper.GetStringSlice(varExcludeText)
		configData.Hostname = viper.GetString(varHostname)
		configData.Log = viper.GetString(varLog)

		b, err := json.MarshalIndent(configData, "", "    ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		if ioutil.WriteFile(configFilePath(), b, 0644) != nil {
			fmt.Fprintf(os.Stderr, "the configuration file at %s could not be written; check permissions and try again", configFilePath())
			os.Exit(126)
		}
	},
}

func configFilePath() string {
	viperConfig := viper.ConfigFileUsed()
	if len(viperConfig) > 0 {
		return viperConfig
	}

	defaultConfig, _ := homedir.Expand("~/.cronitor.json")
	return defaultConfig
}

func init() {
	RootCmd.AddCommand(configureCmd)
	configureCmd.Flags().StringSliceP("exclude-from-name", "e", []string{}, "Substring to always exclude from generated monitor name e.g. $ cronitor configure -e '> /dev/null' -e '/path/to/app'")
	viper.BindPFlag(varExcludeText, configureCmd.Flags().Lookup("exclude-from-name"))
}
