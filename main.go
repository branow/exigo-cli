// Command exigo is the entry point for exigo-cli.
package main

import (
	"fmt"
	"os"

	"exigo-cli/cmd"
	"exigo-cli/internal/cmdutil"
)

func main() {
	err := cmd.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(cmdutil.ExitCode(err))
}
