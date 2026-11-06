package dashboard

import (
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// setupIntegrationTest points the API client at a mock server and returns a
// function that restores the previous state.
func setupIntegrationTest(mockURL string) func() {
	oldBaseURL := lib.BaseURLOverride
	oldAPIKey := viper.GetString("CRONITOR_API_KEY")

	lib.BaseURLOverride = mockURL
	viper.Set("CRONITOR_API_KEY", "test-api-key-1234567890")
	viper.Set("CRONITOR_API_VERSION", "")

	return func() {
		lib.BaseURLOverride = oldBaseURL
		viper.Set("CRONITOR_API_KEY", oldAPIKey)
		viper.Set("CRONITOR_API_VERSION", "")
	}
}

func withCronitorEnv(t *testing.T, env string) {
	t.Helper()
	prev := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", env)
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", prev) })
}
