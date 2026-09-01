package cmd

import (
	"errors"
	"testing"

	"github.com/0xHackerSpace/gh-xeb/internal/gh"
)

func TestRepoPrintsHostAndSlug(t *testing.T) {
	deps := testDeps()

	out, err := runRoot(t, deps, "repo")
	if err != nil {
		t.Fatalf("repo returned error: %v", err)
	}
	if want := "github.com/0xHackerSpace/gh-xeb\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRepoPropagatesResolutionError(t *testing.T) {
	deps := testDeps()
	deps.CurrentRepo = func() (gh.Repo, error) {
		return gh.Repo{}, errors.New("no git remotes found")
	}

	if _, err := runRoot(t, deps, "repo"); err == nil {
		t.Fatal("expected an error when the repository cannot be resolved")
	}
}
