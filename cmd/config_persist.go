package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// New credential-bearing files, and every successful rewrite, use owner-only
// mode 0600 on Unix. Windows applies an owner-restricted ACL instead.
const configModeOwnerOnly os.FileMode = 0600

// configMigrationGuidance is appended to persist errors (directory/write
// failures). Existing 0644 files are not rejected; they are rewritten as 0600
// on the next save. This text is for when the write itself cannot complete.
const configMigrationGuidance = `Use --config or CRONITOR_CONFIG if you need a user-writable config file.
Preferred: inject CRONITOR_API_KEY and CRONITOR_PING_API_KEY in the crontab or service environment (do not store keys in the JSON file).
On Windows, set those variables on the scheduled task or Windows service. A later save rewrites the JSON file as owner-only.`

// configAccessNarrowedWarning is printed to stderr after a successful save
// that tightens a previously shared-readable credential file to owner-only.
const configAccessNarrowedWarning = `WARNING: Cronitor rewrote the configuration file as owner-only (Unix mode 0600; owner-restricted ACL on Windows).
Other users will lose read access.

If jobs run as another account:
  1. Preferred: set CRONITOR_API_KEY and CRONITOR_PING_API_KEY in the crontab or service environment (not in the JSON file). On Windows, set them on the scheduled task or Windows service.
  2. Shared file: after this save you may chmod 0640 and chgrp a dedicated group (Unix), or grant that service account read on the file ACL (Windows), then point --config / CRONITOR_CONFIG at that file. The next Cronitor save will set owner-only again.
  3. Per-user: use --config or CRONITOR_CONFIG with a user-owned 0600 file for jobs that run as that user.

Existing world-readable files keep working for exec/ping until someone saves.`

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

func wrapPersistWriteError(path string, err error) error {
	return fmt.Errorf("the configuration file %s could not be written: %w\n\n%s", path, err, configMigrationGuidance)
}

func wrapPersistDirError(dir string, err error) error {
	return fmt.Errorf("the configuration directory %s could not be created: %w\n\n%s", dir, err, configMigrationGuidance)
}

func printConfigAccessWarning(path string) {
	fmt.Fprintf(os.Stderr, "\nWARNING: %s\n%s\n\n", path, configAccessNarrowedWarning)
}

// persistConfigFile writes credential-bearing Cronitor JSON using an atomic
// replace and owner-only permissions. Existing 0644/0640 files are rewritten
// as 0600 (with a warning). Unrelated 0644 writes (debug logs, crontabs)
// must not use this helper.
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

	warnNarrow := false
	if exists {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write configuration through unexpected symlink %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration path %s is not a regular file", path)
		}
		warnNarrow = existingAccessWillNarrow(path, info)
	}

	if err := atomicWriteConfigFile(path, data, info, exists); err != nil {
		return err
	}
	if warnNarrow {
		printConfigAccessWarning(path)
	}
	return nil
}

func atomicWriteConfigFile(path string, data []byte, existing os.FileInfo, exists bool) error {
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

	if exists {
		if err := persistPreserveOwner(tmpName, existing); err != nil {
			return wrapPersistWriteError(path, err)
		}
	}
	if err := persistLockdownNewFile(tmpName); err != nil {
		return wrapPersistWriteError(path, err)
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

	// Re-apply owner-only after replace so Windows dest ACLs cannot linger.
	if err := persistLockdownNewFile(path); err != nil {
		return wrapPersistWriteError(path, err)
	}
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
