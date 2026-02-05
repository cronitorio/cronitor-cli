package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var printJSON bool

var listCmd = &cobra.Command{
	GroupID: GroupCron,
	Use:     "list <optional path>",
	Short: "Search for and list all cron jobs",
	Long: `
Cronitor list scans for cron jobs and displays them in an easy to read format.

Example:
  $ cronitor list
      > List all cron jobs in your user crontab and system directory

  $ cronitor list /path/to/crontab
      > Instead of the user crontab, list the jobs in a provided a crontab file (or directory of crontabs)

  $ cronitor list --json
      > Output all discovered cron jobs as JSON
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		crontabs := gatherCrontabs(args)
		if len(crontabs) == 0 {
			printWarningText("No crontab files found", false)
			return
		}

		if printJSON {
			if err := printListAsJSON(os.Stdout, crontabs); err != nil {
				fmt.Fprintf(os.Stderr, "Error encoding JSON: %s\n", err)
				os.Exit(1)
			}
		} else {
			printListAsTable(os.Stdout, crontabs)
		}
	},
}

func init() {
	RootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&printJSON, "json", "j", false, "Output as JSON")
}

// gatherCrontabs collects crontabs from the specified args or the default locations.
func gatherCrontabs(args []string) []*lib.Crontab {
	var username string
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	crontabs := []*lib.Crontab{}

	if len(args) > 0 {
		if isPathToDirectory(args[0]) {
			crontabs = lib.ReadCrontabsInDirectory(username, args[0], crontabs)
		} else {
			crontabs = lib.ReadCrontabFromFile(username, args[0], crontabs)
		}
	} else {
		users := parseUsers()
		if len(users) == 0 {
			users = []string{username}
		}

		for _, user := range users {
			crontabs = lib.ReadCrontabFromFile(user, fmt.Sprintf("user:%s", user), crontabs)
		}
		crontabs = lib.ReadCrontabFromFile(username, lib.SYSTEM_CRONTAB, crontabs)
		crontabs = lib.ReadCrontabsInDirectory(username, lib.DROP_IN_DIRECTORY, crontabs)
	}

	return crontabs
}

// jobLines returns only the lines that have a command to run.
func jobLines(crontab *lib.Crontab) []*lib.Line {
	var lines []*lib.Line
	for _, line := range crontab.Lines {
		if len(line.CommandToRun) > 0 {
			lines = append(lines, line)
		}
	}
	return lines
}

// printListAsTable renders crontabs as human-readable tables.
func printListAsTable(w io.Writer, crontabs []*lib.Crontab) {
	fmt.Fprintln(w)
	for _, crontab := range crontabs {
		lines := jobLines(crontab)
		if len(lines) == 0 {
			continue
		}

		table := tablewriter.NewWriter(w)
		table.SetHeader([]string{"Schedule", "Command"})
		table.SetAutoWrapText(true)
		table.SetHeaderAlignment(3)
		table.SetColMinWidth(0, 17)
		table.SetColMinWidth(1, 100)

		for _, line := range lines {
			table.Append([]string{line.CronExpression, line.CommandToRun})
		}

		printSuccessText(fmt.Sprintf("Checking %s", crontab.DisplayName()), false)
		table.Render()
		fmt.Fprintln(w)
	}
}

// printListAsJSON marshals crontabs to JSON and writes to w.
// Only crontabs with job lines are included, and within each crontab
// only job lines are emitted (matching the table output behavior).
func printListAsJSON(w io.Writer, crontabs []*lib.Crontab) error {
	var output []lib.Crontab
	for _, crontab := range crontabs {
		jobs := jobLines(crontab)
		if len(jobs) == 0 {
			continue
		}
		// Shallow copy with only job lines so comments/env vars are excluded
		filtered := *crontab
		filtered.Lines = jobs
		output = append(output, filtered)
	}

	if output == nil {
		output = []lib.Crontab{}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}
