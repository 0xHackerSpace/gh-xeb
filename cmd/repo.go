package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRepoCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "repo",
		Short: "Print the repository resolved from the current directory",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			repo, err := deps.CurrentRepo()
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "%s/%s\n", repo.Host, repo)
			return nil
		},
	}
}
