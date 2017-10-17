package cmd

import (
	"fmt"
	"os"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sync"
	"net/http"
	"time"
)

var cfgFile string
var apiKey string
var verbose bool
var version string
var defaultConfigFile string

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cronitor",
	Short: "Command line tools for cronitor.io",
	Long: ``,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
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
	version = "0.1.0"
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", cfgFile, "config file (default is .cronitor.json)")
	RootCmd.PersistentFlags().StringVarP(&apiKey,"api-key", "k", apiKey, "Cronitor API Key")
	RootCmd.PersistentFlags().BoolVarP(&verbose,"verbose", "v", false, "Verbose output")

	viper.BindPFlag("CRONITOR-API-KEY", RootCmd.PersistentFlags().Lookup("api-key"))
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
	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Println("Reading config from", viper.ConfigFileUsed())
	}
}

func sendPing(endpoint string, uniqueIdentifier string, group *sync.WaitGroup) {
	if verbose {
		fmt.Printf("Sending %s ping", endpoint)
	}

	Client := &http.Client{
		Timeout: time.Second * 3,
	}

	for i:=1; i<=6; i++  {
		// Determine the ping API host. After a few failed attempts, try using cronitor.io instead
		var host string
		if i > 2 && host == "cronitor.link" {
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
