package cmd

import (
	"github.com/spf13/cobra"
	"sync"
	"errors"
)

var start bool
var complete bool
var fail bool
var msg string

var pingCmd = &cobra.Command{
	Use:   "ping <code>",
	Short: "Send a single ping to Cronitor",
	Long:  ``,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("a unique monitor code is required")
		}

		if len(getEndpointFromFlag()) == 0 {
			return errors.New("an endpoint flag is required")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup

		wg.Add(1)
		go sendPing(getEndpointFromFlag(), args[0], msg, &wg)
		wg.Wait()
	},
}

func getEndpointFromFlag() string {
	if fail {
		return "fail"
	} else if complete {
		return "complete"
	} else if start {
		return "run"
	}

	return ""
}

func init() {
	RootCmd.AddCommand(pingCmd)
	pingCmd.Flags().BoolVar(&start, "start", false, "Send a /run ping")
	pingCmd.Flags().BoolVar(&complete, "complete", false, "Send a /complete ping")
	pingCmd.Flags().BoolVar(&fail, "fail", false, "Send a /fail ping")
	pingCmd.Flags().StringVar(&msg, "msg", "", "Optional message to send with ping")
}
