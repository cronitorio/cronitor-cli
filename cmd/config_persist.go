package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Allowed credential-file modes on Unix. New files are created as 0600.
// An existing 0640 file is preserved so an operator can share the file with
// a dedicated cron group. Anything world-readable (0644+) is rejected.
const (
	configModeOwnerOnly     os.FileMode = 0600
	configModeGroupReadable os.FileMode = 0640
)

// configMigrationGuidance is appended to persist errors so operators can move
// off world-readable credential files without Cronitor silently chmod'ing a
// shared system config (which would break non-root cron).
const configMigrationGuidance = `Cronitor no longer writes world-readable credential files. Allowed file modes are 0600 or 0640.
Supported options:
  1. Preferred: inject CRONITOR_API_KEY and CRONITOR_PING_API_KEY in the crontab or service environment (do not store keys in the JSON file).
  2. Shared file: set --config or CRONITOR_CONFIG to a file that is mode 0640 and group-readable by a group the cron user is in. After root creates a 0600 file, run chgrp <cron-group> and chmod 0640 on that file.
  3. Per-user: use --config or CRONITOR_CONFIG with a user-owned 0600 file for jobs that run as that user.

Specify an alternate config file using the --config argument or the CRONITOR_CONFIG environment variable.`

// persistTestHook is invoked after the temp file is fully written and closed,
// immediately before the atomic rename. Tests set this to simulate a failure
// that must leave the previous valid file in place.
var persistTestHook func() error

// cronitorEnvAllowlist is the only environment variable names that may appear
// in verbose configure output. Values are never printed.
var cronitorEnvAllowlist = []string{
	"CRONITOR_API_KEY",
	"CRONITOR_PING_API_KEY",
	"CRONITOR_CONFIG",
	"CRONITOR_EXCLUDE_TEXT",
	"CRONITOR_HOSTNAME",
	"CRONITOR_LOG",
	"CRONITOR_ENV",
	"CRONITOR_DASH_USER",
	"CRONITOR_DASH_PASS",
	"CRONITOR_ALLOWED_IPS",
	"CRONITOR_USERS",
	"CRONITOR_API_VERSION",
	"CRONITOR_CORS_ALLOWED_ORIGINS",
	"CRONITOR_MCP_ENABLED",
}

func isAllowedConfigMode(perm os.FileMode) bool {
	return perm == configModeOwnerOnly || perm == configModeGroupReadable
}

func configOverlyPermissiveError(path string, perm os.FileMode) error {
	return fmt.Errorf("the configuration file %s has overly permissive permissions (%#o); allowed modes are 0600 or 0640.\n\n%s", path, perm, configMigrationGuidance)
}

func wrapPersistWriteError(path string, err error) error {
	return fmt.Errorf("the configuration file %s could not be written: %w\n\n%s", path, err, configMigrationGuidance)
}

func wrapPersistDirError(dir string, err error) error {
	return fmt.Errorf("the configuration directory %s could not be created: %w\n\n%s", dir, err, configMigrationGuidance)
}

// persistConfigFile writes credential-bearing Cronitor JSON using an atomic
// replace and restricted permissions. Unrelated 0644 writes (debug logs,
// crontabs) must not use this helper.
func persistConfigFile(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("configuration file path is empty\n\n%s", configMigrationGuidance)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return wrapPersistDirError(dir, err)
	}

	info, err := os.Lstat(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return wrapPersistWriteError(path, err)
	}

	mode := configModeOwnerOnly
	if exists {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write configuration through unexpected symlink %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration path %s is not a regular file", path)
		}
		if err := checkExistingConfigPerms(path, info); err != nil {
			return err
		}
		mode = existingConfigMode(info)
	}

	return atomicWriteConfigFile(path, data, mode, info, exists)
}

