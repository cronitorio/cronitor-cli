package main

import (
	"cronitor/cmd"
	"os"
)

func main() {
	// Ensure that flags on `exec` commands are not parsed by Cobra
	// Inject a `--` param
	commandIndex := 0
	argsEscaped := false
	for idx, arg := range os.Args {
		if arg == "exec" {
			commandIndex = idx + 2
		}

		if commandIndex > 0 && arg == "--" {
			argsEscaped = true
		}
	}

	if commandIndex > 0 && !argsEscaped {
		os.Args = append(os.Args, "")
		copy(os.Args[commandIndex+1:], os.Args[commandIndex:])
		os.Args[commandIndex] = "--"
	}

	cmd.Execute()
}
