// Package vault wraps the official Vault SDK behind a narrow interface, so
// commands depend on this package's types rather than on api.Client and can be
// tested without a Vault server.
package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/vault/api"
)

// Health is the subset of Vault's health endpoint worth showing a user.
type Health struct {
	Version     string `json:"version"`
	ClusterName string `json:"clusterName,omitempty"`
	Initialized bool   `json:"initialized"`
	Sealed      bool   `json:"sealed"`
	Standby     bool   `json:"standby"`
}

// Token describes the caller's own token. It never carries the token value.
type Token struct {
	DisplayName string   `json:"displayName"`
	Policies    []string `json:"policies"`
	TTL         int      `json:"ttlSeconds"`
	Renewable   bool     `json:"renewable"`
	EntityID    string   `json:"entityId,omitempty"`
	Accessor    string   `json:"accessor,omitempty"`
	// AuthPath is where this token came from when we logged in ourselves.
	AuthPath string `json:"authPath,omitempty"`
}

// Mount is one secret engine visible to the caller.
type Mount struct {
	Path         string   `json:"path"`
	Type         string   `json:"type"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities"`
}

// Client is the slice of Vault this extension uses.
type Client interface {
	Address() string
	Health(ctx context.Context) (Health, error)
	Token(ctx context.Context) (Token, error)
	Mounts(ctx context.Context) ([]Mount, error)
	Capabilities(ctx context.Context, path string) ([]string, error)
	ReadSecret(ctx context.Context, path string) (Secret, error)
}

// Options configures how a client authenticates.
type Options struct {
	// GitHubToken is invoked only if no Vault token can be found, to log in
	// through Vault's GitHub auth method. It is a function so the common path
	// -- a Vault token already present -- never pays for resolving a GitHub
	// credential it will not use, which can mean shelling out to the keyring.
	GitHubToken func() (string, error)
	// AuthPath is the mount path of that auth method, without the auth/ prefix.
	AuthPath string
}

// ErrNoToken is returned when no Vault token is available and no GitHub token
// was supplied to log in with.
var ErrNoToken = errors.New("no Vault token available")

type client struct {
	api      *api.Client
	authPath string
}

// New builds an authenticated client. Configuration comes from the standard
// VAULT_* environment variables, which the SDK reads for us — including
// VAULT_ADDR, VAULT_NAMESPACE and the TLS settings.
//
// The token is resolved in the same order the vault CLI uses, with one
// addition at the end:
//
//  1. VAULT_TOKEN
//  2. ~/.vault-token
//  3. a fresh login through Vault's GitHub auth method
//
// A token minted by step 3 lives only for this process. It is deliberately not
// written to ~/.vault-token: this extension should not quietly install
// credentials on a machine.
func New(ctx context.Context, opts Options) (Client, error) {
	cfg := api.DefaultConfig()
	if cfg.Error != nil {
		return nil, fmt.Errorf("reading Vault configuration: %w", cfg.Error)
	}

	c, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating Vault client: %w", err)
	}

	authPath := opts.AuthPath
	if authPath == "" {
		authPath = "github"
	}

	wrapped := &client{api: c}

	// api.NewClient already picked up VAULT_TOKEN.
	if c.Token() == "" {
		if token := tokenFromDisk(); token != "" {
			c.SetToken(token)
		}
	}

	if c.Token() == "" {
		if opts.GitHubToken == nil {
			return nil, ErrNoToken
		}
		githubToken, err := opts.GitHubToken()
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrNoToken, err)
		}
		if err := wrapped.loginWithGitHub(ctx, authPath, githubToken); err != nil {
			return nil, err
		}
		wrapped.authPath = "auth/" + authPath
	}

	return wrapped, nil
}

