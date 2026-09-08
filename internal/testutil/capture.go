package testutil

import (
	"bytes"
	"io"
	"os"
	"sync"
)

// CaptureStdout captures everything written to os.Stdout while fn executes.
func CaptureStdout(fn func()) string {
	stdout, _ := CaptureStdoutStderr(fn)
	return stdout
}

// CaptureStdoutStderr captures os.Stdout and os.Stderr while fn executes.
func CaptureStdoutStderr(fn func()) (stdout, stderr string) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr

	var wg sync.WaitGroup
	var outBuf, errBuf bytes.Buffer
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(&outBuf, rOut)
	}()
	go func() {
		defer wg.Done()
		io.Copy(&errBuf, rErr)
	}()

	fn()

	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	wg.Wait()
	rOut.Close()
	rErr.Close()
	return outBuf.String(), errBuf.String()
}
