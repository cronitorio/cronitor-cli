package cmd

import (
	"fmt"
	"os"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sync"
	"net/http"
	"time"
	"io/ioutil"
	"net/url"
	"strconv"
)

var version = "0.8.1"
var cfgFile string
var userAgent string

// Flags that are either global or used in multiple commands
var apiKey string
var debugLog string
var dev bool
var verbose bool
var noStdoutPassthru bool

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cronitor",
	Short: fmt.Sprintf("Cronitor CLI tools version %s", version),
	Long:  ``,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	userAgent = fmt.Sprintf("CronitorCli/%s", version)
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", cfgFile, "Config file (default: .cronitor.json)")
	RootCmd.PersistentFlags().StringVarP(&apiKey, "api-key", "k", apiKey, "Cronitor API Key")
	RootCmd.PersistentFlags().StringVarP(&apiKey, "ping-api-key", "p", apiKey, "Ping API Key")
	RootCmd.PersistentFlags().StringVarP(&apiKey, "hostname", "n", apiKey, "A unique identifier for this host (default: system hostname)")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "Verbose output")
	RootCmd.PersistentFlags().BoolVar(&noStdoutPassthru, "no-stdout", noStdoutPassthru, "Do not send cron job output to Cronitor when your job completes")

	RootCmd.PersistentFlags().BoolVar(&dev, "use-dev", dev, "Dev mode")
	RootCmd.PersistentFlags().MarkHidden("use-dev")

	RootCmd.PersistentFlags().StringVar(&debugLog, "log", debugLog, "Write debug logs to supplied file")
	RootCmd.PersistentFlags().MarkHidden("log")

	viper.BindPFlag("CRONITOR-API-KEY", RootCmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag("CRONITOR-HOSTNAME", RootCmd.PersistentFlags().Lookup("hostname"))
	viper.BindPFlag("CRONITOR-CONFIG", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("CRONITOR-LOG", RootCmd.PersistentFlags().Lookup("log"))
	viper.BindPFlag("CRONITOR-PING-API-AUTH-KEY", RootCmd.PersistentFlags().Lookup("ping-api-key"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory
		viper.AddConfigPath(home)
		viper.SetConfigName(".cronitor")
	}

	viper.AutomaticEnv() // read in environment variables that match
	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log("Reading config from " + viper.ConfigFileUsed())
	}
}

func sendPing(endpoint string, uniqueIdentifier string, message string, group *sync.WaitGroup) {
	defer group.Done()

	Client := &http.Client{
		Timeout: time.Second * 3,
	}

	pingApiAuthKey := viper.GetString("CRONITOR-PING-API-AUTH-KEY")
	hostname := effectiveHostname()
	stamp := strconv.FormatFloat(makeStamp(), 'f', 3, 64)

	if len(message) > 0 {
		message = fmt.Sprintf("&msg=%s", url.QueryEscape(truncateString(message, 2000)))
	}

	if len(pingApiAuthKey) > 0 {
		pingApiAuthKey = fmt.Sprintf("&auth_key=%s", truncateString(pingApiAuthKey, 50))
	}

	if len(hostname) > 0 {
		hostname = fmt.Sprintf("&hostname=%s", url.QueryEscape(truncateString(hostname, 50)))
	}

	for i := 1; i <= 6; i++ {
		// Determine the ping API host. After a few failed attempts, try using cronitor.io instead
		var host string
		if dev {
			host = "http://dev.cronitor.io"
		} else if i > 2 && host == "https://cronitor.link" {
			host = "https://cronitor.io"
		} else {
			host = "https://cronitor.link"
		}

		uri := fmt.Sprintf("%s/%s/%s?try=%d&stamp=%s%s%s%s", host, uniqueIdentifier, endpoint, i, stamp, message, pingApiAuthKey, hostname)
		log("Sending ping " + uri)

		request, err := http.NewRequest("GET", uri, nil)
		request.Header.Add("User-Agent", userAgent)
		response, err := Client.Do(request)

		if err != nil {
			fmt.Println(err)

			// After 3 failed attempts, begin to sleep between tries
			if i > 2 {
				time.Sleep(time.Second * 2)
			}
			continue
		}

		_, err = ioutil.ReadAll(response.Body)
		if err == nil && response.StatusCode < 400 {
			break
		}

		response.Body.Close()
	}
}

func effectiveHostname() string {
	if len(viper.GetString("CRONITOR-HOSTNAME")) > 0 {
		return viper.GetString("CRONITOR-HOSTNAME")
	}

	hostname, _ := os.Hostname()
	return hostname
}

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}

	return s[:length]
}

func log(msg string) {
	if len(debugLog) > 0 {
		if verbose {
			fmt.Println("Writing to log file " + debugLog)
		}
		f, _ := os.OpenFile(debugLog, os.O_APPEND|os.O_WRONLY, 0644)
		defer f.Close()
		f.WriteString(msg)
	}

	if verbose {
		fmt.Println(msg)
	}
}

func makeStamp() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}
