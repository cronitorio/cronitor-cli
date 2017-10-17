package cmd

import (
	"github.com/spf13/cobra"
	"sync"
	"errors"
	"fmt"
	"os"
)

var start bool
var complete bool
var fail bool
var msg string

var pingCmd = &cobra.Command{
	Use:   "ping <code>",
	Short: "Send a single ping to Cronitor",
	Long: ``,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("A unique monitor code is required")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup
		var endpoint string
		if fail {
			endpoint = "fail"
		} else if complete {
			endpoint = "complete"
		} else if start {
			endpoint = "run"
		} else {
			fmt.Fprintln(os.Stderr, "an endpoint flag must be provided")
			os.Exit(1)
		}

		wg.Add(1)
		go sendPing(endpoint, args[0], msg, &wg)
		wg.Wait()
	},
}

func init() {
	RootCmd.AddCommand(pingCmd)
	pingCmd.Flags().BoolVar(&start, "start", false, "Send a /run ping")
	pingCmd.Flags().BoolVar(&complete, "complete", false, "Send a /complete ping")
	pingCmd.Flags().BoolVar(&fail, "fail", false, "Send a /fail ping")
	pingCmd.Flags().StringVar(&msg, "msg", "", "Optional message to send with ping" )
}

