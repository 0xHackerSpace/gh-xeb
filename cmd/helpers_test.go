package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
	"github.com/0xHackerSpace/gh-cli-extension/internal/vault"
)

// fakeREST serves canned JSON per path. Anything not in bodies is an error, so
// a test that reaches an unexpected endpoint fails loudly instead of silently
// decoding a zero value.
type fakeREST struct {
	bodies map[string]string
	errs   map[string]error
	header http.Header
}

func (f fakeREST) Get(path string, response interface{}) error {
	if err, ok := f.errs[path]; ok {
		return err
	}
	body, ok := f.bodies[path]
	if !ok {
		return fmt.Errorf("fakeREST: no canned response for %q", path)
	}
	return json.Unmarshal([]byte(body), response)
}

func (f fakeREST) Request(_ string, path string, _ io.Reader) (*http.Response, error) {
	if err, ok := f.errs[path]; ok {
		return nil, err
	}
	body, ok := f.bodies[path]
	if !ok {
		return nil, fmt.Errorf("fakeREST: no canned response for %q", path)
	}
	header := f.header
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

// testDeps returns dependencies that never touch the network. Tests override
// only the fields they care about.
func testDeps() Deps {
	return Deps{
		NewRESTClient: func() (gh.RESTClient, error) { return fakeREST{}, nil },
		CurrentAuth: func() gh.Auth {
			return gh.Auth{Host: "github.com", Source: "keyring", HasToken: true}
		},
		CurrentRepo: func() (gh.Repo, error) {
			return gh.Repo{Host: "github.com", Owner: "0xHackerSpace", Name: "gh-cli-extension"}, nil
		},
		CLIVersion: func() (string, error) { return "2.4.0", nil },
		Getenv:     func(string) string { return "" },

		GitHubToken: func() (string, error) { return "gho_fake", nil },
		NewVaultClient: func(context.Context, vault.Options) (vault.Client, error) {
			return fakeVault{}, nil
		},
	}
}

// fakeVault is a Vault server that never was. Zero values are useful: a test
// that only cares about mounts leaves the rest alone.
type fakeVault struct {
	addr      string
	health    vault.Health
	healthErr error
	token     vault.Token
	tokenErr  error
	mounts    []vault.Mount
	mountsErr error
	caps      map[string][]string
	capsErr   error
	secrets   map[string]vault.Secret
	secretErr error
}

func (f fakeVault) Address() string {
	if f.addr == "" {
		return "https://vault.example.com:8200"
	}
	return f.addr
}

func (f fakeVault) Health(context.Context) (vault.Health, error) {
	return f.health, f.healthErr
}

func (f fakeVault) Token(context.Context) (vault.Token, error) {
	return f.token, f.tokenErr
}

func (f fakeVault) Mounts(context.Context) ([]vault.Mount, error) {
	return f.mounts, f.mountsErr
}

func (f fakeVault) Capabilities(_ context.Context, path string) ([]string, error) {
	if f.capsErr != nil {
		return nil, f.capsErr
	}
	return f.caps[path], nil
}

func (f fakeVault) ReadSecret(_ context.Context, path string) (vault.Secret, error) {
	if f.secretErr != nil {
		return vault.Secret{}, f.secretErr
	}
	s, ok := f.secrets[path]
	if !ok {
		return vault.Secret{}, fmt.Errorf("no secret at %s", path)
	}
	return s, nil
}

// vaultDeps returns deps whose Vault client is the given fake.
func vaultDeps(v vault.Client) Deps {
	deps := testDeps()
	deps.NewVaultClient = func(context.Context, vault.Options) (vault.Client, error) {
		return v, nil
	}
	return deps
}

func runRoot(t *testing.T, deps Deps, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	root := NewRootCmd(deps)
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)

	err := root.Execute()
	return out.String(), err
}

// fakeClient adapts a fakeREST into the Deps.NewRESTClient signature.
func fakeClient(c fakeREST) func() (gh.RESTClient, error) {
	return func() (gh.RESTClient, error) { return c, nil }
}
