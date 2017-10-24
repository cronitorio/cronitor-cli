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
	ApiKey         string   `json:"CRONITOR-API-KEY"`
	PingApiAuthKey string   `json:"CRONITOR-PING-API-AUTH-KEY"`
	ExcludeText    []string `json:"CRONITOR-EXCLUDE-TEXT,omitempty"`
	Hostname       string   `json:"CRONITOR-HOSTNAME"`
}

// configureCmd represents the configure command
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Write required configuration variables to the selected config file.",
	Long:  `Set configuration variables`,
	Run: func(cmd *cobra.Command, args []string) {
		configData := ConfigFile{}
		configData.ApiKey = viper.GetString("CRONITOR-API-KEY")
		configData.PingApiAuthKey = viper.GetString("CRONITOR-PING-API-AUTH-KEY")
		configData.ExcludeText = viper.GetStringSlice("CRONITOR-EXCLUDE-TEXT")
		configData.Hostname = viper.GetString("CRONITOR-HOSTNAME")

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
	configureCmd.Flags().String("ping-api-auth-key", "", "Ping api auth key - see https://cronitor.io/docs/understanding-ping-urls#security")
	viper.BindPFlag("CRONITOR-PING-API-AUTH-KEY", configureCmd.Flags().Lookup("ping-api-auth-key"))

	configureCmd.Flags().StringSliceP("exclude-from-name", "e", []string{}, "Substring to always exclude from generated monitor name e.g. $ cronitor configure -e '> /dev/null' -e '/path/to/app'")
	viper.BindPFlag("CRONITOR-EXCLUDE-TEXT", configureCmd.Flags().Lookup("exclude-from-name"))
}