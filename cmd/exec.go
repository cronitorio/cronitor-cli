package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"sync"
	"os/exec"
	"errors"
)

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with Cronitor monitoring.",
	Long: ``,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return errors.New("A unique monitor code and cli command are required")
		}

		return nil
	},

	Run: func(cmd *cobra.Command, args []string) {
		var wg sync.WaitGroup
		wg.Add(1)

		go sendPing("run", args[0], &wg)

		wrappedCommand := exec.Command("sh", "-c", args[1])
		err := wrappedCommand.Run()

		if err == nil {
			wg.Add(1)
			go sendPing("complete", args[0], &wg)
		} else {
			fmt.Println(err)
			wg.Add(1)
			go sendPing("fail", args[0], &wg)
		}

		wg.Wait()
	},
}

func init() {
	RootCmd.AddCommand(execCmd)
	RootCmd.Flags()
}

