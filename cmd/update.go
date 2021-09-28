package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Sorry!\n\nAutomatic updates are no longer available. Please download and install the latest release from https://cronitor.io/docs/using-cronitor-cli\n\n")
	},
}

func init() {
	RootCmd.AddCommand(updateCmd)
}
