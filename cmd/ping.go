package cmd

import (
	"errors"
	"github.com/spf13/cobra"
	"sync"
)

var run bool
var complete bool
var fail bool
var tick bool
var msg string

var pingCmd = &cobra.Command{
	Use:   "ping <key>",
	Short: "Send a telemetry ping to Cronitor",
	Long: `
Ping the specified monitor to report current status.

Example:
  Notify Cronitor that your job has started to run
  $ cronitor ping d3x0c1 --run

Example with a custom hostname:
  $ cronitor ping d3x0c1 --run --hostname "custom-name"
  If no hostname is provided, the system hostname is used.

Example with a custom message:
  $ cronitor ping d3x0c1 --fail -msg "Error: Job was not successful"

Example when using authenticated ping requests:
  $ cronitor ping d3x0c1 --complete --ping-api-key 9134e94e13a098dbaca57c2df2f2c06f

	`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("a unique monitor key is required")
		}

		if len(getEndpointFromFlag()) == 0 {
			return errors.New("an endpoint flag is required")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup

		wg.Add(1)
		go sendPing(getEndpointFromFlag(), args[0], msg, "", makeStamp(), nil, nil, &wg)
		wg.Wait()
	},
}

func getEndpointFromFlag() string {
	if fail {
		return "fail"
	} else if complete {
		return "complete"
	} else if run {
		return "run"
	} else if tick {
		return "tick"
	}

	return ""
}

func init() {
	RootCmd.AddCommand(pingCmd)
	pingCmd.Flags().BoolVar(&run, "run", false, "Report job is running")
	pingCmd.Flags().BoolVar(&complete, "complete", false, "Report job completion")
	pingCmd.Flags().BoolVar(&fail, "fail", false, "Report job failure")
	pingCmd.Flags().BoolVar(&tick, "tick", false, "Send a heartbeat")
	pingCmd.Flags().StringVar(&msg, "msg", "", "Optional message to send with ping")
}
