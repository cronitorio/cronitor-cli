package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var launchDashboard = execDashboard

// dash is retained as a launcher so existing services and MCP configurations
// can migrate without changing their command lines.
var dashCmd = &cobra.Command{
	Use:                "dash",
	Short:              "Start the separately installed Crontab Guru Dashboard",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := dashboardExecutable()
		if err != nil {
			return err
		}
		env := os.Environ()
		for flag, name := range map[string]string{
			"config": varConfig, "env": varEnv, "api-key": varApiKey,
			"ping-api-key": varPingApiKey, "ping-api-host": varPingApiHost,
			"hostname": varHostname, "log": varLog, "users": varUsers,
			"api-version": varApiVersion,
		} {
			if value := RootCmd.PersistentFlags().Lookup(flag); value != nil && value.Changed {
				prefix := name + "="
				filtered := env[:0]
				for _, entry := range env {
					if !strings.HasPrefix(entry, prefix) {
						filtered = append(filtered, entry)
					}
				}
				env = append(filtered, prefix+value.Value.String())
			}
		}
		for _, name := range []string{"verbose", "use-dev"} {
			if flag := RootCmd.PersistentFlags().Lookup(name); flag != nil && flag.Changed {
				args = append([]string{"--" + name + "=" + flag.Value.String()}, args...)
			}
		}
		return launchDashboard(path, args, env)
	},
}

func dashboardExecutable() (string, error) {
	if path := os.Getenv("CRONTAB_DASHBOARD_BIN"); path != "" {
		return exec.LookPath(path)
	}
	if executable, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(executable); err == nil {
			executable = real
		}
		name := "crontab-dashboard"
		if filepath.Ext(executable) == ".exe" {
			name += ".exe"
		}
		if path, err := exec.LookPath(filepath.Join(filepath.Dir(executable), name)); err == nil {
			return path, nil
		}
	}
	if path, err := exec.LookPath("crontab-dashboard"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("Crontab Guru Dashboard is now installed separately. Install its release bundle from https://github.com/cronitorio/crontab-dashboard, then run 'crontab-dashboard' or 'cronitor dash'")
}

func init() { RootCmd.AddCommand(dashCmd) }
