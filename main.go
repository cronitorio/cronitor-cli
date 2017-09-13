package main

import "fmt"

import (
	"gopkg.in/urfave/cli.v1" // imports as package "cli"
	"os"
	"os/exec"
	"time"
	"net/http"
	"sync"
)

func sendPing(endpoint str, group *sync.WaitGroup) {
	Client := &http.Client{
		Timeout: time.Second * 3,
	}

	for i:=1; i<=5; i++  {
		_, err = Client.Get( fmt.Sprintf("https://cronitor.link/%s/%s?try=%d", c.Args().Get(0), endpoint, i))
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
			Destination: &verbose,
		},
	}
	app.Action = func(c *cli.Context) error {

		var wg sync.WaitGroup

		if verbose {
			fmt.Println("Sending run ping")
		}

		wg.Add(1)
		go sendPing("run", &wg)

		cmd := exec.Command("sh", "-c", c.Args().Get(1))
		err := cmd.Run()
		if err == nil {
			if verbose {
				fmt.Println("Sending complete ping")
			}
			wg.Add(1)
			go sendPing("complete", &wg)

		} else {
			fmt.Println(err)
			if verbose {
				fmt.Println("Sending fail ping")
			}
			wg.Add(1)
			go sendPing("fail", &wg)
		}

		wg.Wait()
		return nil
	}

	app.Run(os.Args)
}
