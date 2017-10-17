// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"io/ioutil"
	"github.com/spf13/viper"
	"encoding/json"
	"os"
)

type ConfigFile struct {
	ApiKey     string `json:"CRONITOR-API-KEY"`
	ExcludeText []string `json:"CRONITOR-EXCLUDE-TEXT,omitempty"`
}

// configureCmd represents the configure command
var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Write required configuration variables to the selected config file.",
	Long: `Set configuration variables`,
	Run: func(cmd *cobra.Command, args []string) {
		configData := ConfigFile{}
		configData.ApiKey = viper.GetString("CRONITOR-API-KEY")
		configData.ExcludeText = viper.GetStringSlice("CRONITOR-EXCLUDE-TEXT")

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
	viper.BindPFlag("CRONITOR-EXCLUDE-TEXT", configureCmd.Flags().Lookup("exclude-from-name"))
}
