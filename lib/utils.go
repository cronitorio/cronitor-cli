package lib

import (
	"math/rand"
	"os"
	"path/filepath"
)

func randomMinute() int {
	return rand.Intn(59)
}

func EnumerateFiles(dirToEnumerate string) []string {
	var fileList []string
	entries, err := os.ReadDir(dirToEnumerate)
	if err != nil {
		return fileList
	}

	for _, entry := range entries {
		if entry.Name()[0] == '.' {
			continue
		}
		fileList = append(fileList, filepath.Join(dirToEnumerate, entry.Name()))
	}

	return fileList
}
