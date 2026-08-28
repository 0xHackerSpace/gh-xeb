package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
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
	}
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
