package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"sync"
	"os/exec"
	"errors"
)

var monitorCode, command string
var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with Cronitor monitoring.",
	Long: ``,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("A unique monitor code and cli command are required")
		}

		monitorCode = args[0]
		command = args[1]
		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup
		wg.Add(1)

		if verbose {
			fmt.Println(fmt.Sprintf("Running command: %s", command))
		}

		go sendPing("run", monitorCode, "", &wg)

		output, err := exec.Command("sh", "-c", command).Output()
		fmt.Println(string(output))

		if err == nil {
			wg.Add(1)
			go sendPing("complete", monitorCode, "", &wg)
		} else {
			wg.Add(1)
			go sendPing("fail", monitorCode, err.Error(), &wg)
		}

		wg.Wait()
	},
}

func init() {
	RootCmd.AddCommand(execCmd)
	RootCmd.Flags()
}

