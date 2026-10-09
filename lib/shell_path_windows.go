//go:build windows

package lib

func pathWritable(string) bool { return false }

func warnShimBinaryOwnership(string, string) {}
