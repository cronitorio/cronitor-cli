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
	"runtime"
	"fmt"
	"encoding/json"
	// "errors"
	// "os/exec"
	// "sync"
	// "io"
	"io/ioutil"
	"strings"

	"github.com/spf13/cobra"
)

type ParsedLine struct {
	Name  string
	FullLine string
	CronExpression string
	CommandToRun string
}

type Rule struct {
	RuleType string `json:"rule_type"`
	Value string	`json:"value"`
}

type Monitor struct {
	Name string `json:"name"`
	Key string `json:"key"`
	Rules []Rule `json:"rules"`
	Tags []string `json:"tags"`
}



func check(e error) {
	if e != nil {
			panic(e)
	}
}

// discoverCmd represents the discover command
var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Args: func(cmd *cobra.Command, args []string) error {
		// if len(args) < 2 {
		// 	return errors.New("A unique monitor code and cli command are required")
		// }

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cronPath := "/etc/crontab"
		if len(args[0]) > 0 {
			cronPath = args[0]
		}
		if runtime.GOOS == "windows" {
			// TODO bail out here
		}
		dat, err := ioutil.ReadFile(cronPath)
		check(err)
		lines := strings.Split(string(dat), "\n")

		// parse each line
		var parsedLines []ParsedLine
		for _, line := range lines {
			// # Skip the current line if it's a comment, empty, or MAILTO
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "MAILTO") || line == "" {
				continue
			}

			// # Split line by whitespace
			splitLine := strings.Fields(line)
			cronExpression := strings.Join(splitLine[0:5], " ")
			command := strings.Join(splitLine[5:], " ")
			fmt.Println(cronExpression, command)
			
			// # If we have an @ cron expression - eg @hourly, cronitor doesn't support those, so skip this line
			if strings.HasPrefix(cronExpression, "@") {
				fmt.Println("Non standard '@' format cron expression detected, cannot create cronitor job. Skpping...")
				continue
			}
			
			monitor := ParsedLine{}
			monitor.CronExpression = cronExpression
			monitor.CommandToRun = command
			monitor.FullLine = line
			parsedLines = append(parsedLines, monitor)
		}

		// construct JSON payload
		var monitors []Monitor
		for _, pline := range parsedLines {
			var rules []Rule
			rule := Rule{"not_on_schedule", pline.CronExpression}
			rules = append(rules, rule)
			monitor := Monitor{pline.Name, "some_key", rules, []string{"tags", "are", "cool"}}
			monitors = append(monitors, monitor)			
		}

		b, _ := json.Marshal(monitors)
		fmt.Println(string(b))
					  
	},
}

func init() {
	RootCmd.AddCommand(discoverCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// discoverCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// discoverCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
