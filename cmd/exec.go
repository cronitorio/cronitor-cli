package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"sync"
	"net/http"
	"time"
	"os/exec"
	"errors"
)

var execCmd = &cobra.Command{
	Use:   "exec",
	Short: "Execute a command with Cronitor monitoring",
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

		//var verbose Flag
		//verbose = cmd.PersistentFlags().Lookup("verbose")
		//if verbose {
		//	fmt.Println("is verbose")
		//} else {
		//	fmt.Println("is verbose")
		//}

		verbose := true
		go sendPing("run", args[0], verbose, &wg)

		wrappedCommand := exec.Command("sh", "-c", args[1])
		err := wrappedCommand.Run()

		if err == nil {
			wg.Add(1)
			go sendPing("complete", args[0], verbose, &wg)
		} else {
			fmt.Println(err)
			wg.Add(1)
			go sendPing("fail", args[0], verbose, &wg)
		}

		wg.Wait()
	},
}

func init() {
	RootCmd.AddCommand(execCmd)
	RootCmd.Flags()
}

func sendPing(endpoint string, uniqueIdentifier string, verbose bool, group *sync.WaitGroup) {
	if verbose {
		fmt.Printf("Sending %s ping", endpoint)
	}

	Client := &http.Client{
		Timeout: time.Second * 3,
	}

	for i:=1; i<=6; i++  {
		// Determine the ping API host. After a few failed attempts, try using cronitor.io instead
		var host string
		if i > 3 && host == "cronitor.link" {
			host = "cronitor.io"
		} else {
			host = "cronitor.link"
		}

		_, err := Client.Get( fmt.Sprintf("https://%s/%s/%s?try=%d", host, uniqueIdentifier, endpoint, i))
		if err == nil {
			break
		}
	}

	group.Done()
}