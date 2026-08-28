// Command gh-cli-extension is a GitHub CLI extension.
//
// It is installed with `gh extension install 0xHackerSpace/gh-cli-extension`
// and invoked as `gh cli-extension <subcommand>`.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/0xHackerSpace/gh-cli-extension/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		// Cobra already printed usage errors; only surface runtime failures.
		var silent cmd.SilentError
		if !errors.As(err, &silent) {
			fmt.Fprintf(os.Stderr, "gh cli-extension: %v\n", err)
		}
		os.Exit(1)
	}
}
