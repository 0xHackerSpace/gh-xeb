package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type user struct {
	Login string `json:"login"`
	Name  string `json:"name"`
	ID    int64  `json:"id"`
}

func newWhoamiCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the authenticated GitHub user",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, err := deps.NewRESTClient()
			if err != nil {
				return err
			}

			var u user
			if err := client.Get("user", &u); err != nil {
				return fmt.Errorf("fetching authenticated user: %w", err)
			}

			name := u.Name
			if name == "" {
				name = "no display name"
			}
			fmt.Fprintf(c.OutOrStdout(), "You are @%s (%s).\n", u.Login, name)
			return nil
		},
	}
}
