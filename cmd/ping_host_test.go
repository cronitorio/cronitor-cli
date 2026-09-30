package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

func TestNormalizePingApiHost(t *testing.T) {
	cases := map[string]string{
		"":                           defaultPingApiHost,
		"   ":                        defaultPingApiHost,
		"eu.cronitor.link":           "https://eu.cronitor.link",
		" eu.cronitor.link ":         "https://eu.cronitor.link",
		"https://eu.cronitor.link/":  "https://eu.cronitor.link",
		"http://proxy.internal:8080": "http://proxy.internal:8080",
	}
	for input, want := range cases {
		if got := normalizePingApiHost(input); got != want {
			t.Errorf("normalizePingApiHost(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestPingHostForAttempt(t *testing.T) {
	cases := map[string][]string{
		// Default behavior is unchanged: retries alternate with cronitor.io from attempt 3.
		"https://cronitor.link": {
			"https://cronitor.link", "https://cronitor.link", "https://cronitor.io",
			"https://cronitor.link", "https://cronitor.io", "https://cronitor.link",
		},
		// Other cronitor.link hosts keep the cronitor.io fallback.
		"https://eu.cronitor.link": {
			"https://eu.cronitor.link", "https://eu.cronitor.link", "https://cronitor.io",
			"https://eu.cronitor.link", "https://cronitor.io", "https://eu.cronitor.link",
		},
		// Non-Cronitor hosts (for example a private proxy) are never bypassed.
		"http://proxy.internal:8080": {
			"http://proxy.internal:8080", "http://proxy.internal:8080", "http://proxy.internal:8080",
			"http://proxy.internal:8080", "http://proxy.internal:8080", "http://proxy.internal:8080",
		},
	}
	for configured, want := range cases {
		for attempt := 1; attempt <= len(want); attempt++ {
			if got := pingHostForAttempt(configured, attempt); got != want[attempt-1] {
				t.Errorf("pingHostForAttempt(%q, %d) = %q, want %q", configured, attempt, got, want[attempt-1])
			}
		}
	}
}

func TestSendPing_UsesConfiguredPingApiHost(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path+"?state="+r.URL.Query().Get("state"))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	oldOverride, oldDev := lib.PingHostOverride, dev
	oldHost, oldPingKey, oldAPIKey := viper.GetString(varPingApiHost), viper.GetString(varPingApiKey), viper.GetString(varApiKey)
	t.Cleanup(func() {
		lib.PingHostOverride, dev = oldOverride, oldDev
		viper.Set(varPingApiHost, oldHost)
		viper.Set(varPingApiKey, oldPingKey)
		viper.Set(varApiKey, oldAPIKey)
	})
	lib.PingHostOverride, dev = "", false
	viper.Set(varPingApiHost, server.URL)
	viper.Set(varPingApiKey, "pingkey123")
	viper.Set(varApiKey, "")

	sendPing("tick", "host-metrics", "", "", 0, nil, nil, nil, "", nil)

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/ping/pingkey123/host-metrics?state=tick" {
		t.Fatalf("configured host received %v, want one tick ping", paths)
	}
}

func TestConfigure_SavesPingApiHost(t *testing.T) {
	resetConfigureTestState(t)
	oldHost := viper.GetString(varPingApiHost)
	t.Cleanup(func() { viper.Set(varPingApiHost, oldHost) })
	path := filepath.Join(t.TempDir(), "cronitor.json")
	viper.Set(varConfig, path)
	viper.Set(varPingApiHost, "eu.cronitor.link")

	captureOutput(t, func() { configureCmd.Run(configureCmd, []string{}) })

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got ConfigFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.PingApiHost != "eu.cronitor.link" {
		t.Errorf("ping api host: got %q", got.PingApiHost)
	}
}
