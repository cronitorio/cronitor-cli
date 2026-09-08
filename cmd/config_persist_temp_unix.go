//go:build !windows && !darwin

package cmd

import (
	"os"
	"path/filepath"
)

// persistCreateTemp makes the staging file next to the destination.
func persistCreateTemp(path string, _ os.FileInfo, _ bool) (*os.File, error) {
	return os.CreateTemp(filepath.Dir(path), ".cronitor-config-*.tmp")
}
