package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/spf13/viper"
)

// TestDiscoverSyncKeepsWrappedPrefix drives processCrontab twice. Discover's
// update leaves Line.Code empty and the code on Mon; Write has to use GetCode
// or the first sync drops the custom binary path and flags.
func TestDiscoverSyncKeepsWrappedPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Echo the monitor key the client sent. That key is the line code, not
		// the hash PutMonitors indexes by, so Attributes.Code is not applied
		// and Line.Code becomes empty while Mon.Code keeps the code.
		var req []struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		type monitor struct {
			Attributes struct {
				Key  string `json:"key"`
				Code string `json:"code"`
			} `json:"attributes"`
		}
		resp := make([]monitor, len(req))
		for i, m := range req {
			resp[i].Attributes.Key = m.Key
			resp[i].Attributes.Code = "SHOULD_NOT_APPLY"
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	cleanup := setupIntegrationTest(srv.URL + "/api")
	defer cleanup()

	oldAuto, oldSilent, oldDry, oldNoStdout := isAutoDiscover, isSilent, dryRun, noStdoutPassthru
	oldExisting := existingMonitors
	oldEnv := viper.GetString("CRONITOR_ENV")
	oldNotes := notificationList
	t.Cleanup(func() {
		isAutoDiscover, isSilent, dryRun, noStdoutPassthru = oldAuto, oldSilent, oldDry, oldNoStdout
		existingMonitors = oldExisting
		viper.Set("CRONITOR_ENV", oldEnv)
		notificationList = oldNotes
	})
	isAutoDiscover = true
	isSilent = true
	dryRun = false
	noStdoutPassthru = true
	existingMonitors = ExistingMonitors{}
	notificationList = ""
	viper.Set("CRONITOR_ENV", "prod")

	path := filepath.Join(t.TempDir(), "crontab")
	original := "# Name: Backup\n0 * * * * /opt/cronitor --env staging -c alt.json exec k1 /bin/true\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	syncOnce := func() {
		t.Helper()
		ct := lib.CrontabFactory("wrap-sync", path)
		if !processCrontab(ct) {
			t.Fatal("sync did not update the crontab")
		}
	}
	syncOnce()
	first := readCrontab(t, path)
	syncOnce()
	second := readCrontab(t, path)
	if first != second {
		t.Fatalf("sync output changed\n first: %s\nsecond: %s", first, second)
	}
	want := "# Name: Backup\n0 * * * * /opt/cronitor --env staging -c alt.json exec --no-stdout k1 /bin/true\n"
	if second != want {
		t.Fatalf("sync wrote %q", second)
	}
}

// TestWrittenExecLineRuns builds the CLI and runs the line Write emits.
// --no-stdout before exec is rejected by cobra; after exec the job runs.
func TestWrittenExecLineRuns(t *testing.T) {
	bin := buildCronitor(t)
	cfg := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	originalEnv := viper.GetString("CRONITOR_ENV")
	viper.Set("CRONITOR_ENV", "prod")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", originalEnv) })

	line := lib.Line{
		IsJob:          true,
		CronExpression: "0 * * * *",
		CommandToRun:   "echo hello-from-job",
		Code:           "k1",
		Mon:            lib.Monitor{NoStdoutPassthru: true},
		Crontab:        lib.Crontab{IsUserCrontab: true},
	}
	written := strings.TrimSpace(line.Write())
	want := "0 * * * * cronitor --env prod exec --no-stdout k1 echo hello-from-job"
	if written != want {
		t.Fatalf("written line\n got %s\nwant %s", written, want)
	}

	out, err := runCronitorLine(t, bin, cfg, written)
	if err != nil {
		t.Fatalf("written line failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "hello-from-job") {
		t.Fatalf("job did not run: %s", out)
	}
	if strings.Contains(out, "unknown command") {
		t.Fatalf("cobra rejected the written line: %s", out)
	}

	// The placement this used to emit. Cobra treats the key as a subcommand.
	bad := "0 * * * * cronitor --env prod --no-stdout exec k1 echo hello-from-job"
	badOut, badErr := runCronitorLine(t, bin, cfg, bad)
	if badErr == nil || !strings.Contains(badOut, `unknown command "k1"`) || strings.Contains(badOut, "hello-from-job") {
		t.Fatalf("pre-exec --no-stdout should be rejected, err=%v\n%s", badErr, badOut)
	}
}

// TestEnvExecValuePingsFirstExec runs `--env exec exec realkey` and checks the
// ping key. The arg scanner uses the first exec as the subcommand.
func TestEnvExecValuePingsFirstExec(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bin := buildCronitor(t)
	cfg := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "--config", cfg, "--ping-api-host", srv.URL, "--env", "exec", "exec", "realkey", "echo", "should-not-run")
	cmd.Env = cronitorTestEnv()
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected the command realkey to fail, output: %s", out)
	}
	if strings.Contains(string(out), "should-not-run") {
		t.Fatalf("echo ran, so the key was not exec: %s", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) == 0 {
		t.Fatalf("no ping was sent: %s", out)
	}
	for _, path := range paths {
		if strings.Contains(path, "/realkey/") {
			t.Fatalf("pinged realkey: %v", paths)
		}
	}
	if !strings.Contains(paths[0], "/exec/") {
		t.Fatalf("ping path %q, want the exec key", paths[0])
	}
}

func runCronitorLine(t *testing.T, bin, cfg, crontabLine string) (string, error) {
	t.Helper()
	fields := strings.Fields(crontabLine)
	// Drop the five schedule fields and the literal binary name.
	args := append([]string{"--config", cfg}, fields[6:]...)
	cmd := exec.Command(bin, args...)
	cmd.Env = cronitorTestEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func cronitorTestEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "CRONITOR_") {
			continue
		}
		env = append(env, entry)
	}
	return env
}

func buildCronitor(t *testing.T) string {
	t.Helper()
	root := moduleRoot(t)
	bin := filepath.Join(t.TempDir(), "cronitor")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
