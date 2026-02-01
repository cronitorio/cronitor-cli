package testutil

import (
	"bytes"
	"io"
	"os"
)

// CaptureStdout captures everything written to os.Stdout while fn executes.
func CaptureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()

	return buf.String()
}
