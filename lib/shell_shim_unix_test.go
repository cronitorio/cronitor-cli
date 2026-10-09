//go:build !windows

package lib

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// closeInheritedFDs keeps a leaked fd 3 from looking like cron's own fd.
// os/exec does not close fds >= 3, so the wrapper would skip monitoring.
func closeInheritedFDs() {
	f, err := os.Open("/proc/self/fd")
	if err != nil {
		return
	}
	defer f.Close()
	names, _ := f.Readdirnames(-1)
	self := int(f.Fd())
	for _, name := range names {
		fd, err := strconv.Atoi(name)
		if err != nil || fd < 3 || fd == self {
			continue
		}
		syscall.CloseOnExec(fd)
	}
}

func TestOpenFd3SkipsCronitor(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "invoked")
	bin := filepath.Join(dir, "cronitor")
	stub := "#!/bin/sh\nprintf RAN >> \"$LOG\"\nexit 42\n"
	if err := os.WriteFile(bin, []byte(stub), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), bin)
	if err != nil {
		t.Fatal(err)
	}
	fdPath := filepath.Join(dir, "fd3")
	f, err := os.Create(fdPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 printf JOB >&3")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "LOG="+log)
	cmd.ExtraFiles = []*os.File{f}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exit=%v\n%s", err, out)
	}
	if got := readFile(t, fdPath); got != "JOB" {
		t.Fatalf("fd 3 = %q, want the caller's file", got)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("open fd 3 still ran cronitor: %s", readFile(t, log))
	}
}

func TestWrapperSignalRemovesTempDir(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hung")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0755); err != nil {
		t.Fatal(err)
	}
	wrapper, err := InstallShimWrapper(filepath.Join(dir, "cronitor-shell"), script)
	if err != nil {
		t.Fatal(err)
	}
	root := os.Getenv("TMPDIR")
	if root == "" {
		root = os.TempDir()
	}
	cmd := exec.Command(wrapper, "-c", "MONITORIO=k1 printf x")
	cmd.Env = append(os.Environ(), "CRONITOR_REAL_SHELL=/bin/sh", "TMPDIR="+root)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		cmd.Process.Kill()
		t.Fatal("SIGTERM did not stop the wrapper")
	}
	if leftovers, _ := filepath.Glob(filepath.Join(root, "cronitor-shim.*")); len(leftovers) > 0 {
		t.Fatalf("signal left %s", leftovers)
	}
}
