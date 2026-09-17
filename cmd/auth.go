package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	varAuthManaged           = "CRONITOR_AUTH_MANAGED"
	varMachineCredentialName = "CRONITOR_MACHINE_CREDENTIAL_NAME"
	authLoginTimeoutDefault  = 0
)

var (
	authYes       bool
	authNoBrowser bool
	authTimeout   string
	authForce     bool
)

const authLoginLong = `Start WorkOS device authorization, then exchange the approved session
for a Cronitor machine credential stored in the resolved config file.

The user code and verification URL are printed. The device polling code,
access token, and API key are never printed.`

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Log in, show status, or log out",
	Long: `Authenticate CronitorCLI with WorkOS device authorization.

Login opens a browser (or prints a verification URL), shows a short user
code, and after you approve the request stores a host machine credential
as CRONITOR_API_KEY. WorkOS tokens are used once to create that credential
and are then discarded. They are never refreshed or written to disk.

  cronitor auth login
  cronitor signup        (alias for auth login)
  cronitor auth status
  cronitor auth logout`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var authLoginCmd = &cobra.Command{
	Use:     "login",
	Aliases: []string{"signup"},
	Short:   "Log in and install a machine credential",
	Long:    authLoginLong + "\n\ncronitor signup is an alias for this command.",
	RunE:    runAuthLoginE,
}

// signupCmd is a top-level alias for auth login. It shares RunE and flags so
// `cronitor signup` is the same device-auth path — no separate TUI or
// website sign-up key mint.
var signupCmd = &cobra.Command{
	Use:   "signup",
	Short: "Alias for auth login",
	Long:  "signup is an alias for auth login.\n\n" + authLoginLong,
	RunE:  runAuthLoginE,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current machine credential",
	Long:  `Show metadata for the current machine credential. The API key is never printed.`,
	Run: func(cmd *cobra.Command, args []string) {
		runAuthStatus()
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoke the machine credential and remove it locally",
	Long: `Revoke the machine credential stored in the resolved config file
and remove CRONITOR_API_KEY, CRONITOR_AUTH_MANAGED, and
CRONITOR_MACHINE_CREDENTIAL_NAME. Other settings are kept.

Logout reads the stored key and ownership metadata together from the config
file. An explicit --api-key or CRONITOR_API_KEY that differs from the stored
key is refused so a session override cannot revoke one credential and erase
another. Other commands keep the usual flag/env/config precedence.`,
	Run: func(cmd *cobra.Command, args []string) {
		runAuthLogout()
	},
}

func init() {
	RootCmd.AddCommand(authCmd)
	RootCmd.AddCommand(signupCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authLogoutCmd)

	addAuthLoginFlags(authLoginCmd)
	addAuthLoginFlags(signupCmd)

	authLogoutCmd.Flags().BoolVar(&authYes, "yes", false, "Revoke without prompting")
	authLogoutCmd.Flags().BoolVar(&authForce, "force", false, "Remove the local API key without a confirmed remote revocation, or remove a key that was not installed by auth login")
}

func addAuthLoginFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&authYes, "yes", false, "Install or replace a machine credential without prompting")
	cmd.Flags().BoolVar(&authNoBrowser, "no-browser", false, "Print the verification URL without opening a browser")
	cmd.Flags().StringVar(&authTimeout, "timeout", "", "Maximum time to wait for authorization (default: device expiry)")
}

func resetAuthLoginFlags(cmd *cobra.Command) {
	_ = cmd.Flags().Set("yes", "false")
	_ = cmd.Flags().Set("no-browser", "false")
	_ = cmd.Flags().Set("timeout", "")
}

func resetAuthFlags() {
	authYes = false
	authNoBrowser = false
	authTimeout = ""
	authForce = false
	resetAuthLoginFlags(authLoginCmd)
	resetAuthLoginFlags(signupCmd)
	_ = authLogoutCmd.Flags().Set("yes", "false")
	_ = authLogoutCmd.Flags().Set("force", "false")
}

func runAuthLoginE(cmd *cobra.Command, args []string) error {
	runAuthLogin()
	return nil
}

func resetAPIKeyFlag() {
	f := RootCmd.PersistentFlags().Lookup("api-key")
	if f == nil {
		return
	}
	_ = f.Value.Set("")
	f.Changed = false
	apiKey = ""
}

