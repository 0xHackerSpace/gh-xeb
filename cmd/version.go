package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version is overridden at release time via
// -ldflags="-X github.com/0xHackerSpace/gh-xeb/cmd.version=v1.2.3".
var version = ""

func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the extension version",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprintln(c.OutOrStdout(), resolveVersion())
			return nil
		},
	}
}
