package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cronitorio/cronitor-cli/lib"
	"github.com/kballard/go-shellquote"
	"github.com/spf13/viper"
)

// runSync drives `cronitor sync` itself. Tests must not assign line.Code and
// call Write; that skips the flag handling and the API round trip.
func runSync(t *testing.T, serverURL, path string, extra ...string) string {
	t.Helper()
	lib.BaseURLOverride = serverURL + "/api"
	viper.Set(varApiKey, "test-api-key-1234567890")
	existingMonitors = ExistingMonitors{}
	userAbortedSync = false
	dryRun = false
	execStyle = false
	convertToShim = false
	noStdoutPassthru = false
	_ = discoverCmd.Flags().Set("exec-style", "false")
	_ = discoverCmd.Flags().Set("convert-to-shim", "false")
	_ = discoverCmd.Flags().Set("dry-run", "false")
	_ = discoverCmd.Flags().Set("no-stdout", "false")
	for _, name := range []string{"exec-style", "convert-to-shim", "dry-run", "no-stdout"} {
		if f := discoverCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
	for _, name := range []string{"env", "api-key", "hostname"} {
		if f := RootCmd.PersistentFlags().Lookup(name); f != nil {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		}
	}

	cfg := filepath.Join(t.TempDir(), "cronitor.json")
	if err := os.WriteFile(cfg, []byte(`{"CRONITOR_API_KEY":"test-api-key-1234567890"}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Execute on the subcommand is ignored: cobra always runs from the root.
	args := []string{"--config", cfg, "sync", "--auto", "--silent"}
	args = append(args, extra...)
	args = append(args, path)
	RootCmd.SetArgs(args)

	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStderr, oldStdout := os.Stderr, os.Stdout
	os.Stderr, os.Stdout = stderrW, stdoutW
	execErr := RootCmd.Execute()
	stderrW.Close()
	stdoutW.Close()
	os.Stderr, os.Stdout = oldStderr, oldStdout
	errText, _ := io.ReadAll(stderrR)
	outText, _ := io.ReadAll(stdoutR)
	stderrR.Close()
	stdoutR.Close()
	if execErr != nil {
		t.Fatalf("sync: %v\nstderr:\n%s\nstdout:\n%s", execErr, errText, outText)
	}
	return string(errText)
}

func monitorServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"total_monitor_count":0,"page_size":50,"monitors":[]}`)
			return
		}
		var body []map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		out := make([]map[string]interface{}, 0, len(body))
		for _, mon := range body {
			key, _ := mon["key"].(string)
			code, _ := mon["code"].(string)
			if code == "" {
				if len(key) >= 6 {
					code = "m" + key[:6]
				} else {
					code = "mcode1"
				}
			}
			out = append(out, map[string]interface{}{
				"key": key,
				"attributes": map[string]string{
					"key":  key,
					"code": code,
				},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}))
}

func withSyncFixture(t *testing.T) (serverURL, shimPath string) {
	t.Helper()
	server := monitorServer(t)
	t.Cleanup(server.Close)
	shimPath = filepath.Join(t.TempDir(), "cronitor-shell")
	oldOverride := lib.ShimShellPathOverride
	oldBase := lib.BaseURLOverride
	oldAuto, oldSilent := isAutoDiscover, isSilent
	oldExec, oldConvert := execStyle, convertToShim
	oldDry := dryRun
	oldMonitors := existingMonitors
	oldKey := viper.GetString(varApiKey)
	oldCfg := cfgFile
	lib.ShimShellPathOverride = shimPath
	t.Cleanup(func() {
		lib.ShimShellPathOverride = oldOverride
		lib.BaseURLOverride = oldBase
		isAutoDiscover = oldAuto
		isSilent = oldSilent
		execStyle = oldExec
		convertToShim = oldConvert
		dryRun = oldDry
		existingMonitors = oldMonitors
		cfgFile = oldCfg
		viper.Set(varApiKey, oldKey)
		viper.Set(varConfig, oldCfg)
		noStdoutPassthru = false
		for _, name := range []string{"env", "api-key", "hostname", "config"} {
			if f := RootCmd.PersistentFlags().Lookup(name); f != nil {
				_ = f.Value.Set(f.DefValue)
				f.Changed = false
			}
		}
		if f := discoverCmd.Flags().Lookup("no-stdout"); f != nil {
			_ = f.Value.Set("false")
			f.Changed = false
		}
		_ = discoverCmd.Flags().Set("convert-to-shim", "false")
		_ = discoverCmd.Flags().Set("exec-style", "false")
		_ = discoverCmd.Flags().Set("auto", "false")
		_ = discoverCmd.Flags().Set("silent", "false")
		if oldCfg == "" {
			_ = RootCmd.PersistentFlags().Set("config", "")
		} else {
			_ = RootCmd.PersistentFlags().Set("config", oldCfg)
		}
		RootCmd.SetArgs(nil)
	})
	return server.URL, shimPath
}

func writeCron(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "crontab")
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func hasLine(body, line string) bool {
	for _, got := range strings.Split(body, "\n") {
		if got == line {
			return true
		}
	}
	return false
}

func readCron(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSyncDefaultShimThroughCommand(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	path := writeCron(t, strings.Join([]string{
		"SHELL=/bin/bash",
		"# cronitor: ignore",
		"15 2 * * * echo skip-me",
		"# cronitor: ignore",
		"16 2 * * * date +%Y",
		"# cronitor: ignore",
		"17 2 * * * date +\\%Y",
		"0 2 * * * /usr/local/bin/backup.sh --full",
		"30 2 * * * cd /tmp && echo \"hi\"",
		"45 2 * * * date +\\%Y",
		"@hourly /usr/bin/true",
	}, "\n"))

	runSync(t, serverURL, path)
	first := readCron(t, path)
	if !strings.Contains(first, "CRONITOR_REAL_SHELL=/bin/bash\n") {
		t.Fatalf("prior bash shell not preserved:\n%s", first)
	}
	if !strings.Contains(first, "SHELL="+shimPath+"\n") {
		t.Fatalf("SHELL= not pointed at wrapper:\n%s", first)
	}
	if hasLine(first, "SHELL=/bin/bash") {
		t.Fatalf("old SHELL line still present:\n%s", first)
	}
	if strings.Contains(first, "cronitor exec") {
		t.Fatalf("new lines used exec style:\n%s", first)
	}
	if !strings.Contains(first, "MONITORIO=") {
		t.Fatalf("expected MONITORIO lines:\n%s", first)
	}
	for _, untouched := range []string{
		"15 2 * * * echo skip-me",
		"16 2 * * * date +%Y",
		"17 2 * * * date +\\%Y",
	} {
		if !hasLine(first, untouched) {
			t.Fatalf("unmonitored line %q was rewritten:\n%s", untouched, first)
		}
	}
	if strings.Contains(first, `+\\%`) {
		t.Fatalf("percent was double-escaped:\n%s", first)
	}
	if !strings.Contains(first, `date +\%Y`) {
		t.Fatalf("percent not escaped once:\n%s", first)
	}
	if !strings.Contains(first, `cd /tmp && echo "hi"`) {
		t.Fatalf("complex command was re-quoted:\n%s", first)
	}
	if _, err := os.Stat(shimPath); err != nil {
		t.Fatalf("wrapper was not installed: %v", err)
	}

	runSync(t, serverURL, path)
	second := readCron(t, path)
	if first != second {
		t.Fatalf("second sync changed the crontab\n--- first\n%s\n--- second\n%s", first, second)
	}
	runSync(t, serverURL, path)
	third := readCron(t, path)
	if second != third {
		t.Fatalf("third sync changed the crontab\n--- second\n%s\n--- third\n%s", second, third)
	}
}

func TestSyncKeepsExistingExecLineByteForByte(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	execLine := "0 1 * * * /opt/cronitor --env staging exec k1 /bin/echo already"
	path := writeCron(t, strings.Join([]string{
		execLine,
		"0 2 * * * /bin/true",
	}, "\n"))

	runSync(t, serverURL, path)
	first := readCron(t, path)
	if !strings.Contains(first, execLine+"\n") {
		t.Fatalf("exec line was rewritten:\n%s", first)
	}
	if strings.Contains(first, "cronitor exec m") || strings.Count(first, "exec ") != 1 {
		t.Fatalf("exec line was not left alone:\n%s", first)
	}
	if !strings.Contains(first, "SHELL="+shimPath) {
		t.Fatalf("new line did not install the shim:\n%s", first)
	}
	if !strings.Contains(first, "MONITORIO=") {
		t.Fatalf("new line was not shim style:\n%s", first)
	}
	if !strings.Contains(first, "CRONITOR_REAL_SHELL=/bin/sh\n") {
		t.Fatalf("missing default real shell:\n%s", first)
	}

	runSync(t, serverURL, path)
	if second := readCron(t, path); second != first {
		t.Fatalf("mixed crontab not stable\n--- first\n%s\n--- second\n%s", first, second)
	}
}

func TestSyncConvertSkipsInexpressibleFlags(t *testing.T) {
	serverURL, _ := withSyncFixture(t)
	// Sync rewrites the saved prefix. --no-stdout moves to after exec.
	keptIn := "0 1 * * * /opt/cronitor --env staging --no-stdout exec k1 /bin/echo already"
	kept := "0 1 * * * /opt/cronitor --env staging exec --no-stdout k1 /bin/echo already"
	kept2 := "0 3 * * * cronitor exec --no-stdout k3 /bin/true"
	path := writeCron(t, strings.Join([]string{
		keptIn,
		"0 2 * * * cronitor exec k2 /bin/true",
		kept2,
		`0 4 * * * cronitor exec k4 echo "hello  world"`,
	}, "\n"))

	stderr := runSync(t, serverURL, path, "--convert-to-shim")
	if !strings.Contains(stderr, "skipped") || !strings.Contains(stderr, "--env") || !strings.Contains(stderr, "--no-stdout") {
		t.Fatalf("expected a skip notice:\n%s", stderr)
	}
	if !strings.Contains(stderr, "monitor k1") || strings.Contains(stderr, "line ") {
		t.Fatalf("skip notice is not keyed by monitor code:\n%s", stderr)
	}
	first := readCron(t, path)
	if !strings.Contains(first, kept+"\n") || !strings.Contains(first, kept2+"\n") {
		t.Fatalf("flagged lines lost flags or left --no-stdout before exec:\n%s", first)
	}
	if strings.Contains(first, "--no-stdout exec") {
		t.Fatalf("--no-stdout stayed before exec:\n%s", first)
	}
	if !strings.Contains(first, "MONITORIO=k2 /bin/true") {
		t.Fatalf("plain exec line was not converted:\n%s", first)
	}
	if !strings.Contains(first, `MONITORIO=k4 echo "hello  world"`) {
		t.Fatalf("command bytes were rebuilt:\n%s", first)
	}
	if strings.Contains(first, "MONITORIO=k1") || strings.Contains(first, "MONITORIO=k3") {
		t.Fatalf("flagged line was converted:\n%s", first)
	}
	if again := runSync(t, serverURL, path, "--convert-to-shim"); again != stderr {
		t.Fatalf("skip notices changed across runs\n--- first\n%s\n--- second\n%s", stderr, again)
	}

	runSync(t, serverURL, path, "--exec-style")
	reverted := readCron(t, path)
	if !strings.Contains(reverted, kept+"\n") {
		t.Fatalf("exec-style round trip dropped flags:\n%s", reverted)
	}
	if !strings.Contains(reverted, kept2+"\n") {
		t.Fatalf("exec-style round trip dropped --no-stdout:\n%s", reverted)
	}
	if strings.Contains(reverted, "MONITORIO=") {
		t.Fatalf("exec-style left a marker:\n%s", reverted)
	}
	if !strings.Contains(reverted, `echo "hello  world"`) {
		t.Fatalf("reverted command lost bytes:\n%s", reverted)
	}
}

func TestSyncExecStyleRevertsShimAndRestoresShell(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	path := writeCron(t, strings.Join([]string{
		"SHELL=/bin/bash",
		"0 1 * * * /opt/cronitor --env staging exec k1 /bin/echo already",
		"0 2 * * * /bin/true",
	}, "\n"))
	runSync(t, serverURL, path)
	shimmed := readCron(t, path)
	if !strings.Contains(shimmed, "SHELL="+shimPath) || !strings.Contains(shimmed, "MONITORIO=") {
		t.Fatalf("setup sync did not shim:\n%s", shimmed)
	}
	if !strings.Contains(shimmed, "/opt/cronitor --env staging exec k1 /bin/echo already") {
		t.Fatalf("setup sync rewrote the exec line:\n%s", shimmed)
	}

	runSync(t, serverURL, path, "--exec-style")
	reverted := readCron(t, path)
	if strings.Contains(reverted, "MONITORIO=") {
		t.Fatalf("exec-style left a MONITORIO line:\n%s", reverted)
	}
	if strings.Contains(reverted, "CRONITOR_REAL_SHELL=") {
		t.Fatalf("real-shell line kept after revert:\n%s", reverted)
	}
	if !strings.Contains(reverted, "SHELL=/bin/bash\n") {
		t.Fatalf("prior shell was not restored:\n%s", reverted)
	}
	if strings.Contains(reverted, shimPath) {
		t.Fatalf("wrapper SHELL still installed:\n%s", reverted)
	}
	if !strings.Contains(reverted, "/opt/cronitor --env staging exec k1 /bin/echo already") {
		t.Fatalf("exec line changed during revert:\n%s", reverted)
	}
	if strings.Contains(reverted, `\\%`) {
		t.Fatalf("reverted line double-escaped a percent:\n%s", reverted)
	}

	runSync(t, serverURL, path, "--exec-style")
	if again := readCron(t, path); again != reverted {
		t.Fatalf("exec-style not idempotent\n--- first\n%s\n--- second\n%s", reverted, again)
	}
}

func TestSyncExecStyleOnNewLines(t *testing.T) {
	serverURL, _ := withSyncFixture(t)
	path := writeCron(t, "SHELL=/bin/bash\n0 * * * * date +\\%Y\n")
	runSync(t, serverURL, path, "--exec-style", "--no-stdout")
	got := readCron(t, path)
	if strings.Contains(got, "MONITORIO=") || strings.Contains(got, "cronitor-shell") {
		t.Fatalf("exec-style wrote a shim:\n%s", got)
	}
	if !strings.Contains(got, "SHELL=/bin/bash\n") {
		t.Fatalf("SHELL changed even though no shim was installed:\n%s", got)
	}
	if !strings.Contains(got, "cronitor exec --no-stdout ") || !strings.Contains(got, `date +\%Y`) {
		t.Fatalf("new line was not exec style with --no-stdout after exec:\n%s", got)
	}
	if strings.Contains(got, "--no-stdout exec") {
		t.Fatalf("--no-stdout landed before exec:\n%s", got)
	}
	if strings.Contains(got, `+\\%`) {
		t.Fatalf("double-escaped percent:\n%s", got)
	}
	runSync(t, serverURL, path, "--exec-style")
	if again := readCron(t, path); again != got {
		t.Fatalf("not stable\n--- first\n%s\n--- second\n%s", got, again)
	}
}

func TestSyncSystemCrontabUserField(t *testing.T) {
	if _, err := exec.Command("id", "-u", "root").Output(); err != nil {
		t.Skip("root user not resolvable")
	}
	serverURL, _ := withSyncFixture(t)
	path := writeCron(t, strings.Join([]string{
		"0 4 * * * root /usr/bin/true",
		"0 5 * * * root date +\\%Y",
		"0 6 * * * root MONITORIO=k9 /bin/echo hi",
	}, "\n"))
	runSync(t, serverURL, path)
	first := readCron(t, path)
	if strings.Contains(first, `+\\%`) {
		t.Fatalf("user-field line double-escaped %%:\n%s", first)
	}
	for _, snippet := range []string{
		"0 4 * * * root MONITORIO=",
		"0 5 * * * root MONITORIO=",
		`date +\%Y`,
		"0 6 * * * root MONITORIO=k9 /bin/echo hi",
	} {
		if !strings.Contains(first, snippet) {
			t.Fatalf("missing %q in\n%s", snippet, first)
		}
	}
	// The username stays a single field in front of the marker.
	if strings.Contains(first, "root root") || strings.Contains(first, "MONITORIO=root") {
		t.Fatalf("user field was wrapped or duplicated:\n%s", first)
	}
	runSync(t, serverURL, path)
	if second := readCron(t, path); second != first {
		t.Fatalf("system crontab not stable\n--- first\n%s\n--- second\n%s", first, second)
	}
}

func TestSyncFlagsOnNewLinesStayExec(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	// --env and --no-stdout are copied onto exec lines. A MONITORIO line cannot
	// carry them, so new jobs stay exec style.
	viper.Set("CRONITOR_ENV", "staging")
	t.Cleanup(func() { viper.Set("CRONITOR_ENV", "") })
	path := writeCron(t, "0 * * * * /bin/true\n")
	runSync(t, serverURL, path, "--no-stdout")
	got := readCron(t, path)
	if strings.Contains(got, "MONITORIO=") || strings.Contains(got, shimPath) {
		t.Fatalf("flagged sync wrote a shim:\n%s", got)
	}
	if !strings.Contains(got, "cronitor --env staging exec --no-stdout ") {
		t.Fatalf("new line dropped sync flags or put --no-stdout before exec:\n%s", got)
	}

	hostPath := writeCron(t, "0 * * * * /bin/true\n")
	viper.Set("CRONITOR_ENV", "")
	runSync(t, serverURL, hostPath, "--hostname", "box")
	hosted := readCron(t, hostPath)
	if strings.Contains(hosted, "MONITORIO=") || strings.Contains(hosted, shimPath) {
		t.Fatalf("--hostname sync wrote a shim:\n%s", hosted)
	}
	if !strings.Contains(hosted, "cronitor exec ") {
		t.Fatalf("--hostname sync did not stay exec style:\n%s", hosted)
	}
}

func TestSyncMultipleShellsStayExec(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	path := writeCron(t, "SHELL=/bin/bash\nSHELL=/bin/zsh\n0 * * * * /bin/true\n")
	stderr := runSync(t, serverURL, path, "--no-stdout")
	got := readCron(t, path)
	if !strings.Contains(stderr, "more than one SHELL=") {
		t.Fatalf("expected a multiple-shell notice:\n%s", stderr)
	}
	if strings.Contains(got, "MONITORIO=") || strings.Contains(got, shimPath) || strings.Contains(got, "CRONITOR_REAL_SHELL=") {
		t.Fatalf("multiple SHELL= installed a shim:\n%s", got)
	}
	if !hasLine(got, "SHELL=/bin/bash") || !hasLine(got, "SHELL=/bin/zsh") {
		t.Fatalf("SHELL lines were rewritten:\n%s", got)
	}
	if !strings.Contains(got, "cronitor exec --no-stdout ") || strings.Contains(got, "--no-stdout exec") {
		t.Fatalf("multiple SHELL= did not put --no-stdout after exec:\n%s", got)
	}
}

func TestSyncDryRunDoesNotInstallWrapper(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	os.RemoveAll(filepath.Dir(shimPath))
	path := writeCron(t, "0 * * * * /bin/true\n")
	runSync(t, serverURL, path, "--dry-run")
	if _, err := os.Stat(shimPath); !os.IsNotExist(err) {
		t.Fatalf("dry-run created %s", shimPath)
	}
	if _, err := os.Stat("/etc/cronitor"); err == nil {
		// Present only if something else created it. The override path must stay absent.
	}
	body := readCron(t, path)
	if strings.Contains(body, "MONITORIO=") {
		t.Fatalf("dry-run rewrote the crontab:\n%s", body)
	}
}

func TestSyncConvertCompoundRoundTrip(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	app := t.TempDir()
	marker := filepath.Join(app, "ran")
	script := filepath.Join(app, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf ran > \"$1\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	command := "cd " + app + " && ./run.sh " + marker
	original := "SHELL=/bin/sh\n0 1 * * * cronitor exec k1 " + shellquote.Join(command) + "\n"
	path := writeCron(t, original)

	runSync(t, serverURL, path, "--convert-to-shim")
	shimmed := readCron(t, path)
	want := "CRONITOR_REAL_SHELL=/bin/sh\nSHELL=" + shimPath + "\n0 1 * * * MONITORIO=k1 " + command + "\n"
	if shimmed != want {
		t.Fatalf("convert did not unquote the compound command\nwant:\n%s\ngot:\n%s", want, shimmed)
	}
	runLine := exec.Command(shimPath, "-c", "MONITORIO=k1 "+command)
	runLine.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "CRONITOR_API_KEY=")
	if out, err := runLine.CombinedOutput(); err != nil {
		t.Fatalf("shim line failed: %v\n%s", err, out)
	}
	if got := readCron(t, marker); got != "ran" {
		t.Fatalf("shim line ran as %q", got)
	}
	os.Remove(marker)

	runSync(t, serverURL, path, "--exec-style")
	if got := readCron(t, path); got != original {
		t.Fatalf("revert is not the #65 line\nwant:\n%s\ngot:\n%s", original, got)
	}
	stubDir := t.TempDir()
	stub := "#!/bin/sh\nseen=0\nwhile [ $# -gt 0 ]; do\n  if [ \"$seen\" = 0 ]; then [ \"$1\" = exec ] && seen=1; shift; continue; fi\n  case \"$1\" in -*) shift ;; *) shift; break ;; esac\ndone\nif [ $# -eq 1 ]; then exec sh -c \"$1\"; fi\n"
	if err := os.WriteFile(filepath.Join(stubDir, "cronitor"), []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	field := strings.TrimPrefix(strings.TrimSpace(original), "SHELL=/bin/sh\n")
	field = strings.TrimPrefix(field, "0 1 * * * ")
	execLine := exec.Command("sh", "-c", field)
	execLine.Env = append(os.Environ(), "PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := execLine.CombinedOutput(); err != nil {
		t.Fatalf("exec line failed: %v\n%s\nfield: %s", err, out, field)
	}
	if got := readCron(t, marker); got != "ran" {
		t.Fatalf("exec line ran as %q", got)
	}
}

func TestSyncNoStdoutRewritesShimLines(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	path := writeCron(t, strings.Join([]string{
		"CRONITOR_REAL_SHELL=/bin/bash",
		"SHELL=" + shimPath,
		"0 * * * * MONITORIO=abc /bin/true",
	}, "\n"))
	stderr := runSync(t, serverURL, path, "--no-stdout")
	if !strings.Contains(stderr, "monitor abc") || !strings.Contains(stderr, "--no-stdout") {
		t.Fatalf("expected a rewrite notice:\n%s", stderr)
	}
	got := readCron(t, path)
	want := "SHELL=/bin/bash\n0 * * * * cronitor exec --no-stdout abc /bin/true\n"
	if got != want {
		t.Fatalf("shim line was not rewritten\nwant:\n%s\ngot:\n%s", want, got)
	}
	if strings.Contains(got, "MONITORIO=") || strings.Contains(got, "CRONITOR_REAL_SHELL=") || strings.Contains(got, shimPath) {
		t.Fatalf("shim left behind:\n%s", got)
	}
	if again := runSync(t, serverURL, path, "--no-stdout"); again != "" {
		t.Fatalf("second --no-stdout still noticed:\n%s", again)
	}
	if second := readCron(t, path); second != got {
		t.Fatalf("rewrite not stable\n--- first\n%s\n--- second\n%s", got, second)
	}
}

func TestSyncShellAfterShimIsLeftAlone(t *testing.T) {
	serverURL, shimPath := withSyncFixture(t)
	body := strings.Join([]string{
		"CRONITOR_REAL_SHELL=/bin/zsh",
		"SHELL=" + shimPath,
		"0 * * * * MONITORIO=abc /bin/true",
		"SHELL=/bin/bash",
	}, "\n") + "\n"
	path := writeCron(t, body)
	stderr := runSync(t, serverURL, path)
	if !strings.Contains(stderr, "SHELL= line follows") {
		t.Fatalf("expected a notice:\n%s", stderr)
	}
	if got := readCron(t, path); got != body {
		t.Fatalf("SHELL after the shim was rewritten\nwant:\n%s\ngot:\n%s", body, got)
	}
}

func TestSyncConvertRedactsAPIKey(t *testing.T) {
	serverURL, _ := withSyncFixture(t)
	const secret = "supersecretvalue"
	const secretEq = "othersupersecret"
	const secretShort = "thirdsecret"
	path := writeCron(t, strings.Join([]string{
		"0 9 * * * cronitor --api-key " + secret + " exec k9 /bin/true",
		"0 8 * * * cronitor --api-key=" + secretEq + " exec k8 /bin/true",
		"0 7 * * * cronitor -k " + secretShort + " exec k7 /bin/true",
	}, "\n"))
	stderr := runSync(t, serverURL, path, "--convert-to-shim")
	for _, leak := range []string{secret, secretEq, secretShort} {
		if strings.Contains(stderr, leak) {
			t.Fatalf("convert-skip notice leaked %q:\n%s", leak, stderr)
		}
	}
	if !strings.Contains(stderr, "--api-key") || !strings.Contains(stderr, "-k") || !strings.Contains(stderr, "<redacted>") {
		t.Fatalf("notice dropped the flag name:\n%s", stderr)
	}
	if !strings.Contains(stderr, "monitor k9") {
		t.Fatalf("notice is not keyed by monitor code:\n%s", stderr)
	}
	got := readCron(t, path)
	if strings.Contains(got, "MONITORIO=k9") || strings.Contains(got, "MONITORIO=k8") || strings.Contains(got, "MONITORIO=k7") {
		t.Fatalf("keyed line was converted:\n%s", got)
	}
}

func TestSyncNewLinesKeepVerbatimCommand(t *testing.T) {
	serverURL, _ := withSyncFixture(t)
	commands := []string{
		`echo "a  b"   >  /tmp/out`,
		`"/opt/my app/run.sh"`,
	}
	body := "0 * * * * " + commands[0] + "\n15 * * * * " + commands[1] + "\n"
	path := writeCron(t, body)
	runSync(t, serverURL, path)
	got := readCron(t, path)
	for _, cmd := range commands {
		if !strings.Contains(got, "MONITORIO=") || !strings.Contains(got, " "+cmd) {
			t.Fatalf("new line lost verbatim bytes %q\n%s", cmd, got)
		}
	}
}

func TestInvokedExecutableKeepsSymlink(t *testing.T) {
	got := invokedExecutable("cronitor", func(string) (string, error) {
		return "/opt/homebrew/bin/cronitor", nil
	}, os.Getwd)
	if got != "/opt/homebrew/bin/cronitor" {
		t.Fatalf("PATH lookup = %q", got)
	}
	got = invokedExecutable("/opt/homebrew/bin/cronitor", func(string) (string, error) {
		t.Fatal("absolute path must not be resolved")
		return "", nil
	}, os.Getwd)
	if got != "/opt/homebrew/bin/cronitor" {
		t.Fatalf("absolute = %q", got)
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "real-cronitor")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "cronitor")
	if err := os.Symlink(target, link); err != nil {
		t.Skip(err)
	}
	got = invokedExecutable(link, func(string) (string, error) {
		t.Fatal("absolute path must not call LookPath")
		return "", nil
	}, func() (string, error) {
		t.Fatal("absolute path must not call Getwd")
		return "", nil
	})
	if got != link {
		t.Fatalf("symlink resolved to %q, want %q", got, link)
	}
	got = invokedExecutable("cronitor", func(string) (string, error) {
		return link, nil
	}, os.Getwd)
	if got != link {
		t.Fatalf("LookPath symlink resolved to %q, want %q", got, link)
	}
}

func TestValidateShellShimFlags(t *testing.T) {
	oldExec, oldConvert := execStyle, convertToShim
	t.Cleanup(func() {
		execStyle = oldExec
		convertToShim = oldConvert
	})
	execStyle, convertToShim = true, true
	if err := validateShellShimFlags(); err == nil {
		t.Fatal("expected mutually exclusive flags to error")
	}
	execStyle, convertToShim = false, true
	if err := validateShellShimFlags(); err != nil {
		t.Fatal(err)
	}
}
