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

	"github.com/0xHackerSpace/gh-cli-extension/internal/backstage"
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
		NewBackstageClient: func(backstage.Config) (backstage.Client, error) {
			return &fakeBackstage{}, nil
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

// fakeBackstage is a catalog that never was. It records the queries it is
// given so a test can assert on the filters a command built, which is where
// the interesting logic lives.
type fakeBackstage struct {
	addr string

	facets    map[string][]backstage.Facet
	facetsErr error

	page     backstage.EntityPage
	pageErr  error
	entities []backstage.Entity
	allErr   error

	byRef     map[string]backstage.Entity
	entityErr error

	// queries records every EntityQuery the command passed in, in order.
	queries []backstage.EntityQuery
	// refs records every EntityRef looked up.
	refs []backstage.EntityRef

	// Scaffolder state.
	scaffolded  []scaffoldCall
	scaffoldErr error
	statuses    []string
	taskCalls   int
	taskErr     error
	events      []backstage.TaskEvent
	eventsErr   error
}

// scaffoldCall is one submitted run.
type scaffoldCall struct {
	Ref    backstage.EntityRef
	Values map[string]interface{}
}

func (f *fakeBackstage) BaseURL() string {
	if f.addr == "" {
		return "https://backstage.example.com"
	}
	return f.addr
}

func (f *fakeBackstage) Ping(ctx context.Context) error {
	_, err := f.Facets(ctx, "kind")
	return err
}

func (f *fakeBackstage) Facets(_ context.Context, _ ...string) (map[string][]backstage.Facet, error) {
	return f.facets, f.facetsErr
}

func (f *fakeBackstage) Entities(_ context.Context, q backstage.EntityQuery) (backstage.EntityPage, error) {
	f.queries = append(f.queries, q)
	return f.page, f.pageErr
}

func (f *fakeBackstage) AllEntities(_ context.Context, q backstage.EntityQuery) ([]backstage.Entity, error) {
	f.queries = append(f.queries, q)
	return f.entities, f.allErr
}

// scaffolded records the templates submitted and with which values, so a test
// can assert on the payload without a server. Each Task/TaskEvents pair is
// served from the scripted state below.
func (f *fakeBackstage) Scaffold(_ context.Context, ref backstage.EntityRef, values map[string]interface{}) (backstage.Task, error) {
	f.scaffolded = append(f.scaffolded, scaffoldCall{Ref: ref, Values: values})
	if f.scaffoldErr != nil {
		return backstage.Task{}, f.scaffoldErr
	}
	return backstage.Task{ID: "task-1", Status: "open", TemplateRef: ref.String(), URL: f.TaskURL("task-1")}, nil
}

// Task walks f.statuses one entry per call, staying on the last one, so a test
// can script a run that is open, then processing, then done.
func (f *fakeBackstage) Task(_ context.Context, id string) (backstage.Task, error) {
	if f.taskErr != nil {
		return backstage.Task{}, f.taskErr
	}
	status := "completed"
	if len(f.statuses) > 0 {
		if f.taskCalls < len(f.statuses) {
			status = f.statuses[f.taskCalls]
		} else {
			status = f.statuses[len(f.statuses)-1]
		}
	}
	f.taskCalls++
	return backstage.Task{ID: id, Status: status, URL: f.TaskURL(id)}, nil
}

func (f *fakeBackstage) TaskEvents(_ context.Context, _ string, after int) ([]backstage.TaskEvent, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	var out []backstage.TaskEvent
	for _, event := range f.events {
		if event.ID > after {
			out = append(out, event)
		}
	}
	return out, nil
}

func (f *fakeBackstage) TaskURL(id string) string { return f.BaseURL() + "/create/tasks/" + id }

func (f *fakeBackstage) Entity(_ context.Context, ref backstage.EntityRef) (backstage.Entity, error) {
	f.refs = append(f.refs, ref)
	if f.entityErr != nil {
		return backstage.Entity{}, f.entityErr
	}
	entity, ok := f.byRef[ref.String()]
	if !ok {
		return backstage.Entity{}, fmt.Errorf("no entity %s in the catalog: %w", ref, backstage.ErrNotFound)
	}
	return entity, nil
}

// backstageDeps returns deps whose Backstage client is the given fake.
func backstageDeps(c backstage.Client) Deps {
	deps := testDeps()
	deps.NewBackstageClient = func(backstage.Config) (backstage.Client, error) { return c, nil }
	return deps
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
