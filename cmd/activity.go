package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"errors"
	"github.com/spf13/viper"
	"encoding/json"
	"bytes"
)

var before string
var only string

var activityCmd = &cobra.Command{
	Use:   "activity",
	Short: "View monitor activity",
	Long:  `
View monitor pings and alerts

Examples:
  View combined pings and alerts in reverse chronological order:
  $ cronitor activity d3x0c1

  View only alerts:
  $ cronitor activity d3x0c1 --only alerts

  View only pings:
  $ cronitor activity d3x0c1 --only pings

  View only pings before a certain timestamp:
  $ cronitor activity d3x0c1 --only pings --before 1510971199.905
`,

	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("a unique monitor code is required")
		}

		if len(only) > 0 && !isValidOnlyFilter() {
			return errors.New("invalid argument supplied to 'only'. Expecting 'pings' or 'alerts'")
		}

		if len(viper.GetString("CRONITOR-API-KEY")) < 10 {
			return errors.New("you must provide an API key with this command or save a key using 'cronitor configure'")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		url := createActivityApiUrl(args[0])
		response, err := sendApiRequest(url)
		if err != nil {
			fatal(fmt.Sprintf("Request to %s failed: %s", url, err), 1)
		}

		buf := new(bytes.Buffer)
		json.Indent(buf, response, "", "  ")
		fmt.Println(url)
		if bufString :=  buf.String(); bufString != "[]" {
			fmt.Println(bufString)
		} else {
			fmt.Println("No activity")
		}
	},
}

func init() {
	RootCmd.AddCommand(activityCmd)
	activityCmd.Flags().StringVar(&only,"only", only, "Accepted values: pings, alerts")
	activityCmd.Flags().StringVar(&before, "before", before, "Return events before provided timestamp")
}

func createActivityApiUrl(uniqueIdentifier string) string {
	endpoint := "activity"
	if len(only) > 0 {
		endpoint = only
	}

	beforeFilter := ""
	if len(before) > 0 {
		beforeFilter = fmt.Sprintf("?before=%s", before)
	}

	return fmt.Sprintf("%s/%s/%s%s", effectiveApiUrl(), uniqueIdentifier, endpoint, beforeFilter)
}

func isValidOnlyFilter() bool {
	switch only {
	case
		"pings",
		"alerts":
			return true
	}

	return false
}
