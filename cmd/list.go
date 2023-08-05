package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/user"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

const listLongDescription = `
Cronitor list scans for cron jobs and displays them in an easy to read table

Example:
  $ cronitor list
      > List all cron jobs in your user crontab and system directory

  $ cronitor list /path/to/crontab
      > Instead of the user crontab, list the jobs in a provided a crontab file (or directory of crontabs)
	`

var printJson bool

var listCmd = &cobra.Command{
	Use:   "list <optional path>",
	Short: "Search for and list all cron jobs",
	Long:  listLongDescription,
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},

	Run: runListCmd,
}

func init() {
	RootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolVarP(&printJson, "json", "j", false, "Print output in json format")
}

func runListCmd(cmd *cobra.Command, args []string) {
	var username string
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	crontabs := []*lib.Crontab{}

	if len(args) > 0 {
		// A supplied argument can be a specific file or a directory
		if isPathToDirectory(args[0]) {
			crontabs = lib.ReadCrontabsInDirectory(username, args[0], crontabs)
		} else {
			crontabs = lib.ReadCrontabFromFile(username, args[0], crontabs)
		}
	} else {
		// Without a supplied argument look at the user crontab, system crontab, and the system drop-in directory
		crontabs = lib.ReadCrontabFromFile(username, "", crontabs)
		crontabs = lib.ReadCrontabFromFile(username, lib.SYSTEM_CRONTAB, crontabs)
		crontabs = lib.ReadCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)
	}

	cts := filterEmptyCrontabs(crontabs)

	// using a switch here in case we want to add more output formats in the future
	switch {
	case len(crontabs) == 0:
		printWarningText("No crontab files found", false)
	case printJson:
		printToJson(cts)
	default:
		printToTable(cts)
	}
}

// filterEmptyCrontabs removes any empty crontabs and lines
func filterEmptyCrontabs(crontabs []*lib.Crontab) []*lib.Crontab {
	cts := []*lib.Crontab{}

	for _, crontab := range crontabs {
		if len(crontab.Lines) == 0 {
			continue
		}

		ct := &lib.Crontab{
			Lines:         []*lib.Line{},
			IsUserCrontab: crontab.IsUserCrontab,
			Filename:      crontab.Filename,
			User:          crontab.User}

		for _, line := range crontab.Lines {
			if len(line.CommandToRun) == 0 {
				continue
			}

			ct.Lines = append(ct.Lines, line)
		}

		if len(ct.Lines) > 0 {
			ct.Filename = crontab.Filename
			ct.User = crontab.User

			cts = append(cts, ct)
		}
	}

	return cts
}

func printToTable(crontabs []*lib.Crontab) {
	for _, crontab := range crontabs {
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Schedule", "Command"})
		table.SetAutoWrapText(true)
		table.SetHeaderAlignment(3)
		table.SetColMinWidth(0, 17)
		table.SetColMinWidth(1, 100)

		for _, line := range crontab.Lines {
			if len(line.CommandToRun) == 0 {
				continue
			}

			table.Append([]string{line.CronExpression, line.CommandToRun})
		}

		fmt.Println()
		printSuccessText(fmt.Sprintf("Checking %s", crontab.DisplayName()), false)
		table.Render()
		fmt.Println()
	}
}

func printToJson(crontabs []*lib.Crontab) {
	jd, err := MarshalWithoutEscaping(crontabs)
	if err != nil {
		printErrorText(fmt.Sprintf("Error converting to json: %s", err.Error()), false)
	}

	fmt.Println(string(jd))
}

// MarshalWithoutEscaping returns the JSON encoding of an interface.
// We use this instead of json.Marshal because we don't want to escape characters for HTML.
func MarshalWithoutEscaping(i interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(i)
	if err != nil {
		return nil, err
	}

	// Encode adds a newline to the end of the buffer, so we trim it off.
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}
