//go:build windows

package lib

import (
	"strings"
	"testing"
)

func closeInheritedFDs() {}

func TestWindowsWriteDoesNotEmitShim(t *testing.T) {
	ct := parseContent(t, "15 * * * * /bin/true")
	ct.WriteMode = WriteModeShim
	got := ct.Write()
	if strings.Contains(got, "MONITORIO=") || strings.Contains(got, "cronitor-shell") || strings.Contains(got, "CRONITOR_REAL_SHELL") {
		t.Fatalf("windows wrote a shim:\n%s", got)
	}
}