func runAuthLogin() {
	if err := confirmInstallOrReplace(); err != nil {
		failAndExit(err.Error())
		return
	}

	authKit := lib.WorkOSAuthKitURL()
	clientID := lib.WorkOSClientID()
	if clientID == "" {
		failAndExit("WorkOS Connect client_id is not configured. Set CRONITOR_WORKOS_CLIENT_ID.")
		return
	}

	device, err := lib.RequestDeviceAuthorization(authKit, clientID, lib.WorkOSScope())
	if err != nil {
		failAndExit(redactSecrets(err.Error()))
		return
	}
	rememberSecret(device.DeviceCode)

	printDeviceInstructions(device)

	openURL := device.VerificationURIComplete
	if openURL == "" {
		openURL = device.VerificationURI
	}
	if !authNoBrowser && openURL != "" {
		openBrowserFn(openURL)
	}

	timeout, err := parseAuthTimeout(authTimeout)
	if err != nil {
		failAndExit(err.Error())
		return
	}
	expiresAt := nowFn().Add(time.Duration(device.ExpiresIn) * time.Second)
	interval := lib.PollInterval(device.Interval)

	token, err := pollDeviceGrant(authKit, clientID, device.DeviceCode, interval, timeout, expiresAt)
	if err != nil {
		failAndExit(redactSecrets(err.Error()))
		return
	}
	rememberSecret(token.AccessToken)
	rememberSecret(token.RefreshToken)
	rememberSecret(token.IDToken)

	hostname := effectiveHostname()
	cred, err := lib.CreateMachineCredential(lib.APIBaseURL(dev), token.AccessToken, hostname)
	token.Discard()
	if err != nil {
		failAndExit(redactSecrets(err.Error()))
		return
	}
	rememberSecret(cred.Key)

	if err := saveAuthManagedCredential(cred.Key, cred.Name); err != nil {
		failAndExit(redactSecrets(err.Error()))
		return
	}

	printAuthLoginSuccess(cred)
}

func printDeviceInstructions(device *lib.DeviceAuthorization) {
	fmt.Println()
	fmt.Println("Open this URL in a browser and enter the code:")
	fmt.Println()
	fmt.Println("  " + device.VerificationURI)
	fmt.Println()
	fmt.Println("  Code: " + device.UserCode)
	if device.VerificationURIComplete != "" && device.VerificationURIComplete != device.VerificationURI {
		fmt.Println()
		fmt.Println("Or open:")
		fmt.Println("  " + device.VerificationURIComplete)
	}
	fmt.Println()
	Info("Waiting for authorization...")
}

func printAuthLoginSuccess(cred *lib.MachineCredential) {
	Success(fmt.Sprintf("Logged in as machine credential %s", cred.Name))
	if cred.Kind != "" {
		fmt.Printf("Kind: %s\n", cred.Kind)
	}
	if cred.Organization != "" {
		fmt.Printf("Organization: %s\n", cred.Organization)
	}
	if len(cred.Scopes) > 0 {
		fmt.Printf("Scopes: %s\n", strings.Join(cred.Scopes, ", "))
	}
	fmt.Printf("Config: %s\n", configFilePath())
}

func pollDeviceGrant(authKit, clientID, deviceCode string, interval, timeout time.Duration, expiresAt time.Time) (*lib.DeviceToken, error) {
	deadline := expiresAt
	if timeout > 0 {
		flagDeadline := nowFn().Add(timeout)
		if deadline.IsZero() || flagDeadline.Before(deadline) {
			deadline = flagDeadline
		}
	}
	if deadline.IsZero() {
		deadline = nowFn().Add(lib.DefaultDeviceExpiry)
	}

	for {
		if !nowFn().Before(deadline) {
			return nil, fmt.Errorf("device authorization expired")
		}

		token, oauthErr, err := lib.ExchangeDeviceToken(authKit, clientID, deviceCode)
		if err != nil {
			return nil, err
		}
		if token != nil {
			return token, nil
		}

		switch oauthErr {
		case "authorization_pending":
			// keep polling
		case "slow_down":
			interval += lib.SlowDownIncrement
		case "access_denied":
			return nil, fmt.Errorf("authorization denied")
		case "expired_token":
			return nil, fmt.Errorf("device authorization expired")
		case "":
			return nil, fmt.Errorf("device authorization failed")
		default:
			return nil, fmt.Errorf("device authorization failed: %s", oauthErr)
		}

		sleepForPoll(interval, deadline)
		if !nowFn().Before(deadline) {
			return nil, fmt.Errorf("device authorization expired")
		}
	}
}

func parseAuthTimeout(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return authLoginTimeoutDefault, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid --timeout %q (use a duration such as 5m)", raw)
	}
	return d, nil
}

