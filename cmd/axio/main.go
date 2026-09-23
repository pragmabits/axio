// Command axio reads the logs written by the axio logging library.
//
// Install it with:
//
//	go install github.com/pragmabits/axio/cmd/axio@latest
//
// and run axio --help for the commands.
package main

import (
	"fmt"
	"os"

	"github.com/pragmabits/axio/internal/cli"
)

func main() {
	if err := cli.NewRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "axio: %v\n", err)
		os.Exit(1)
	}
}
