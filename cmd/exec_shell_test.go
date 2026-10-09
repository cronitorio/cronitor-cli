package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestInterpreterForExec(t *testing.T) {
	statFile := func(string) (os.FileInfo, error) {
		return fakeFileInfo{mode: 0755}, nil
	}
	statDir := func(string) (os.FileInfo, error) {
		return fakeFileInfo{dir: true, mode: 0755}, nil
	}
	statMissing := func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	statPlain := func(string) (os.FileInfo, error) {
		return fakeFileInfo{mode: 0644}, nil
	}
	statBash := func(name string) (os.FileInfo, error) {
		if name == "/bin/bash" {
			return fakeFileInfo{mode: 0755}, nil
		}
		return nil, os.ErrNotExist
	}

	abs := testAbsPath("custom-shell")

	tests := []struct {
		name     string
		goos     string
		shell    string
		stat     func(string) (os.FileInfo, error)
		wantName string
		wantFlag string
	}{
		{name: "windows ignores SHELL", goos: "windows", shell: abs, stat: statFile, wantName: "powershell.exe", wantFlag: "-Command"},
		{name: "windows ignores empty SHELL", goos: "windows", shell: "", stat: statMissing, wantName: "powershell.exe", wantFlag: "-Command"},
		{name: "absolute executable", goos: "linux", shell: abs, stat: statFile, wantName: abs, wantFlag: "-c"},
		{name: "trims shell", goos: "linux", shell: "  " + abs + "  ", stat: statFile, wantName: abs, wantFlag: "-c"},
		{name: "relative falls back to bash", goos: "linux", shell: "zsh", stat: statBash, wantName: "bash", wantFlag: "-c"},
		{name: "missing falls back to bash", goos: "linux", shell: abs, stat: statBash, wantName: "bash", wantFlag: "-c"},
		{name: "directory falls back to bash", goos: "linux", shell: abs, stat: func(name string) (os.FileInfo, error) {
			if name == "/bin/bash" {
				return fakeFileInfo{mode: 0755}, nil
			}
			return statDir(name)
		}, wantName: "bash", wantFlag: "-c"},
		{name: "non-executable falls back to bash", goos: "linux", shell: abs, stat: func(name string) (os.FileInfo, error) {
			if name == "/bin/bash" {
				return fakeFileInfo{mode: 0755}, nil
			}
			return statPlain(name)
		}, wantName: "bash", wantFlag: "-c"},
		{name: "empty falls back to bash", goos: "linux", shell: "", stat: statBash, wantName: "bash", wantFlag: "-c"},
		{name: "no bash falls back to sh", goos: "linux", shell: "", stat: statMissing, wantName: "sh", wantFlag: "-c"},
		{name: "unusable shell and no bash", goos: "linux", shell: "bash", stat: statMissing, wantName: "sh", wantFlag: "-c"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			stat := func(name string) (os.FileInfo, error) {
				calls++
				return tc.stat(name)
			}
			gotName, gotFlag := interpreterForExec(tc.goos, tc.shell, stat)
			if gotName != tc.wantName || gotFlag != tc.wantFlag {
				t.Fatalf("interpreterForExec(%q, %q) = (%q, %q), want (%q, %q)", tc.goos, tc.shell, gotName, gotFlag, tc.wantName, tc.wantFlag)
			}
			if tc.goos == "windows" && calls != 0 {
				t.Fatalf("windows path called stat %d times", calls)
			}
		})
	}
}

func TestMakeSubcommandExecShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("SHELL", `C:\Windows\System32\bash.exe`)
		cmd := makeSubcommandExec("Write-Output hi")
		if len(cmd.Args) < 2 || cmd.Args[0] != "powershell.exe" || cmd.Args[1] != "-Command" {
			t.Fatalf("windows args = %q", cmd.Args)
		}
		return
	}

	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Fatal("/bin/sh missing")
	}
	t.Setenv("SHELL", "/bin/sh")
	cmd := makeSubcommandExec("true")
	if cmd.Args[0] != "/bin/sh" || cmd.Args[1] != "-c" || cmd.Args[2] != "true" {
		t.Fatalf("SHELL=/bin/sh args = %q", cmd.Args)
	}

	t.Setenv("SHELL", "zsh")
	cmd = makeSubcommandExec("true")
	if _, err := os.Stat("/bin/bash"); err == nil {
		if cmd.Args[0] != "bash" || cmd.Args[1] != "-c" {
			t.Fatalf("relative SHELL should fall back to bash, args = %q", cmd.Args)
		}
	} else if cmd.Args[0] != "sh" {
		t.Fatalf("relative SHELL should fall back to sh, args = %q", cmd.Args)
	}

	t.Setenv("SHELL", "")
	cmd = makeSubcommandExec("true")
	if _, err := os.Stat("/bin/bash"); err == nil {
		if cmd.Args[0] != "bash" {
			t.Fatalf("empty SHELL args = %q", cmd.Args)
		}
	}
}

type fakeFileInfo struct {
	dir  bool
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "f" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.dir }
func (f fakeFileInfo) Sys() any           { return nil }

func testAbsPath(name string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(`C:\`, name)
	}
	return "/" + name
}
