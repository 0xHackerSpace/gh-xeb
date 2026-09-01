// Package cmd wires up the cobra command tree for the extension.
package cmd

import (
	"github.com/spf13/cobra"
)

// SilentError marks an error whose message was already shown to the user, so
// the entrypoint sets a non-zero exit code without printing anything further.
type SilentError struct{ Err error }

func (e SilentError) Error() string { return e.Err.Error() }
func (e SilentError) Unwrap() error { return e.Err }

// NewRootCmd builds the root command against the given dependencies. It is
// exported so tests can execute the tree with fakes, against a buffer instead
// of the process streams.
func NewRootCmd(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "xeb <command>",
		Short: "Playground extension for exploring the gh CLI extension API",
		Long: `xeb is a GitHub CLI extension used to explore what
extensions can do: talking to the REST and GraphQL APIs through go-gh,
resolving the current repository, and shelling out to gh itself.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Without a subcommand, print help rather than failing.
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}

	root.AddCommand(newWhoamiCmd(deps))
	root.AddCommand(newRepoCmd(deps))
	root.AddCommand(newDoctorCmd(deps))
	root.AddCommand(newVaultCmd(deps))
	root.AddCommand(newBackstageCmd(deps))
	root.AddCommand(newHarnessCmd(deps))
	root.AddCommand(newVersionCmd())

	return root
}

// Execute runs the root command against os.Args.
func Execute() error {
	return NewRootCmd(DefaultDeps()).Execute()
}
