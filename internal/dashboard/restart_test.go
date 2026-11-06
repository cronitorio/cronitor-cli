package dashboard

import "testing"

// Units written for `cronitor dash` must still be found when the running
// binary is crontab-dashboard.
func TestSystemdServiceCandidatesKeepCronitorNames(t *testing.T) {
	got := map[string]bool{}
	for _, name := range systemdServiceCandidates("crontab-dashboard") {
		got[name] = true
	}
	for _, want := range []string{"crontab-dashboard.service", "cronitor-dashboard.service", "cronitor.service", "crontab-guru-dashboard.service"} {
		if !got[want] {
			t.Errorf("missing %s", want)
		}
	}
}
