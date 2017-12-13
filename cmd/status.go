package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"errors"
	"github.com/spf13/viper"
	"encoding/json"
	"bytes"
	"github.com/olekukonko/tablewriter"
	"os"
)

type StatusMonitor struct {
	Name       string  `json:"name"`
	Code       string  `json:"code"`
	Passing    bool	  `json:"passing"`
	Status     string  `json:"status"`
}

type StatusMonitors struct {
	Monitors	[]StatusMonitor	`json:"monitors"`
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "View monitor status",
	Long:  `
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
		url := effectiveApiUrl()
		if len(args) > 0 {
			url = url + "/" + args[0]
		}

		response, err := sendApiRequest(url)
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
		table.SetHeader([]string{"Health", "Name", "Code", "Status"})
		table.SetAutoWrapText(false)
		table.SetHeaderAlignment(3)

		for _, v := range responseMonitors.Monitors {
			state := "Pass"
			if !v.Passing {
				state = "Fail"
			}
			table.Append([]string{state, v.Name, v.Code, v.Status})
		}

		table.Render()
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}

