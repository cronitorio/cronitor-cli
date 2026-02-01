package testutil

import (
	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ExecuteCommand runs a cobra command with the given args against a mock server,
// capturing stdout and returning the output along with any error.
// It sets up lib.BaseURLOverride and a test API key automatically.
func ExecuteCommand(root *cobra.Command, mockServerURL string, args ...string) (string, error) {
	lib.BaseURLOverride = mockServerURL
	viper.Set("CRONITOR_API_KEY", "test-api-key-1234567890")

	root.SetArgs(args)

	var execErr error
	output := CaptureStdout(func() {
		execErr = root.Execute()
	})

	return output, execErr
}
