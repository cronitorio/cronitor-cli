package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFindAssetPicksEachBinary(t *testing.T) {
	release := &GithubRelease{}
	for _, name := range []string{
		"crontab-dashboard_linux_amd64.tar.gz", "crontab-dashboard_linux_amd64.tar.gz.sha256",
		"linux_amd64.tar.gz.md5", "linux_amd64.tar.gz", "linux_amd64.tar.gz.sha256",
		"linux_arm64.tar.gz",
	} {
		release.Assets = append(release.Assets, struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		}{name, "https://example.test/" + name})
	}
	if got, _ := findAsset(release, "cronitor", "linux", "amd64"); got != "linux_amd64.tar.gz" {
		t.Errorf("cronitor asset = %q", got)
	}
	if got, _ := findAsset(release, DashboardBinaryName, "linux", "amd64"); got != "crontab-dashboard_linux_amd64.tar.gz" {
		t.Errorf("dashboard asset = %q", got)
	}
	if got, _ := findAsset(release, DashboardBinaryName, "linux", "arm64"); got != "" {
		t.Errorf("missing dashboard asset matched %q", got)
	}
}

// fakeRelease serves a GitHub release whose binaries are shell scripts that
// print which binary they are.
func fakeRelease(t *testing.T) {
	t.Helper()
	platform := runtime.GOOS + "_" + runtime.GOARCH
	files := map[string][]byte{}
	for asset, body := range map[string]string{
		platform + ".tar.gz":                        "new cronitor",
		"crontab-dashboard_" + platform + ".tar.gz": "new dashboard",
	} {
		archive := tarGz(t, "#!/bin/sh\n# "+body+"\nexit 0\n")
		sum := sha256.Sum256(archive)
		files[asset] = archive
		files[asset+".sha256"] = []byte(hex.EncodeToString(sum[:]) + "\n")
	}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			release := GithubRelease{TagName: "999.0"}
			for name := range files {
				release.Assets = append(release.Assets, struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				}{name, server.URL + "/" + name})
			}
			json.NewEncoder(w).Encode(release)
			return
		}
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	}))
	t.Cleanup(server.Close)
	oldURL := releaseURL
	releaseURL = server.URL + "/latest"
	t.Cleanup(func() { releaseURL = oldURL })
}

func tarGz(t *testing.T, script string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "bin", Mode: 0755, Size: int64(len(script))})
	tw.Write([]byte(script))
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func installFake(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n# old\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func useExecutable(t *testing.T, path, binary string) {
	t.Helper()
	oldPath, oldBinary := executablePath, updateBinary
	executablePath = func() (string, error) { return path, nil }
	updateBinary = binary
	t.Cleanup(func() { executablePath, updateBinary = oldPath, oldBinary })
}

func assertContains(t *testing.T, path, want string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), want) {
		t.Errorf("%s = %q, want %q", filepath.Base(path), b, want)
	}
}

func runFakeUpdate(t *testing.T) {
	t.Helper()
	status, err := checkForUpdate()
	if err != nil {
		t.Fatal(err)
	}
	if err := performUpdate(status); err != nil {
		t.Fatal(err)
	}
}

func TestCronitorUpdateAlsoUpdatesDashboard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixtures")
	}
	fakeRelease(t)
	dir := t.TempDir()
	cli := installFake(t, dir, "cronitor")
	dash := installFake(t, dir, DashboardBinaryName)
	useExecutable(t, cli, "cronitor")
	runFakeUpdate(t)
	assertContains(t, cli, "new cronitor")
	assertContains(t, dash, "new dashboard")
}

func TestDashboardUpdateAlsoUpdatesCronitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixtures")
	}
	fakeRelease(t)
	dir := t.TempDir()
	cli := installFake(t, dir, "cronitor")
	dash := installFake(t, dir, DashboardBinaryName)
	useExecutable(t, dash, DashboardBinaryName)
	runFakeUpdate(t)
	assertContains(t, dash, "new dashboard")
	assertContains(t, cli, "new cronitor")
}

func TestCronitorUpdateWithoutDashboardInstallsNoDashboard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixtures")
	}
	fakeRelease(t)
	dir := t.TempDir()
	cli := installFake(t, dir, "cronitor")
	useExecutable(t, cli, "cronitor")
	runFakeUpdate(t)
	assertContains(t, cli, "new cronitor")
	if _, err := os.Stat(filepath.Join(dir, DashboardBinaryName)); !os.IsNotExist(err) {
		t.Errorf("dashboard was installed: %v", err)
	}
}
