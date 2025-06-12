//go:build !windows
// +build !windows

package cmd

func GetNextRunFromMonitorKey(key string) string {
	return ""
}
