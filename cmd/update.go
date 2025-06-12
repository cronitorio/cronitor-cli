package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"crypto/sha256"
	"encoding/hex"

	"archive/tar"
	"compress/gzip"

	"github.com/spf13/cobra"
)

type GithubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type UpdateStatus struct {
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	HasUpdate      bool   `json:"has_update"`
	DownloadURL    string `json:"download_url,omitempty"`
	AssetName      string `json:"asset_name,omitempty"`
	Error          string `json:"error,omitempty"`
}

const (
	checksumExtension = ".sha256"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update to the latest version",
	Run:   runUpdate,
}

func init() {
	RootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) {
	status, err := checkForUpdate()
	if err != nil {
		fatal(fmt.Sprintf("Error checking for updates: %v", err), 1)
	}

	if !status.HasUpdate {
		fmt.Printf("You are already on the latest version (%s)\n", status.CurrentVersion)
		return
	}

	fmt.Printf("Updating from version %s to %s...\n", status.CurrentVersion, status.LatestVersion)

	if err := performUpdate(status); err != nil {
		fatal(fmt.Sprintf("Error during update: %v", err), 1)
	}

	fmt.Printf("Update complete! You are now on version %s\n", status.LatestVersion)
}

// checkForUpdate checks if a new version is available
func checkForUpdate() (*UpdateStatus, error) {
	currentVersion := Version
	status := &UpdateStatus{
		CurrentVersion: currentVersion,
	}

	// Get latest release info
	release, err := getLatestRelease()
	if err != nil {
		status.Error = err.Error()
		return status, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	status.LatestVersion = latestVersion
	status.HasUpdate = isNewer(latestVersion, currentVersion)

	if status.HasUpdate {
		// Find the appropriate asset for current platform
		expectedName := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
		for _, asset := range release.Assets {
			if strings.HasPrefix(asset.Name, expectedName) && !strings.HasSuffix(asset.Name, checksumExtension) {
				status.DownloadURL = asset.BrowserDownloadURL
				status.AssetName = asset.Name
				break
			}
		}

		if status.DownloadURL == "" {
			err := fmt.Errorf("no release found for %s/%s", runtime.GOOS, runtime.GOARCH)
			status.Error = err.Error()
			return status, err
		}
	}

	return status, nil
}

// performUpdate downloads and installs the update
func performUpdate(status *UpdateStatus) error {
	if !status.HasUpdate {
		return fmt.Errorf("no update available")
	}

	// Get latest release info again to get checksum
	release, err := getLatestRelease()
	if err != nil {
		return err
	}

	// Get checksum
	checksum, err := downloadChecksum(release, status.AssetName+checksumExtension)
	if err != nil {
		return fmt.Errorf("error downloading checksum: %v", err)
	}

	// Get current executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("error getting executable path: %v", err)
	}

	// Download and verify binary
	tmpFile := executable + ".new"
	if err := downloadAndVerifyFile(status.DownloadURL, tmpFile, strings.TrimSpace(string(checksum))); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("error downloading update: %v", err)
	}

	// Make new file executable
	if err := os.Chmod(tmpFile, 0755); err != nil {
		os.Remove(tmpFile) // Clean up
		return fmt.Errorf("error setting permissions: %v", err)
	}

	// Test that the new binary is executable by running it with --version
	execCmd := exec.Command(tmpFile, "--help")
	if err := execCmd.Run(); err != nil {
		os.Remove(tmpFile) // Clean up
		return fmt.Errorf("error verifying new binary: %v", err)
	}

	// Replace the current executable
	if err := replaceExecutable(executable, tmpFile); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("error installing new version: %v", err)
	}

	return nil
}

// replaceExecutable safely replaces the current executable with the new one
func replaceExecutable(currentPath, newPath string) error {
	// Rename current executable to .old (backup)
	oldFile := currentPath + ".old"
	if err := os.Rename(currentPath, oldFile); err != nil {
		return fmt.Errorf("error backing up current version: %v", err)
	}

	// Move new executable into place
	if err := os.Rename(newPath, currentPath); err != nil {
		// Try to restore old version
		os.Rename(oldFile, currentPath)
		return fmt.Errorf("error installing new version: %v", err)
	}

	// Clean up old version
	os.Remove(oldFile)
	return nil
}

func getLatestRelease() (*GithubRelease, error) {
	resp, err := http.Get("https://api.github.com/repos/cronitorio/cronitor-cli/releases/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GithubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}

	return &release, nil
}

func downloadChecksum(release *GithubRelease, checksumFile string) ([]byte, error) {
	for _, asset := range release.Assets {
		if asset.Name == checksumFile {
			resp, err := http.Get(asset.BrowserDownloadURL)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			return io.ReadAll(resp.Body)
		}
	}
	return nil, fmt.Errorf("checksum file not found for release")
}

func downloadAndVerifyFile(url, dest, expectedChecksum string) error {
	// Download to memory first to verify before writing to disk
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Read the entire response body into memory and calculate checksum
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %v", err)
	}

	// Verify checksum before proceeding
	hasher := sha256.New()
	hasher.Write(body)
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum verification failed (expected: %s, got: %s)", expectedChecksum, actualChecksum)
	}

	// Create gzip reader from verified data
	gzipReader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("error creating gzip reader: %v", err)
	}
	defer gzipReader.Close()

	// Create tar reader
	tarReader := tar.NewReader(gzipReader)

	// Read the first (and should be only) file from the archive
	_, err = tarReader.Next()
	if err != nil {
		return fmt.Errorf("error reading tar: %v", err)
	}

	// Create output file
	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the decompressed data to the file
	if _, err := io.Copy(out, tarReader); err != nil {
		return fmt.Errorf("error extracting file: %v", err)
	}

	return nil
}

func isNewer(latest, current string) bool {
	// Split versions into parts
	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	// Convert to integers for comparison
	latestMajor, _ := strconv.Atoi(latestParts[0])
	latestMinor, _ := strconv.Atoi(latestParts[1])
	currentMajor, _ := strconv.Atoi(currentParts[0])
	currentMinor, _ := strconv.Atoi(currentParts[1])

	// Compare major version first
	if latestMajor > currentMajor {
		return true
	}
	if latestMajor < currentMajor {
		return false
	}

	// If major versions are equal, compare minor versions
	return latestMinor > currentMinor
}
