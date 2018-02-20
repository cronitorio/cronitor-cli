package main

import (
	"cronitor/cmd"
	"os"
	"github.com/getsentry/raven-go"
)

var version = "1.11.0"

func init() {
    raven.SetDSN("***REMOVED***")
    raven.SetRelease(version)
    cmd.Version = version
}

func main() {
	// Ensure that flags on `exec` commands are not parsed by Cobra
	// Inject a `--` param
	commandIndex := 0
	argsEscaped := false
	for idx, arg := range os.Args {
		if arg == "exec" {
			commandIndex = idx + 2
		}

		if arg == "help" && commandIndex == 0 {
			break
		}

		if commandIndex > 0 && arg == "--" {
			argsEscaped = true
		}
	}

	if commandIndex > 0 && !argsEscaped && len(os.Args) > commandIndex + 1 {
		os.Args = append(os.Args, "")
		copy(os.Args[commandIndex+1:], os.Args[commandIndex:])
		os.Args[commandIndex] = "--"
	}

	raven.CapturePanicAndWait(cmd.Execute, nil)
}
