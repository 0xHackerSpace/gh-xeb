// Command gh-xeb is a GitHub CLI extension.
//
// It is installed with `gh extension install 0xHackerSpace/gh-xeb`
// and invoked as `gh xeb <subcommand>`.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xHackerSpace/gh-xeb/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Cobra already printed usage errors; only surface runtime failures.
		var silent cmd.SilentError
		if !errors.As(err, &silent) {
			fmt.Fprintf(os.Stderr, "gh xeb: %v\n", err)
		}
		os.Exit(1)
	}
}
