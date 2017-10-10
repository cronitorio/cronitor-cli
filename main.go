package main

import "fmt"

import (
	"gopkg.in/urfave/cli.v1" // imports as package "cli"
	"os"
	"os/exec"
	"time"
	"net/http"
	"sync"
	//"github.com/spf13/cobra/cobra/cmd"
)

func sendPing(endpoint string, uniqueIdentifier string, verbose bool, group *sync.WaitGroup) {
	if verbose {
		fmt.Printf("Sending %s ping", endpoint)
	}

	Client := &http.Client{
		Timeout: time.Second * 3,
	}

	for i:=1; i<=5; i++  {
		_, err := Client.Get( fmt.Sprintf("https://cronitor.link/%s/%s?try=%d", uniqueIdentifier, endpoint, i))
		if err == nil {
			break
		}
	}

	group.Done()
}

func main() {
	verbose := false

	app := cli.NewApp()
	app.Name = "Cronitor Agent"
	app.Usage = "https://cronitor.io/docs/server-agent"
	app.Version = "0.1.0"
	app.Compiled = time.Now()
	app.Authors = []cli.Author{
		cli.Author{
			Name:  "Shane Harter",
			Email: "shane@cronitor.io",
		},
	}
	app.Copyright = "(c) 2017 Cronitor.io"
	app.ArgsUsage = "UNIQUEKEY COMMAND"
	app.Flags = []cli.Flag {
		cli.BoolTFlag{
			Name:        "verbose",
			Usage:       "Verbose output",
		},
	}

	app.Action = func(c *cli.Context) error {
		var wg sync.WaitGroup
		wg.Add(1)

		if verbose {
			fmt.Println("is verbose")
		} else {
			fmt.Println("is verbose")
		}

		go sendPing("run", c.Args().Get(0), verbose, &wg)

		if len(c.Args()) > 1 {
			cmd := exec.Command("sh", "-c", c.Args().Get(1))
			err := cmd.Run()
			if err == nil {
				wg.Add(1)
				go sendPing("complete", c.Args().Get(0), verbose, &wg)
			} else {
				fmt.Println(err)
				wg.Add(1)
				go sendPing("fail", c.Args().Get(0), verbose, &wg)
			}
		}

		wg.Wait()
		return nil
	}

	app.Run(os.Args)
}


$ cronitor ping {unique code} [--start|--complete|--fail]

$ cronitor exec {unique code} {command}

$ cronitor config {API KEY}
 	- API Key

$ cronitor discover [crontab] [--rewrite] [--exclude-from-name] [--grace-seconds]
	- Only on linux
	- Read crontab, split using same ruby logic
	- Build internal struct
		- For each line, create a unique key
			- Combine the hostname of the machine plus a hash of the current line
			- Build name by stripping common things like `2&> /dev/null` and strip other strings supplied
			- No notifications or other settings
			- Create auto-tag for the hostname ?

	- Use put endpoint
	- Result from PUT and re-build the crontab lines
	- If rewrite flag is set, write the crontab and say "sucessful". Otherwise write to stdout


