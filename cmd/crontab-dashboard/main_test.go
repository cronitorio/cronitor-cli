package main

import (
	"reflect"
	"testing"

	"github.com/cronitorio/cronitor-cli/cmd"
)

func TestDashboardArgsRunDash(t *testing.T) {
	for _, tc := range []struct{ in, want []string }{
		{[]string{"crontab-dashboard"}, []string{"crontab-dashboard", "dash"}},
		{[]string{"crontab-dashboard", "--port", "9100"}, []string{"crontab-dashboard", "dash", "--port", "9100"}},
		{[]string{"crontab-dashboard", "dash", "--safe-mode"}, []string{"crontab-dashboard", "dash", "--safe-mode"}},
	} {
		if got := dashboardArgs(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("dashboardArgs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDashboardRegistersDash(t *testing.T) {
	setup()
	found, _, err := cmd.RootCmd.Find([]string{"dash"})
	if err != nil || found.Name() != "dash" {
		t.Fatalf("dash not registered: %v", err)
	}
	if found.Flags().Lookup("port") == nil {
		t.Fatal("dash is missing --port")
	}
	if cmd.UpdateBinary() != cmd.DashboardBinaryName {
		t.Errorf("updater binary = %q", cmd.UpdateBinary())
	}
	if cmd.RootCmd.Use != "crontab-dashboard" {
		t.Errorf("root Use = %q", cmd.RootCmd.Use)
	}
}
