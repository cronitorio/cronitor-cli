package main

import (
	"cronitor/cmd"
	"os"
	"github.com/getsentry/raven-go"
)

func init() {
    raven.SetDSN("***REMOVED***")
    raven.SetRelease(cmd.Version)
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

	cmd.Execute()
	//raven.CapturePanicAndWait(cmd.Execute, nil)
}
