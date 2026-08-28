package cmd

import (
	"os"

	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
)

// Deps holds everything the command tree reaches the outside world through.
// Commands take it as a constructor parameter so tests can substitute fakes
// without touching global state — see docs/adr/0009-inject-dependencies.md.
type Deps struct {
	NewRESTClient func() (gh.RESTClient, error)
	CurrentAuth   func() gh.Auth
	CurrentRepo   func() (gh.Repo, error)
	CLIVersion    func() (string, error)
	Getenv        func(key string) string
}

// DefaultDeps wires the real implementations.
func DefaultDeps() Deps {
	return Deps{
		NewRESTClient: gh.NewRESTClient,
		CurrentAuth:   gh.CurrentAuth,
		CurrentRepo:   gh.CurrentRepo,
		CLIVersion:    gh.CLIVersion,
		Getenv:        os.Getenv,
	}
}
