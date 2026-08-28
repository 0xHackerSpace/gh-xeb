package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/vault"
)

func healthyVault() fakeVault {
	return fakeVault{
		health: vault.Health{Version: "1.17.2", Initialized: true},
		token: vault.Token{
			DisplayName: "github-IanOliv",
			Policies:    []string{"default", "platform-ro"},
			TTL:         2761200, // 767h
			Renewable:   true,
			EntityID:    "8f2a1c04-0000-0000-0000-000000000000",
		},
		mounts: []vault.Mount{
			{Path: "cubbyhole/", Type: "cubbyhole", Capabilities: []string{"create", "read", "update", "delete", "list"}},
			{Path: "database/", Type: "database", Capabilities: []string{"read"}},
			{Path: "kv/", Type: "kv-v2", Capabilities: []string{"read", "list"}},
			{Path: "platform/", Type: "kv-v2", Capabilities: []string{"read"}},
		},
		caps: map[string][]string{
			"kv/data/prod/db": {"read"},
			"secret/root":     {"root"},
			"blocked/path":    {"deny", "read"},
			"ops/thing":       {"read", "sudo"},
		},
	}
}

func TestVaultOverview(t *testing.T) {
	out, err := runRoot(t, vaultDeps(healthyVault()), "vault")
	if err != nil {
		t.Fatalf("vault returned error: %v", err)
	}

	want := strings.Join([]string{
		"Vault  https://vault.example.com:8200",
		"       v1.17.2, unsealed",
		"",
		"Token",
		"  display name   github-IanOliv",
		"  policies       default, platform-ro",
		"  ttl            767h (renewable)",
		"  entity         8f2a1c04-0000-0000-0000-000000000000",
		"",
		"Mounts (4 visible)",
		"  cubbyhole/  cubbyhole  create, read, update, delete, list",
		"  database/   database   read",
		"  kv/         kv-v2      read, list",
		"  platform/   kv-v2      read",
		"",
	}, "\n")

	if out != want {
		t.Errorf("overview mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestVaultOverviewReportsSealedServer(t *testing.T) {
	v := healthyVault()
	v.health = vault.Health{Version: "1.17.2", Initialized: true, Sealed: true}

	out, err := runRoot(t, vaultDeps(v), "vault")
	if err != nil {
		t.Fatalf("vault returned error: %v", err)
	}
	if !strings.Contains(out, "SEALED") {
		t.Errorf("a sealed server must be obvious, got:\n%s", out)
	}
}

func TestVaultTokenSubcommand(t *testing.T) {
	out, err := runRoot(t, vaultDeps(healthyVault()), "vault", "token")
	if err != nil {
		t.Fatalf("vault token returned error: %v", err)
	}
	for _, want := range []string{"github-IanOliv", "default, platform-ro", "767h (renewable)"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestVaultTokenNeverPrintsASecret(t *testing.T) {
	v := healthyVault()
	v.token.Accessor = "hmac-sha256:deadbeef"

	out, err := runRoot(t, vaultDeps(v), "vault", "token")
	if err != nil {
		t.Fatalf("vault token returned error: %v", err)
	}
	// The GitHub token in testDeps must never reach the output either.
	if strings.Contains(out, "gho_fake") {
		t.Errorf("the GitHub token leaked into the output:\n%s", out)
	}
}

func TestVaultMountsSubcommand(t *testing.T) {
	out, err := runRoot(t, vaultDeps(healthyVault()), "vault", "mounts")
	if err != nil {
		t.Fatalf("vault mounts returned error: %v", err)
	}
	if !strings.HasPrefix(out, "cubbyhole/") {
		t.Errorf("mounts should be listed without indentation, got:\n%s", out)
	}
	if strings.Contains(out, "Mounts (") {
		t.Errorf("the subcommand should not print the overview heading, got:\n%s", out)
	}
}

func TestVaultMountsWithNoneVisible(t *testing.T) {
	v := healthyVault()
	v.mounts = nil

	out, err := runRoot(t, vaultDeps(v), "vault", "mounts")
	if err != nil {
		t.Fatalf("vault mounts returned error: %v", err)
	}
	if !strings.Contains(out, "none visible") {
		t.Errorf("an empty list must say so rather than printing nothing, got %q", out)
	}
}

func TestVaultCan(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    map[string]string
		alsoHas string
	}{
		{
			name: "read only",
			path: "kv/data/prod/db",
			want: map[string]string{"read": "yes", "update": "no", "list": "no"},
		},
		{
			name:    "root permits everything",
			path:    "secret/root",
			want:    map[string]string{"read": "yes", "update": "yes", "delete": "yes"},
			alsoHas: "root",
		},
		{
			name: "deny overrides an explicit grant",
			path: "blocked/path",
			want: map[string]string{"read": "no"},
		},
		{
			name:    "sudo is surfaced separately",
			path:    "ops/thing",
			want:    map[string]string{"read": "yes"},
			alsoHas: "sudo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runRoot(t, vaultDeps(healthyVault()), "vault", "can", tt.path)
			if err != nil {
				t.Fatalf("vault can returned error: %v", err)
			}
			for action, answer := range tt.want {
				line := action + " "
				if !strings.Contains(out, line) {
					t.Fatalf("no line for %q in:\n%s", action, out)
				}
				for _, l := range strings.Split(out, "\n") {
					if strings.HasPrefix(strings.TrimSpace(l), action+" ") {
						if !strings.HasSuffix(strings.TrimSpace(l), answer) {
							t.Errorf("%s: got %q, want it to end in %q", action, strings.TrimSpace(l), answer)
						}
					}
				}
			}
			if tt.alsoHas != "" && !strings.Contains(out, "also: ") {
				t.Errorf("expected an 'also:' line mentioning %q, got:\n%s", tt.alsoHas, out)
			}
		})
	}
}

func TestVaultCanRequiresAPath(t *testing.T) {
	if _, err := runRoot(t, vaultDeps(healthyVault()), "vault", "can"); err == nil {
		t.Fatal("expected an error when no path is given")
	}
}

func TestVaultJSONOutput(t *testing.T) {
	out, err := runRoot(t, vaultDeps(healthyVault()), "vault", "--json")
	if err != nil {
		t.Fatalf("vault --json returned error: %v", err)
	}

	var payload struct {
		Address string       `json:"address"`
		Health  vault.Health `json:"health"`
		Token   vault.Token  `json:"token"`
		Mounts  []vault.Mount
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if payload.Address != "https://vault.example.com:8200" {
		t.Errorf("unexpected address %q", payload.Address)
	}
	if len(payload.Mounts) != 4 {
		t.Errorf("expected 4 mounts, got %d", len(payload.Mounts))
	}
	if payload.Token.TTL != 2761200 {
		t.Errorf("ttl should be seconds for machine consumers, got %d", payload.Token.TTL)
	}
}

func TestVaultCanJSONExposesResolvedBooleans(t *testing.T) {
	out, err := runRoot(t, vaultDeps(healthyVault()), "vault", "can", "blocked/path", "--json")
	if err != nil {
		t.Fatalf("vault can --json returned error: %v", err)
	}

	var payload struct {
		Path         string          `json:"path"`
		Capabilities []string        `json:"capabilities"`
		Allowed      map[string]bool `json:"allowed"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Allowed["read"] {
		t.Error("deny must win over an explicit read grant")
	}
	// The raw capability list is preserved so a consumer can see why.
	if len(payload.Capabilities) != 2 {
		t.Errorf("raw capabilities should be passed through, got %v", payload.Capabilities)
	}
}

func TestVaultExplainsHowToAuthenticate(t *testing.T) {
	deps := testDeps()
	deps.NewVaultClient = func(context.Context, vault.Options) (vault.Client, error) {
		return nil, vault.ErrNoToken
	}

	_, err := runRoot(t, deps, "vault")
	if err == nil {
		t.Fatal("expected an error when no token is available")
	}
	if !errors.Is(err, vault.ErrNoToken) {
		t.Errorf("the sentinel should survive wrapping, got %v", err)
	}
	for _, want := range []string{"VAULT_TOKEN", "vault login", "gh auth login"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should mention %q, got: %v", want, err)
		}
	}
}

func TestVaultPassesAuthPathThrough(t *testing.T) {
	var got vault.Options

	deps := testDeps()
	deps.NewVaultClient = func(_ context.Context, o vault.Options) (vault.Client, error) {
		got = o
		return healthyVault(), nil
	}

	if _, err := runRoot(t, deps, "vault", "--auth-path", "github-corp"); err != nil {
		t.Fatalf("vault returned error: %v", err)
	}
	if got.AuthPath != "github-corp" {
		t.Errorf("auth path not passed through, got %q", got.AuthPath)
	}
	if got.GitHubToken == nil {
		t.Error("the GitHub token resolver should be supplied for the login fallback")
	}
}

func TestVaultSurfacesServerErrors(t *testing.T) {
	v := healthyVault()
	v.mountsErr = errors.New("permission denied")

	if _, err := runRoot(t, vaultDeps(v), "vault", "mounts"); err == nil {
		t.Fatal("expected the server error to reach the caller")
	}
}