func atomicWriteConfigFile(path string, data []byte, mode os.FileMode, existing os.FileInfo, exists bool) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".cronitor-config-*.tmp")
	if err != nil {
		return wrapPersistWriteError(path, err)
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		_ = tmp.Close()
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()

	// Temp files start owner-only. Never write secrets into a world-readable temp.
	if err := persistApplyMode(tmp, tmpName, configModeOwnerOnly); err != nil {
		return wrapPersistWriteError(path, err)
	}

	if _, err := tmp.Write(data); err != nil {
		return wrapPersistWriteError(path, err)
	}
	if err := tmp.Sync(); err != nil {
		return wrapPersistWriteError(path, err)
	}
	if err := tmp.Close(); err != nil {
		return wrapPersistWriteError(path, err)
	}

	if mode != configModeOwnerOnly {
		if err := persistApplyPathMode(tmpName, mode); err != nil {
			return wrapPersistWriteError(path, err)
		}
	}

	if exists {
		if err := persistPreserveSecurity(tmpName, path, existing); err != nil {
			return wrapPersistWriteError(path, err)
		}
	} else {
		if err := persistLockdownNewFile(tmpName); err != nil {
			return wrapPersistWriteError(path, err)
		}
	}

	if persistTestHook != nil {
		if err := persistTestHook(); err != nil {
			return wrapPersistWriteError(path, err)
		}
	}

	if err := persistReplaceFile(tmpName, path); err != nil {
		return wrapPersistWriteError(path, err)
	}
	renamed = true
	return nil
}

func configFromViper() ConfigFile {
	configData := ConfigFile{}
	configData.ApiKey = viper.GetString(varApiKey)
	configData.PingApiAuthKey = viper.GetString(varPingApiKey)
	configData.ExcludeText = viper.GetStringSlice(varExcludeText)
	configData.Hostname = viper.GetString(varHostname)
	configData.Log = viper.GetString(varLog)
	configData.Env = viper.GetString(varEnv)
	configData.DashUsername = viper.GetString(varDashUsername)
	configData.DashPassword = viper.GetString(varDashPassword)
	configData.AllowedIPs = viper.GetString(varAllowedIPs)
	configData.CorsAllowedOrigins = viper.GetString("CRONITOR_CORS_ALLOWED_ORIGINS")
	configData.Users = viper.GetString(varUsers)
	configData.ApiVersion = viper.GetString(varApiVersion)
	configData.MCPEnabled = viper.GetBool(varMCPEnabled)

	if viper.IsSet("mcp_instances") {
		rawInstances := viper.GetStringMap("mcp_instances")
		configData.MCPInstances = make(map[string]MCPInstanceConfig)
		for name, rawConfig := range rawInstances {
			if configMap, ok := rawConfig.(map[string]interface{}); ok {
				instance := MCPInstanceConfig{}
				if url, ok := configMap["url"].(string); ok {
					instance.URL = url
				}
				if username, ok := configMap["username"].(string); ok {
					instance.Username = username
				}
				if password, ok := configMap["password"].(string); ok {
					instance.Password = password
				}
				configData.MCPInstances[name] = instance
			}
		}
	}

	return configData
}

func persistCurrentConfig() error {
	b, err := json.MarshalIndent(configFromViper(), "", "    ")
	if err != nil {
		return err
	}
	return persistConfigFile(configFilePath(), b)
}

func saveSignupCredentials(respApiKey, respPingApiKey string) error {
	viper.Set(varApiKey, respApiKey)
	viper.Set(varPingApiKey, respPingApiKey)
	return persistCurrentConfig()
}

func formatSignupPersistError(writeErr error) error {
	if writeErr == nil {
		return fmt.Errorf("your account was created but credentials could not be saved. Sign in at https://cronitor.io or retry authorization. Use --config or CRONITOR_CONFIG if you need a user-writable config file.\n\n%s", configMigrationGuidance)
	}
	return fmt.Errorf("%w\n\nyour account was created but credentials could not be saved. Sign in at https://cronitor.io or retry authorization. Use --config or CRONITOR_CONFIG if you need a user-writable config file.\n\n%s", writeErr, configMigrationGuidance)
}

func signupSuccessClientMessage() map[string]string {
	return map[string]string{
		"status":  "ok",
		"message": "Account created. Credentials have been saved.",
	}
}

func secretPresenceLabel(value string) string {
	if value == "" {
		return "Not Set"
	}
	return "Set"
}

func printCronitorEnvSources() {
	fmt.Println("\nEnvironment Variables:")
	for _, name := range cronitorEnvAllowlist {
		if _, ok := os.LookupEnv(name); ok {
			fmt.Printf("%s: Set\n", name)
		} else {
			fmt.Printf("%s: Not Set\n", name)
		}
	}
}
