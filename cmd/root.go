package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"

	"github.com/fatih/color"
	"github.com/getsentry/raven-go"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var Version string = "31.0"

var cfgFile string
var userAgent string

// Flags that are either global or used in multiple commands
var apiKey string
var environment string
var debugLog string
var dev bool
var hostname string
var pingApiKey string
var verbose bool
var noStdoutPassthru bool

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "cronitor",
	Short: shortDescription(Version),
	Long: shortDescription(Version) + `

Command line tools for Cronitor.io. See https://cronitor.io/docs/using-cronitor-cli for details.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fatal(err.Error(), 1)
	}
}

var varApiKey = "CRONITOR_API_KEY"
var varEnv = "CRONITOR_ENV"
var varHostname = "CRONITOR_HOSTNAME"
var varLog = "CRONITOR_LOG"
var varPingApiKey = "CRONITOR_PING_API_KEY"
var varExcludeText = "CRONITOR_EXCLUDE_TEXT"
var varConfig = "CRONITOR_CONFIG"
var varDashUsername = "CRONITOR_DASH_USER"
var varDashPassword = "CRONITOR_DASH_PASS"
var varAllowedIPs = "CRONITOR_ALLOWED_IPS"

func init() {
	userAgent = fmt.Sprintf("CronitorCLI/%s", Version)
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	RootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", cfgFile, "Config file")
	RootCmd.PersistentFlags().StringVar(&environment, "env", environment, "Cronitor Environment")
	RootCmd.PersistentFlags().StringVarP(&apiKey, "api-key", "k", apiKey, "Cronitor API Key")
	RootCmd.PersistentFlags().StringVarP(&pingApiKey, "ping-api-key", "p", pingApiKey, "Ping API Key")
	RootCmd.PersistentFlags().StringVarP(&hostname, "hostname", "n", hostname, "A unique identifier for this host (default: system hostname)")
	RootCmd.PersistentFlags().StringVarP(&debugLog, "log", "l", debugLog, "Write debug logs to supplied file")
	RootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", verbose, "Verbose output")

	RootCmd.PersistentFlags().BoolVar(&dev, "use-dev", dev, "Dev mode")
	RootCmd.PersistentFlags().MarkHidden("use-dev")

	viper.BindPFlag(varApiKey, RootCmd.PersistentFlags().Lookup("api-key"))
	viper.BindPFlag(varEnv, RootCmd.PersistentFlags().Lookup("env"))
	viper.BindPFlag(varHostname, RootCmd.PersistentFlags().Lookup("hostname"))
	viper.BindPFlag(varLog, RootCmd.PersistentFlags().Lookup("log"))
	viper.BindPFlag(varPingApiKey, RootCmd.PersistentFlags().Lookup("ping-api-key"))
	viper.BindPFlag(varConfig, RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag(varDashUsername, RootCmd.PersistentFlags().Lookup("dash-username"))
	viper.BindPFlag(varDashPassword, RootCmd.PersistentFlags().Lookup("dash-password"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {

	viper.AutomaticEnv() // read in environment variables that match
	configFile := viper.GetString(varConfig)

	// If a custom config file is specified by flag or env var, use it. Otherwise use default file.
	if len(configFile) > 0 {
		if len(configFile) < 5 || strings.ToLower(configFile[len(configFile)-5:]) != ".json" {
			fmt.Println("Error: Config file must be a .json file")
		}
		viper.SetConfigFile(configFile)
	} else {
		viper.AddConfigPath(defaultConfigFileDirectory())
		viper.SetConfigName("cronitor")
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		log("Reading config from " + viper.ConfigFileUsed())
	}
}

func sendPing(endpoint string, uniqueIdentifier string, message string, series string, timestamp float64, duration *float64, exitCode *int, metrics map[string]int, schedule string, group *sync.WaitGroup) {
	defer group.Done()

	Client := &http.Client{
		Timeout: time.Second * 10,
	}

	hostname := effectiveHostname()
	pingApiAuthKey := viper.GetString(varPingApiKey)
	apiKey := viper.GetString(varApiKey)
	env := viper.GetString(varEnv)
	pingApiHost := ""
	authenticationKey := ""
	formattedStamp := ""
	formattedDuration := ""
	formattedStatusCode := ""
	formattedMetrics := ""
	formattedSchedule := ""

	if timestamp > 0 {
		formattedStamp = fmt.Sprintf("&stamp=%s", formatStamp(timestamp))
	}

	if len(message) > 0 {
		message = fmt.Sprintf("&msg=%s", url.QueryEscape(truncateString(message, 1000)))
	}

	if len(hostname) > 0 {
		hostname = fmt.Sprintf("&host=%s", url.QueryEscape(truncateString(hostname, 50)))
	}

	if len(env) > 0 {
		env = fmt.Sprintf("&env=%s", url.QueryEscape(truncateString(env, 50)))
	}

	// By passing duration up, we save the computation on the server side
	if duration != nil {
		formattedDuration = fmt.Sprintf("&duration=%s", formatStamp(*duration))
	}

	if schedule != "" {
		formattedSchedule = fmt.Sprintf("&schedule=%s", schedule)
	}

	// We aren't using exit code at time of writing, but we have the field available for healthcheck monitors.
	if exitCode != nil {
		formattedStatusCode = fmt.Sprintf("&status_code=%d", *exitCode)
	}

	// The `series` data is used to match run events with complete or fail. Useful if multiple instances of a job are running.
	if len(series) > 0 {
		series = fmt.Sprintf("&series=%s", series)
	}

	if metrics != nil && len(metrics) > 0 {
		values := url.Values{}
		for key, element := range metrics {
			values.Add("metric", fmt.Sprintf("%s:%d", key, element))
		}
		formattedMetrics = "&" + values.Encode()
	}

	// If a user has set a key specifically for pings, use it.
	if len(pingApiAuthKey) > 0 {
		authenticationKey = pingApiAuthKey
	} else {
		authenticationKey = apiKey
	}

	// If we don't have any authentication key we will need to send an unauthenticated ping.
	// This requires that we have a GUID "monitor code" not a per-user "monitor key"
	if len(authenticationKey) == 0 {
		monitorCodeRegex := regexp.MustCompile(`^[A-Za-z0-9]{3,12}$`)
		if ret := monitorCodeRegex.FindStringSubmatch(uniqueIdentifier); ret == nil {
			log("Cannot send ping: you must provide a valid API key with this command or save a key using 'cronitor configure'")
			return
		}
	}

	pingSent := false
	uri := ""
	for i := 1; i <= 6; i++ {
		if dev {
			pingApiHost = "http://localhost:8000"
		} else if i > 2 && pingApiHost == "https://cronitor.link" {
			pingApiHost = "https://cronitor.io"
		} else {
			pingApiHost = "https://cronitor.link"
		}

		// After 2 failed attempts, take a brief random break before trying again
		if i > 2 {
			time.Sleep(time.Second * time.Duration(float32(i)*1.5*rand.Float32()))
		}

		if len(authenticationKey) > 0 {
			// Authenticated pings when available
			uri = fmt.Sprintf("%s/ping/%s/%s?state=%s&try=%d%s%s%s%s%s%s%s%s%s", pingApiHost, authenticationKey, uniqueIdentifier, endpoint, i, formattedStamp, message, hostname, formattedDuration, series, formattedStatusCode, formattedMetrics, env, formattedSchedule)
		} else {
			// Fallback to sending an unauthenticated ping
			uri = fmt.Sprintf("%s/%s/%s?try=%d%s%s%s%s%s%s%s%s%s", pingApiHost, uniqueIdentifier, endpoint, i, formattedStamp, message, hostname, formattedDuration, series, formattedStatusCode, formattedMetrics, env, formattedSchedule)
		}

		log("Sending ping " + uri)

		request, _ := http.NewRequest("GET", uri, nil)
		request.Header.Add("User-Agent", userAgent)
		response, err := Client.Do(request)

		if err != nil {
			log(err.Error())
			continue
		}

		_, err = ioutil.ReadAll(response.Body)
		response.Body.Close()

		// Any 2xx is considered a successful response
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			pingSent = true
			break
		}

		// Backoff on any 4xx request, e.g. 429 Too Many Requests
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			pingSent = false
			break
		}
	}

	if !pingSent {
		raven.CaptureErrorAndWait(errors.New("Ping failure; retries exhausted: "+uri), nil)
	}
}

func effectiveHostname() string {
	if len(viper.GetString(varHostname)) > 0 {
		return viper.GetString(varHostname)
	}

	hostname, _ := os.Hostname()
	return hostname
}

func effectiveTimezoneLocationName() lib.TimezoneLocationName {

	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-NoProfile", "-c", "(Get-TimeZone | Select-Object -First 1 -Property Id).Id | Write-Output").CombinedOutput()
		if err == nil {
			return lib.TimezoneLocationName{strings.TrimSpace(fmt.Sprintf("%s", out))}
		}
	}

	// First, check if a TZ or CRON_TZ environemnt variable is set -- Diff var used by diff distros
	if locale, isSetFlag := os.LookupEnv("TZ"); isSetFlag {
		return lib.TimezoneLocationName{locale}
	}

	if locale, isSetFlag := os.LookupEnv("CRON_TZ"); isSetFlag {
		return lib.TimezoneLocationName{locale}
	}

	// Attempt to parse timedatectl (should work on FreeBSD, many linux distros)
	if output, err := exec.Command("timedatectl").Output(); err == nil {
		outputString := strings.Replace(string(output), "Time zone", "Timezone", -1)
		r := regexp.MustCompile(`(?m:Timezone:\s+(\S+).+$)`)
		if ret := r.FindStringSubmatch(outputString); ret != nil && len(ret) > 1 {
			return lib.TimezoneLocationName{ret[1]}
		}
	}

	// If /etc/localtime is a symlink, check what it is linking to
	if localtimeFile, err := os.Lstat("/etc/localtime"); err == nil && localtimeFile.Mode()&os.ModeSymlink == os.ModeSymlink {
		if symlink, _ := os.Readlink("/etc/localtime"); len(symlink) > 0 {
			if strings.Contains(symlink, "UTC") {
				return lib.TimezoneLocationName{"UTC"}
			}

			symlinkParts := strings.Split(symlink, "/")
			return lib.TimezoneLocationName{strings.Join(symlinkParts[len(symlinkParts)-2:], "/")}
		}
	}

	// If we happen to have an /etc/timezone, no guarantee it's used, but read that
	if locale, err := ioutil.ReadFile("/etc/timezone"); err == nil {
		return lib.TimezoneLocationName{string(locale)}
	}

	return lib.TimezoneLocationName{""}
}

func defaultConfigFileDirectory() string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("%s\\ProgramData\\Cronitor", os.Getenv("SYSTEMDRIVE"))
	}

	return "/etc/cronitor"
}

func truncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}

	return s[:length]
}

func printSuccessText(message string, indent bool) {
	if isAutoDiscover || isSilent {
		log(message)
	} else {
		color := color.New(color.FgHiGreen)

		if indent {
			color.Println(fmt.Sprintf(" |--► %s", message))
		} else {
			color.Println(fmt.Sprintf("----► %s", message))
		}
	}
}

func printDoneText(message string, indent bool) {
	if isAutoDiscover || isSilent {
		log(message)
	} else {
		printSuccessText(message+" ✔", indent)
	}
}

func printWarningText(message string, indent bool) {
	if isAutoDiscover || isSilent {
		log(message)
	} else {
		color := color.New(color.FgHiYellow)

		if indent {
			color.Println(fmt.Sprintf(" |--► %s", message))
		} else {
			color.Println(fmt.Sprintf("----► %s", message))
		}
	}
}

func printErrorText(message string, indent bool) {
	if isAutoDiscover || isSilent {
		log(message)
	} else {
		red := color.New(color.FgHiRed)
		if indent {
			red.Println(fmt.Sprintf(" |--► %s", message))
		} else {
			red.Println(fmt.Sprintf("----► %s", message))
		}
	}
}

func printLn() {
	if isAutoDiscover || isSilent {
		return
	}

	fmt.Println()
}

func isPathToDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fileInfo.Mode().IsDir()
}

func log(msg string) {
	debugLog := viper.GetString(varLog)
	if len(debugLog) > 0 {
		f, _ := os.OpenFile(debugLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		defer f.Close()
		f.WriteString(msg + "\n")
	}

	if verbose {
		fmt.Println(msg)
	}
}

func fatal(msg string, exitCode int) {
	debugLog := viper.GetString(varLog)
	if len(debugLog) > 0 {
		f, _ := os.OpenFile(debugLog, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		defer f.Close()
		f.WriteString(msg + "\n")
	}

	fmt.Fprintln(os.Stderr, msg)
	os.Exit(exitCode)
}

func makeStamp() float64 {
	return float64(time.Now().UnixNano()) / float64(time.Second)
}

func formatStamp(timestamp float64) string {
	return strconv.FormatFloat(timestamp, 'f', 3, 64)
}

func shortDescription(version string) string {
	return fmt.Sprintf("CronitorCLI version %s", version)
}

func getCronitorApi() *lib.CronitorApi {
	return &lib.CronitorApi{
		IsDev:          dev,
		IsAutoDiscover: isAutoDiscover,
		ApiKey:         viper.GetString(varApiKey),
		UserAgent:      userAgent,
		Logger:         log,
	}
}