func confirmInstallOrReplace() error {
	existing := strings.TrimSpace(viper.GetString(varApiKey))
	if existing == "" && !configHasAPIKey() {
		return nil
	}
	prompt := "Install a machine credential and save it as CRONITOR_API_KEY?"
	if viper.GetBool(varAuthManaged) {
		name := viper.GetString(varMachineCredentialName)
		if name != "" {
			prompt = fmt.Sprintf("Replace existing machine credential %q?", name)
		} else {
			prompt = "Replace the existing machine credential?"
		}
	} else if existing != "" || configHasAPIKey() {
		prompt = "A Cronitor API key is already configured. Replace it with a machine credential for this host?"
	}
	if confirmAuthAction(prompt) {
		return nil
	}
	return fmt.Errorf("login cancelled")
}

func configHasAPIKey() bool {
	return readStoredAuthFromConfig().APIKey != ""
}

// storedAuth is the API key and ownership metadata as one unit from the
// resolved config file. Destructive auth operations use this instead of
// viper's flag/env/config merge so a session override cannot revoke key B
// while clearing managed installation A.
type storedAuth struct {
	APIKey  string
	Managed bool
	Name    string
}

func readStoredAuthFromConfig() storedAuth {
	var out storedAuth
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return out
	}
	var raw map[string]interface{}
	if json.Unmarshal(data, &raw) != nil {
		return out
	}
	for k, v := range raw {
		switch {
		case strings.EqualFold(k, varApiKey):
			if s, ok := v.(string); ok {
				out.APIKey = strings.TrimSpace(s)
			}
		case strings.EqualFold(k, varAuthManaged):
			out.Managed = truthyJSON(v)
		case strings.EqualFold(k, varMachineCredentialName):
			if s, ok := v.(string); ok {
				out.Name = strings.TrimSpace(s)
			}
		}
	}
	return out
}

func truthyJSON(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "true", "yes", "on":
			return true
		}
	case float64:
		return t != 0
	case json.Number:
		n, err := t.Float64()
		return err == nil && n != 0
	}
	return false
}

// explicitAPIKeyOverride reports a key supplied by --api-key or
// CRONITOR_API_KEY, not the viper merge. Empty values are ignored.
func explicitAPIKeyOverride() (key, source string, ok bool) {
	if f := RootCmd.PersistentFlags().Lookup("api-key"); f != nil && f.Changed {
		if v := strings.TrimSpace(f.Value.String()); v != "" {
			return v, "--api-key", true
		}
	}
	if v, exists := os.LookupEnv(varApiKey); exists {
		if v = strings.TrimSpace(v); v != "" {
			return v, varApiKey, true
		}
	}
	return "", "", false
}

// clearManagedAuthMetadataIfReplacingKey drops CRONITOR_AUTH_MANAGED and
// CRONITOR_MACHINE_CREDENTIAL_NAME when configure is about to persist a
// different API key than the one stored in the file (flag or environment).
func clearManagedAuthMetadataIfReplacingKey(cmd *cobra.Command) {
	stored := readStoredAuthFromConfig()
	replacing := false
	if flag := cmd.Flags().Lookup("api-key"); flag != nil && flag.Changed {
		replacing = true
	}
	if envKey, exists := os.LookupEnv(varApiKey); exists {
		if envKey = strings.TrimSpace(envKey); envKey != "" && envKey != stored.APIKey {
			replacing = true
		}
	}
	if strings.TrimSpace(viper.GetString(varApiKey)) != stored.APIKey {
		replacing = true
	}
	if !replacing {
		return
	}
	viper.Set(varAuthManaged, false)
	viper.Set(varMachineCredentialName, "")
}

