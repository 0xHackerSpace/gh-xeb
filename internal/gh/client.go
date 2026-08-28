// Package gh wraps go-gh so commands depend on small interfaces instead of
// concrete API clients, which keeps them testable without network access.
package gh

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	gogh "github.com/cli/go-gh/v2"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/repository"
)

// RESTClient is the slice of go-gh's REST client that this extension uses.
type RESTClient interface {
	// Get decodes a JSON response body into response.
	Get(path string, response interface{}) error
	// Request returns the raw response, for the cases where the headers
	// matter as much as the body (token scopes, rate limits, pagination).
	Request(method string, path string, body io.Reader) (*http.Response, error)
}

// NewRESTClient returns a REST client authenticated with the user's gh
// credentials (gh auth token, GH_TOKEN, or GITHUB_TOKEN).
func NewRESTClient() (RESTClient, error) {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("creating REST client: %w", err)
	}
	return client, nil
}

// Repo identifies a GitHub repository.
type Repo struct {
	Host  string
	Owner string
	Name  string
}

// String renders the repository as owner/name.
func (r Repo) String() string { return r.Owner + "/" + r.Name }

// CurrentRepo resolves the repository for the current working directory, the
// same way `gh` does (honouring the GH_REPO override and git remotes).
func CurrentRepo() (Repo, error) {
	r, err := repository.Current()
	if err != nil {
		return Repo{}, fmt.Errorf("resolving current repository: %w", err)
	}
	return Repo{Host: r.Host, Owner: r.Owner, Name: r.Name}, nil
}

// Auth describes how the user is authenticated, without exposing the token
// itself — nothing in this extension needs to read the secret.
type Auth struct {
	Host     string
	Source   string
	HasToken bool
}

// CurrentAuth reports the default host and where its token comes from.
func CurrentAuth() Auth {
	host, _ := auth.DefaultHost()
	token, source := auth.TokenForHost(host)
	return Auth{Host: host, Source: describeSource(source), HasToken: token != ""}
}

// describeSource turns go-gh's internal source keys into something a user can
// act on. Unknown values are passed through rather than hidden.
func describeSource(source string) string {
	switch source {
	case "oauth_token":
		return "gh config"
	case "gh":
		return "keyring"
	case "default":
		return "none"
	default:
		// An environment variable name, e.g. GH_TOKEN.
		return source
	}
}

// CLIVersion returns the version of the gh binary on PATH.
//
// This is one of the few places where shelling out to gh is correct: the
// version of the host CLI is not exposed through any API.
func CLIVersion() (string, error) {
	stdout, stderr, err := gogh.Exec("--version")
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("running gh --version: %w: %s", err, msg)
		}
		return "", fmt.Errorf("running gh --version: %w", err)
	}
	return parseVersion(stdout.String()), nil
}

// parseVersion pulls the version out of `gh version 2.4.0 (2022-03-23)`.
func parseVersion(output string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	fields := strings.Fields(line)
	if len(fields) >= 3 && fields[0] == "gh" && fields[1] == "version" {
		return fields[2]
	}
	return strings.TrimSpace(line)
}
