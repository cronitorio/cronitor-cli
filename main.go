package main

import (
	"github.com/cronitorio/cronitor-cli/cmd"
	"os"
)

func init() {

}

func main() {
	// Ensure that flags on `exec` commands are not parsed by Cobra
	// Inject a `--` param
	commandIndex := 0
	argsEscaped := false
	for idx, arg := range os.Args {
		if arg == "exec" && commandIndex == 0 {
			// The first "exec" we come across is the one we care about.
			// After we find it we continue looking at the rest of the args but we have our commandIndex set
			commandIndex = idx + 2
		}

		if arg == "help" && commandIndex == 0 {
			break
		}

		if commandIndex > 0 && arg == "--" {
			argsEscaped = true
		}
	}

	if commandIndex > 0 && !argsEscaped && len(os.Args) > commandIndex+1 {
		os.Args = append(os.Args, "")
		copy(os.Args[commandIndex+1:], os.Args[commandIndex:])
		os.Args[commandIndex] = "--"
	}

	cmd.Execute()
}
