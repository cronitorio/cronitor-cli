package cmd

import (
	"os"
	"sync"

	"github.com/cronitorio/cronitor-cli/lib"
)

// Exported for internal/dashboard, which builds on the CLI's configuration,
// API client, and telemetry. The dashboard imports cmd; cmd never imports the
// dashboard.

var (
	VarApiKey       = varApiKey
	VarPingApiKey   = varPingApiKey
	VarEnv          = varEnv
	VarHostname     = varHostname
	VarLog          = varLog
	VarExcludeText  = varExcludeText
	VarDashUsername = varDashUsername
	VarDashPassword = varDashPassword
	VarAllowedIPs   = varAllowedIPs
	VarUsers        = varUsers
	VarMCPEnabled   = varMCPEnabled
)

func Log(msg string)                                   { log(msg) }
func Fatal(msg string, exitCode int)                   { fatal(msg, exitCode) }
func CronitorApi() *lib.CronitorApi                    { return getCronitorApi() }
func ParseUsers() []string                             { return parseUsers() }
func ConfigFilePath() string                           { return configFilePath() }
func PersistConfigFile(path string, data []byte) error { return persistConfigFile(path, data) }
func MakeStamp() float64                               { return makeStamp() }
func FormatStamp(timestamp float64) string             { return formatStamp(timestamp) }
func GetFileSize(f *os.File) (int64, error)            { return getFileSize(f) }
func MakeCronLikeEnv() []string                        { return makeCronLikeEnv() }
func OpenBrowser(url string)                           { openBrowser(url) }
func EffectiveHostname() string                        { return effectiveHostname() }
func CheckForUpdate() (*UpdateStatus, error)           { return checkForUpdate() }
func PerformUpdate(status *UpdateStatus) error         { return performUpdate(status) }

func EffectiveTimezoneLocationName() lib.TimezoneLocationName {
	return effectiveTimezoneLocationName()
}

func CreateDefaultName(line *lib.Line, crontab *lib.Crontab, hostname string, excludeFromName []string, candidates map[string]bool) string {
	return createDefaultName(line, crontab, hostname, excludeFromName, candidates)
}

func SendPing(endpoint, uniqueIdentifier, message, series string, timestamp float64, duration *float64, exitCode *int, metrics map[string]int, schedule string, group *sync.WaitGroup) {
	sendPing(endpoint, uniqueIdentifier, message, series, timestamp, duration, exitCode, metrics, schedule, group)
}

func SaveSignupCredentials(apiKey, pingApiKey string) error {
	return saveSignupCredentials(apiKey, pingApiKey)
}

func FormatSignupPersistError(writeErr error) error { return formatSignupPersistError(writeErr) }
func SignupSuccessClientMessage() map[string]string { return signupSuccessClientMessage() }
