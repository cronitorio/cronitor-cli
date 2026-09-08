package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// New credential-bearing files use owner-only mode 0600 on Unix; Windows
// applies an owner-restricted ACL instead. Existing files keep whatever
// access they already have unless the caller asks to restrict them.
const configModeOwnerOnly os.FileMode = 0600

// configMigrationGuidance is appended to persist errors (directory/write
// failures). This text is for when the write itself cannot complete.
const configMigrationGuidance = `Use --config or CRONITOR_CONFIG if you need a user-writable config file.
Preferred: inject CRONITOR_API_KEY and CRONITOR_PING_API_KEY in the crontab or service environment (do not store keys in the JSON file).
On Windows, set those variables on the scheduled task or Windows service.`

// configSharedReadableNotice is printed to stderr after a save leaves a
// credential file readable by users other than its owner. It never changes
// the file; it tells the operator how to, and what jobs need if they do.
const configSharedReadableNotice = `Note: this configuration file is readable by other users on this host.
It may contain your API key. To make it owner-only run:
  cronitor configure --restrict
Jobs that run as another user then need CRONITOR_API_KEY (and CRONITOR_PING_API_KEY if set) in their crontab or service environment, or their own file via --config / CRONITOR_CONFIG.`

// configNewFileNotice is printed to stderr once, when a credential file is
// created. New files are owner-only, which differs from the world-readable
// files older releases wrote, so an operator with jobs on other accounts
// learns about it at the moment of creation rather than from a failing job.
const configNewFileNotice = `Note: created this configuration file owner-only (mode 0600), so other users on this host cannot read it.
If jobs run as another user, give them CRONITOR_API_KEY (and CRONITOR_PING_API_KEY if set) in their crontab or service environment, point them at their own file with --config / CRONITOR_CONFIG, or chmod this file yourself to share it.`

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

// persistConfigFile writes credential-bearing Cronitor JSON using an atomic
// replace. A new file is created owner-only. An existing file keeps its
// current mode (Unix) or ACL (Windows); nothing narrows access the operator
// did not ask for. Unrelated 0644 writes (debug logs, crontabs) must not use
// this helper.
func persistConfigFile(path string, data []byte) error {
	return persistConfigFileMode(path, data, false)
}

// persistConfigFileMode is persistConfigFile with an explicit choice: when
// restrict is true the written file is owner-only even if it already existed
// with broader access.
func persistConfigFileMode(path string, data []byte, restrict bool) error {
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
	if exists {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write configuration through unexpected symlink %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration path %s is not a regular file", path)
		}
	}

	ownerOnly := !exists || restrict
	if err := atomicWriteConfigFile(path, data, info, exists, ownerOnly); err != nil {
		return err
	}
	switch {
	case !exists:
		fmt.Fprintf(os.Stderr, "\n%s\n%s\n\n", path, configNewFileNotice)
	case !ownerOnly && configReadableByOthers(path, info):
		fmt.Fprintf(os.Stderr, "\n%s\n%s\n\n", path, configSharedReadableNotice)
	}
	return nil
}

func atomicWriteConfigFile(path string, data []byte, existing os.FileInfo, exists, ownerOnly bool) error {
	if exists {
		return updateConfigFileInPlace(path, data, ownerOnly)
	}
	return createConfigFileOwnerOnly(path, data)
}

// createConfigFileOwnerOnly writes a brand-new credential file through an
// owner-only temp file and an atomic rename, so the key is never visible in a
// file with broader access and a crash leaves no partial file behind.
func createConfigFileOwnerOnly(path string, data []byte) error {
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

	// Restrict the temp file before the first byte of the secret is written.
	// CreateTemp already gives 0600 on Unix; Windows needs the DACL applied.
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

// updateConfigFileInPlace rewrites an existing credential file through its
// own inode, the way ioutil.WriteFile did before this helper existed. Owner,
// group, mode bits, ACLs, and extended attributes are untouched because the
// file is never replaced. Only --restrict changes access, after the write.
func updateConfigFileInPlace(path string, data []byte, restrict bool) error {
	if persistTestHook != nil {
		if err := persistTestHook(); err != nil {
			return wrapPersistWriteError(path, err)
		}
	}

	f, err := persistOpenExisting(path)
	if err != nil {
		return wrapPersistWriteError(path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return wrapPersistWriteError(path, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return wrapPersistWriteError(path, err)
	}
	if err := f.Close(); err != nil {
		return wrapPersistWriteError(path, err)
	}

	if restrict {
		if err := persistLockdownNewFile(path); err != nil {
			return fmt.Errorf("configuration saved to %s, but it could not be made owner-only: %w", path, err)
		}
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
