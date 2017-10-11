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
	"errors"
)

type Line struct {
	Name  string
	FullLine string
	CronExpression string
	CommandToRun string
}

type Rule struct {
	RuleType string `json:"rule_type"`
	Value string `json:"value"`
	TimeUnit string `json:"time_unit,omitempty"`
	GraceSeconds uint `json:"grace_seconds,omitempty"`
}

type Monitor struct {
	Name string `json:"name"`
	Key string `json:"key"`
	Rules []Rule `json:"rules"`
	Tags []string `json:"tags"`
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
		if len(args) > 0 {
			cronPath = args[0]
		}
		if runtime.GOOS == "windows" {
			panic(errors.New("sorry, job discovery is not available on Windows"))
		}

		bytes, err := ioutil.ReadFile(cronPath)
		if err != nil {
			panic(err)
		}
		lines := strings.Split(string(bytes), "\n")

		var parsedLines []Line
		for _, line := range lines {
			var cronExpression, command string

			// Skip the current line if it's a comment
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				// todo verbose message this
				continue
			}

			// Split line by whitespace
			splitLine := strings.Fields(line)

			// Parse the line, handling schedules like @daily and standard cron expression
			if len(splitLine) ==  0 {
				// todo verbose message this -- empty line
				continue
			} else if strings.HasPrefix(splitLine[0], "@reboot") {
				// todo verbose message -- @reboot aren't scheduled monitors
				continue
			} else if strings.HasPrefix(splitLine[0], "@") {
				cronExpression = splitLine[0]
				command = strings.Join(splitLine[1:], " ")
			} else if len(splitLine) >= 6 {
				cronExpression = strings.Join(splitLine[0:5], " ")
				command = strings.Join(splitLine[5:], " ")
			} else {
				// todo verbose message this -- could be an environment variable or anything else
				continue
			}

			fmt.Println(cronExpression, command)

			monitor := Line{}
			monitor.CronExpression = cronExpression
			monitor.CommandToRun = command
			monitor.FullLine = line
			parsedLines = append(parsedLines, monitor)
		}

		// construct JSON payload
		var monitors []Monitor
		for _, line := range parsedLines {
			rules := []Rule{createRule(line.CronExpression)}
			name := createName(line.CommandToRun)
			key := createKey(line.CommandToRun, line.CronExpression)
			monitor := Monitor{name, key, rules, []string{"tags", "are", "cool"}}
			monitors = append(monitors, monitor)			
		}

		b, _ := json.Marshal(monitors)
		fmt.Println(string(b))
					  
	},
}

func createName(CommandToRun string) string {
	return CommandToRun
}

func createKey(CommandToRun string, CronExpression string) string {
	return "keykey"
}

func createRule(cronExpression string) Rule {
	var rule Rule
	if strings.HasPrefix(cronExpression, "@yearly") {
		rule = Rule{"complete_ping_not_received", "365", "days", 86400}
	} else if strings.HasPrefix(cronExpression, "@monthly") {
		rule =  Rule{"complete_ping_not_received", "31", "days", 86400}
	} else if strings.HasPrefix(cronExpression, "@weekly") {
		rule =  Rule{"complete_ping_not_received", "7", "days", 86400}
	} else if strings.HasPrefix(cronExpression, "@daily") {
		rule =  Rule{"complete_ping_not_received", "24", "hours", 3600}
	} else if strings.HasPrefix(cronExpression, "@hourly") {
		rule =  Rule{"complete_ping_not_received", "1", "hours", 600}
	} else {
		rule =  Rule{"not_on_schedule", cronExpression, "", 0}
	}

	return rule
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
