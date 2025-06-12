package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type StatusMonitor struct {
	Name    string `json:"name"`
	Key     string `json:"key"`
	Passing bool   `json:"passing"`
	Paused  bool   `json:"paused"`
}

type StatusMonitors struct {
	Monitors []StatusMonitor `json:"monitors"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "View monitor status",
	Long: `
View monitor status

Examples:
  View status of all monitors:
  $ cronitor status

  View status of a single monitor:
  $ cronitor status d3x0c1
`,

	Args: func(cmd *cobra.Command, args []string) error {
		if len(viper.GetString(varApiKey)) < 10 {
			return errors.New("you must provide an API key with this command or save a key using 'cronitor configure'")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		url := getCronitorApi().Url()
		if len(args) > 0 {
			url = url + "/" + args[0]
		}

		// @todo refactor this to use GetMonitors
		response, err := getCronitorApi().GetRawResponse(url)
		if err != nil {
			fatal(fmt.Sprintf("Request to %s failed: %s", url, err), 1)
		}

		buf := new(bytes.Buffer)
		json.Indent(buf, response, "", "  ")
		log("\nResponse:")
		log(buf.String() + "\n")

		// Unmarshall the API response into the StatusMonitor struct.
		// If this is a detail response (with a monitor code argument) we need to unmarshal it directly into a StatusMonitor
		responseMonitors := StatusMonitors{}
		if len(args) == 0 {
			if err = json.Unmarshal(response, &responseMonitors); err != nil {
				fatal(fmt.Sprintf("Error %s from %s: %s", err.Error(), url, response), 1)
			}
		} else {
			singleMonitor := StatusMonitor{}
			if err = json.Unmarshal(response, &singleMonitor); err != nil {
				fatal(fmt.Sprintf("Error %s from %s: %s", err.Error(), url, response), 1)
			}

			responseMonitors.Monitors = []StatusMonitor{singleMonitor}
		}

		fmt.Println(url)
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Health", "Name", "Code", "Alerts"})
		table.SetAutoWrapText(false)
		table.SetHeaderAlignment(3)

		for _, v := range responseMonitors.Monitors {
			state := "Ok"
			if !v.Passing {
				state = "Failing"
			}

			alertStatus := "On"
			if v.Paused {
				alertStatus = "Muted"
			}
			table.Append([]string{state, v.Name, v.Key, alertStatus})
		}

		table.Render()
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}
