package dashboard

import "github.com/cronitorio/cronitor-cli/cmd"

// The dashboard shares the CLI's configuration, API client, and telemetry.
// These names keep the dashboard code as it was when it lived in cmd.

type (
	ConfigFile        = cmd.ConfigFile
	MCPInstanceConfig = cmd.MCPInstanceConfig
	UpdateStatus      = cmd.UpdateStatus
)

// Version is the CLI release the dashboard is built with.
var Version = cmd.Version

var (
	varApiKey       = cmd.VarApiKey
	varPingApiKey   = cmd.VarPingApiKey
	varEnv          = cmd.VarEnv
	varHostname     = cmd.VarHostname
	varLog          = cmd.VarLog
	varExcludeText  = cmd.VarExcludeText
	varDashUsername = cmd.VarDashUsername
	varDashPassword = cmd.VarDashPassword
	varAllowedIPs   = cmd.VarAllowedIPs
	varUsers        = cmd.VarUsers
	varMCPEnabled   = cmd.VarMCPEnabled
)

var (
	log                           = cmd.Log
	fatal                         = cmd.Fatal
	getCronitorApi                = cmd.CronitorApi
	sendPing                      = cmd.SendPing
	parseUsers                    = cmd.ParseUsers
	effectiveTimezoneLocationName = cmd.EffectiveTimezoneLocationName
	effectiveHostname             = cmd.EffectiveHostname
	createDefaultName             = cmd.CreateDefaultName
	configFilePath                = cmd.ConfigFilePath
	persistConfigFile             = cmd.PersistConfigFile
	makeStamp                     = cmd.MakeStamp
	formatStamp                   = cmd.FormatStamp
	getFileSize                   = cmd.GetFileSize
	makeCronLikeEnv               = cmd.MakeCronLikeEnv
	openBrowser                   = cmd.OpenBrowser
	checkForUpdate                = cmd.CheckForUpdate
	performUpdate                 = cmd.PerformUpdate
	saveSignupCredentials         = cmd.SaveSignupCredentials
	formatSignupPersistError      = cmd.FormatSignupPersistError
	signupSuccessClientMessage    = cmd.SignupSuccessClientMessage
)