func confirmAuthAction(prompt string) bool {
	if authYes {
		return true
	}
	line, err := readLineFn(prompt + " [y/N] ")
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

func runAuthStatus() {
	apiKey := strings.TrimSpace(viper.GetString(varApiKey))
	if apiKey == "" {
		failAndExit("Not logged in")
		return
	}
	rememberSecret(apiKey)

	if !viper.GetBool(varAuthManaged) {
		fmt.Println("A Cronitor API key is configured (not managed by auth login).")
		fmt.Printf("Config: %s\n", configFilePath())
		return
	}

	cred, err := lib.GetCurrentMachineCredential(lib.APIBaseURL(dev), apiKey)
	if err != nil {
		if _, gone := err.(*lib.CredentialGoneError); gone {
			failAndExit("Machine credential is no longer valid. Run cronitor auth login.")
			return
		}
		failAndExit(redactSecrets(err.Error()))
		return
	}

	name := cred.Name
	if name == "" {
		name = viper.GetString(varMachineCredentialName)
	}
	fmt.Println("Logged in")
	if name != "" {
		fmt.Printf("Name: %s\n", name)
	}
	if cred.Kind != "" {
		fmt.Printf("Kind: %s\n", cred.Kind)
	}
	if cred.Organization != "" {
		fmt.Printf("Organization: %s\n", cred.Organization)
	}
	if len(cred.Scopes) > 0 {
		fmt.Printf("Scopes: %s\n", strings.Join(cred.Scopes, ", "))
	}
	fmt.Printf("Config: %s\n", configFilePath())
	if cred.Key != "" {
		// Defensive: the contract forbids returning the key. Never print it.
		cred.Key = ""
	}
}

func runAuthLogout() {
	stored := readStoredAuthFromConfig()
	if override, source, ok := explicitAPIKeyOverride(); ok && override != stored.APIKey {
		rememberSecret(override)
		if stored.APIKey != "" {
			rememberSecret(stored.APIKey)
		}
		if stored.APIKey != "" || stored.Managed {
			failAndExit(fmt.Sprintf("%s does not match the API key stored in the config file. Unset the override to log out of this host's machine credential.", source))
			return
		}
	}

	apiKey := stored.APIKey
	managed := stored.Managed
	name := stored.Name

	if apiKey == "" && !managed {
		fmt.Println("Not logged in")
		return
	}

	if !managed && !authForce {
		failAndExit("A Cronitor API key is configured but was not installed by auth login. Use --force to remove it from this config file without revoking it remotely.")
		return
	}

	prompt := "Log out and remove the local API key?"
	if managed && name != "" {
		prompt = fmt.Sprintf("Revoke machine credential %q and remove it from this host?", name)
	} else if managed {
		prompt = "Revoke the machine credential and remove it from this host?"
	}
	if !confirmAuthAction(prompt) {
		failAndExit("logout cancelled")
		return
	}

	if managed && apiKey != "" {
		rememberSecret(apiKey)
		err := lib.DeleteCurrentMachineCredential(lib.APIBaseURL(dev), apiKey)
		if err != nil {
			if _, gone := err.(*lib.CredentialGoneError); gone {
				if clearErr := clearAuthManagedCredential(); clearErr != nil {
					failAndExit(redactSecrets(clearErr.Error()))
					return
				}
				fmt.Println("Machine credential is no longer valid remotely. Removed local copy.")
				return
			}
			if authForce {
				if clearErr := clearAuthManagedCredential(); clearErr != nil {
					failAndExit(redactSecrets(clearErr.Error()))
					return
				}
				fmt.Println("Removed local credential. Remote revocation was not confirmed.")
				return
			}
			failAndExit(redactSecrets(err.Error()) + " Local credential was not removed.")
			return
		}
	}

	if err := clearAuthManagedCredential(); err != nil {
		failAndExit(redactSecrets(err.Error()))
		return
	}
	Success("Logged out")
}

func saveAuthManagedCredential(key, name string) error {
	if key != "" {
		rememberSecret(key)
	}
	viper.Set(varApiKey, key)
	viper.Set(varAuthManaged, true)
	viper.Set(varMachineCredentialName, name)
	return mergePersistConfig(map[string]interface{}{
		varApiKey:                key,
		varAuthManaged:           true,
		varMachineCredentialName: name,
	}, nil, true)
}

func clearAuthManagedCredential() error {
	viper.Set(varApiKey, "")
	viper.Set(varAuthManaged, false)
	viper.Set(varMachineCredentialName, "")
	return mergePersistConfig(nil, []string{varApiKey, varAuthManaged, varMachineCredentialName}, true)
}

func mergePersistConfig(set map[string]interface{}, unset []string, restrict bool) error {
	path := configFilePath()
	merged := map[string]interface{}{}
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &merged) != nil {
			merged = configMapFromFileStruct()
		}
	} else if !os.IsNotExist(err) {
		return wrapPersistWriteError(path, err)
	}

	for _, k := range unset {
		for existing := range merged {
			if strings.EqualFold(existing, k) {
				delete(merged, existing)
			}
		}
	}
	for k, v := range set {
		for existing := range merged {
			if strings.EqualFold(existing, k) && existing != k {
				delete(merged, existing)
			}
		}
		merged[k] = v
	}

	b, err := json.MarshalIndent(merged, "", "    ")
	if err != nil {
		return err
	}
	return persistConfigFileMode(path, b, restrict)
}

func configMapFromFileStruct() map[string]interface{} {
	b, err := json.Marshal(configFromViper())
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if json.Unmarshal(b, &out) != nil {
		return map[string]interface{}{}
	}
	return out
}