// tokenFromDisk reads ~/.vault-token, where the vault CLI stores the token
// after `vault login`. The SDK does not do this for us.
func tokenFromDisk() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(home, ".vault-token"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (c *client) loginWithGitHub(ctx context.Context, authPath, githubToken string) error {
	path := "auth/" + authPath + "/login"

	secret, err := c.api.Logical().WriteWithContext(ctx, path, map[string]interface{}{
		"token": githubToken,
	})
	if err != nil {
		return fmt.Errorf("logging in at %s: %w", path, err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return fmt.Errorf("logging in at %s: no token in response", path)
	}

	c.api.SetToken(secret.Auth.ClientToken)
	return nil
}

func (c *client) Address() string { return c.api.Address() }

// Health reads sys/health, which lives only in the root namespace. Sending
// VAULT_NAMESPACE with it -- as HCP Vault users always have set -- asks for
// <namespace>/sys/health and gets a 404 "unsupported path", so the namespace
// is stripped for this one call. Every other endpoint here is namespaced and
// must keep it.
func (c *client) Health(ctx context.Context) (Health, error) {
	resp, err := c.api.WithNamespace("").Sys().HealthWithContext(ctx)
	if err != nil {
		return Health{}, fmt.Errorf("reading %s health: %w", c.Address(), err)
	}
	return Health{
		Version:     resp.Version,
		ClusterName: resp.ClusterName,
		Initialized: resp.Initialized,
		Sealed:      resp.Sealed,
		Standby:     resp.Standby,
	}, nil
}

func (c *client) Token(ctx context.Context) (Token, error) {
	secret, err := c.api.Auth().Token().LookupSelfWithContext(ctx)
	if err != nil {
		return Token{}, fmt.Errorf("looking up the current token: %w", err)
	}
	if secret == nil {
		return Token{}, errors.New("looking up the current token: empty response")
	}

	// The SDK's accessors handle json.Number and the missing-field cases; an
	// error from them only means the field was absent, which is not fatal
	// here -- a root token has no TTL, for instance.
	ttl, _ := secret.TokenTTL()
	policies, _ := secret.TokenPolicies()
	renewable, _ := secret.TokenIsRenewable()
	accessor, _ := secret.TokenAccessor()

	if policies == nil {
		policies = []string{}
	}

	return Token{
		DisplayName: stringField(secret.Data, "display_name"),
		Policies:    policies,
		TTL:         int(ttl.Seconds()),
		Renewable:   renewable,
		EntityID:    stringField(secret.Data, "entity_id"),
		Accessor:    accessor,
		AuthPath:    c.authPath,
	}, nil
}

// Mounts lists the secret engines this token can see, together with what it
// may do at each mount path.
//
// sys/internal/ui/mounts is used rather than sys/mounts because the latter
// needs a privileged policy; the UI endpoint is what Vault's own web console
// calls and is readable by any authenticated token.
func (c *client) Mounts(ctx context.Context) ([]Mount, error) {
	secret, err := c.api.Logical().ReadWithContext(ctx, "sys/internal/ui/mounts")
	if err != nil {
		return nil, fmt.Errorf("listing mounts: %w", err)
	}
	if secret == nil || secret.Data == nil {
		return []Mount{}, nil
	}

	raw, _ := secret.Data["secret"].(map[string]interface{})
	mounts := make([]Mount, 0, len(raw))
	paths := make([]string, 0, len(raw))

	for path, v := range raw {
		info, _ := v.(map[string]interface{})
		mounts = append(mounts, Mount{
			Path:        path,
			Type:        stringField(info, "type"),
			Description: stringField(info, "description"),
		})
		paths = append(paths, path)
	}

	caps := c.capabilitiesFor(ctx, paths)
	for i := range mounts {
		mounts[i].Capabilities = caps[mounts[i].Path]
		if mounts[i].Capabilities == nil {
			mounts[i].Capabilities = []string{}
		}
	}

	sortMounts(mounts)
	return mounts, nil
}

// capabilitiesFor resolves capabilities for many paths in one round trip.
// Failure is not fatal: the mount list is still worth showing without it.
func (c *client) capabilitiesFor(ctx context.Context, paths []string) map[string][]string {
	out := map[string][]string{}
	if len(paths) == 0 {
		return out
	}

	secret, err := c.api.Logical().WriteWithContext(ctx, "sys/capabilities-self", map[string]interface{}{
		"paths": paths,
	})
	if err != nil || secret == nil {
		return out
	}

	for _, p := range paths {
		if v, ok := secret.Data[p]; ok {
			out[p] = stringSlice(v)
		}
	}
	return out
}

func (c *client) Capabilities(ctx context.Context, path string) ([]string, error) {
	caps, err := c.api.Sys().CapabilitiesSelfWithContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("reading capabilities for %q: %w", path, err)
	}
	return caps, nil
}

// FormatTTL renders a duration in seconds the way Vault's own CLI does.
func FormatTTL(seconds int) string {
	if seconds <= 0 {
		return "none (root or non-expiring)"
	}
	d := time.Duration(seconds) * time.Second
	switch {
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

// ---------------------------------------------------------------------------
// Decoding helpers
//
// Vault's Logical() responses are map[string]interface{}, so every field needs
// a checked assertion. These keep that noise out of the methods above.
// ---------------------------------------------------------------------------

func stringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func boolField(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	b, _ := m[key].(bool)
	return b
}

func stringSlice(v interface{}) []string {
	switch typed := v.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
	}
}

// sortMounts orders mounts by path so the output is stable between runs —
// Vault returns them from a map, in whatever order Go feels like.
func sortMounts(mounts []Mount) {
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
}

// ---------------------------------------------------------------------------
// Reading secrets
// ---------------------------------------------------------------------------

// Secret is one KV entry. Values are strings because that is what a terminal
// and a JSON consumer can both use; non-string values are re-encoded as JSON
// rather than rendered with Go's %v, which would emit map[a:b] and be useless.
type Secret struct {
	Path      string            `json:"path"`
	Mount     string            `json:"mount"`
	MountType string            `json:"mountType"`
	Version   int               `json:"version,omitempty"`
	Data      map[string]string `json:"data"`
}

// Keys returns the field names in a stable order.
func (s Secret) Keys() []string {
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ReadSecret reads a KV secret, accepting the logical path the vault CLI takes
// (secret/prod/db) rather than the API path (secret/data/prod/db). Which of
// the two Vault wants depends on whether the mount is KV v1 or v2, so the
// mount is looked up first -- the same preflight the vault CLI performs.
func (c *client) ReadSecret(ctx context.Context, path string) (Secret, error) {
	path = strings.TrimPrefix(path, "/")

	mount, mountType, v2, err := c.kvMount(ctx, path)
	if err != nil {
		return Secret{}, err
	}

	apiPath := path
	if v2 {
		apiPath = kvDataPath(mount, path)
	}

	secret, err := c.api.Logical().ReadWithContext(ctx, apiPath)
	if err != nil {
		return Secret{}, fmt.Errorf("reading %s: %w", apiPath, err)
	}
	if secret == nil {
		return Secret{}, fmt.Errorf("no secret at %s", path)
	}

	out := Secret{Path: path, Mount: mount, MountType: mountType}

	raw := secret.Data
	if v2 {
		// A v2 response nests the payload under data, with metadata alongside.
		inner, _ := secret.Data["data"].(map[string]interface{})
		if inner == nil {
			return Secret{}, fmt.Errorf("no secret at %s (it may be deleted; check the metadata)", path)
		}
		raw = inner

		if meta, ok := secret.Data["metadata"].(map[string]interface{}); ok {
			if n, ok := meta["version"].(json.Number); ok {
				if v, err := n.Int64(); err == nil {
					out.Version = int(v)
				}
			}
		}
	}

	out.Data = make(map[string]string, len(raw))
	for k, v := range raw {
		out.Data[k] = renderValue(v)
	}
	return out, nil
}

// kvMount asks Vault which engine backs a path and whether it is KV v2.
func (c *client) kvMount(ctx context.Context, path string) (mount, mountType string, v2 bool, err error) {
	secret, err := c.api.Logical().ReadWithContext(ctx, "sys/internal/ui/mounts/"+path)
	if err != nil {
		return "", "", false, fmt.Errorf("resolving the mount for %q: %w", path, err)
	}
	if secret == nil || secret.Data == nil {
		return "", "", false, fmt.Errorf("no mount serves %q", path)
	}

	mount = stringField(secret.Data, "path")
	mountType = stringField(secret.Data, "type")

	if options, ok := secret.Data["options"].(map[string]interface{}); ok {
		if stringField(options, "version") == "2" {
			v2 = true
			mountType = "kv-v2"
		}
	}
	return mount, mountType, v2, nil
}

// kvDataPath rewrites secret/prod/db into secret/data/prod/db.
//
// A path that already carries the data/ segment is left alone, so the API path
// works too -- users reach for it after using `vault can`, which takes API
// paths. The cost is that a v2 secret literally named "data" is unreachable
// this way; the vault CLI has the same ambiguity.
func kvDataPath(mount, path string) string {
	rest := strings.TrimPrefix(path, mount)
	if rest == "data" || strings.HasPrefix(rest, "data/") {
		return path
	}
	return mount + "data/" + rest
}

// renderValue turns a decoded JSON value into something printable, keeping
// structure as JSON instead of Go's map[a:b] formatting.
func renderValue(v interface{}) string {
	switch typed := v.(type) {
	case string:
		return typed
	case nil:
		return ""
	case json.Number:
		return typed.String()
	default:
		if encoded, err := json.Marshal(typed); err == nil {
			return string(encoded)
		}
		return fmt.Sprintf("%v", typed)
	}
}
