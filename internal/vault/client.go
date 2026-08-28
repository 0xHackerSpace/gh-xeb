// Package vault wraps the official Vault SDK behind a narrow interface, so
// commands depend on this package's types rather than on api.Client and can be
// tested without a Vault server.
package vault

import (
	"context"
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

func (c *client) Health(ctx context.Context) (Health, error) {
	resp, err := c.api.Sys().HealthWithContext(ctx)
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
