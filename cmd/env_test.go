package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-xeb/internal/backstage"
	"github.com/0xHackerSpace/gh-xeb/internal/vault"
)

func TestApplyEnv(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		env      string
		explicit bool
		want     string
	}{
		{
			name: "placeholder is filled in even without the flag",
			path: "secret/abcd/{env}/backstage", env: "dev",
			want: "secret/abcd/dev/backstage",
		},
		{
			name: "placeholder honours an explicit value",
			path: "secret/abcd/{env}/backstage", env: "prod", explicit: true,
			want: "secret/abcd/prod/backstage",
		},
		{
			name: "placeholder works at any position",
			path: "{env}/abcd/api", env: "uat",
			want: "uat/abcd/api",
		},
		{
			name: "a literal segment is untouched without the flag",
			path: "secret/abcd/dev/api-1", env: "dev",
			want: "secret/abcd/dev/api-1",
		},
		{
			name: "the default cannot silently rewrite a prod path",
			path: "secret/abcd/prod/api-1", env: "dev",
			want: "secret/abcd/prod/api-1",
		},
		{
			name: "an explicit flag rewrites a literal segment",
			path: "secret/abcd/dev/api-1", env: "prod", explicit: true,
			want: "secret/abcd/prod/api-1",
		},
		{
			name: "an explicit downgrade is still a rewrite",
			path: "secret/abcd/prod/api-1", env: "dev", explicit: true,
			want: "secret/abcd/dev/api-1",
		},
		{
			name: "a path with no environment is left alone",
			path: "secret/meu-app", env: "prod", explicit: true,
			want: "secret/meu-app",
		},
		{
			name: "an app whose name merely contains an environment is safe",
			path: "secret/abcd/dev/developer-portal", env: "prod", explicit: true,
			want: "secret/abcd/prod/developer-portal",
		},
		{
			name: "the API path form works too",
			path: "secret/data/abcd/dev/api-1", env: "uat", explicit: true,
			want: "secret/data/abcd/uat/api-1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyEnv(tc.path, tc.env, tc.explicit); got != tc.want {
				t.Errorf("applyEnv(%q, %q, %v) = %q, want %q",
					tc.path, tc.env, tc.explicit, got, tc.want)
			}
		})
	}
}

func TestValidateEnv(t *testing.T) {
	for _, env := range vaultEnvironments {
		if err := validateEnv(env); err != nil {
			t.Errorf("validateEnv(%q): %v", env, err)
		}
	}

	err := validateEnv("staging")
	if err == nil {
		t.Fatal("want an error for an unknown environment")
	}
	for _, want := range []string{"staging", "dev", "uat", "prod"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err, want)
		}
	}
}

// envSecret is a fake Vault holding one secret per environment, so a test can
// tell which path a command actually read.
func envSecrets() map[string]vault.Secret {
	secrets := map[string]vault.Secret{}
	for _, env := range vaultEnvironments {
		path := "secret/abcd/" + env + "/backstage"
		secrets[path] = vault.Secret{
			Path: path, Mount: "secret/", MountType: "kv-v2",
			Data: map[string]string{"url": "https://" + env + ".example.com", "token": env + "-token"},
		}
	}
	return secrets
}

func TestVaultGetFillsThePlaceholder(t *testing.T) {
	deps := vaultDeps(fakeVault{secrets: envSecrets()})

	out, err := runRoot(t, deps, "vault", "get", "secret/abcd/{env}/backstage")
	if err != nil {
		t.Fatalf("vault get: %v", err)
	}
	if !strings.Contains(out, "secret/abcd/dev/backstage") {
		t.Errorf("the default environment was not used:\n%s", out)
	}
}

func TestVaultGetHonoursAnExplicitEnv(t *testing.T) {
	deps := vaultDeps(fakeVault{secrets: envSecrets()})

	out, err := runRoot(t, deps, "vault", "get", "secret/abcd/{env}/backstage", "--env", "prod")
	if err != nil {
		t.Fatalf("vault get --env prod: %v", err)
	}
	if !strings.Contains(out, "secret/abcd/prod/backstage") {
		t.Errorf("output:\n%s", out)
	}
}

func TestVaultGetRewritesALiteralSegmentOnlyWhenAsked(t *testing.T) {
	deps := vaultDeps(fakeVault{secrets: envSecrets()})

	// Without the flag the path is read exactly as typed.
	out, err := runRoot(t, deps, "vault", "get", "secret/abcd/prod/backstage")
	if err != nil {
		t.Fatalf("vault get: %v", err)
	}
	if !strings.Contains(out, "secret/abcd/prod/backstage") {
		t.Errorf("the default rewrote a path it should not have:\n%s", out)
	}

	// With it, the segment moves.
	out, err = runRoot(t, deps, "vault", "get", "secret/abcd/prod/backstage", "--env", "uat")
	if err != nil {
		t.Fatalf("vault get --env uat: %v", err)
	}
	if !strings.Contains(out, "secret/abcd/uat/backstage") {
		t.Errorf("output:\n%s", out)
	}
}

func TestVaultCanTakesEnv(t *testing.T) {
	deps := vaultDeps(fakeVault{caps: map[string][]string{
		"secret/data/abcd/uat/api-1": {"read"},
	}})

	out, err := runRoot(t, deps, "vault", "can", "secret/data/abcd/{env}/api-1", "--env", "uat")
	if err != nil {
		t.Fatalf("vault can: %v", err)
	}
	if !strings.Contains(out, "secret/data/abcd/uat/api-1") {
		t.Errorf("output:\n%s", out)
	}
	if !strings.Contains(out, "read     yes") {
		t.Errorf("the resolved path was not the one queried:\n%s", out)
	}
}

func TestVaultRejectsAnUnknownEnv(t *testing.T) {
	called := false
	deps := vaultDeps(fakeVault{secrets: envSecrets()})
	inner := deps.NewVaultClient
	deps.NewVaultClient = func(ctx context.Context, o vault.Options) (vault.Client, error) {
		called = true
		return inner(ctx, o)
	}

	_, err := runRoot(t, deps, "vault", "get", "secret/abcd/{env}/backstage", "--env", "staging")
	if err == nil || !strings.Contains(err.Error(), "staging") {
		t.Fatalf("error = %v", err)
	}
	// Validated before anything connects: a typo must not cost a login.
	if called {
		t.Error("the command contacted Vault with an invalid environment")
	}
}

func TestBackstageVaultSecretTakesEnv(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(envSecrets(), &got)

	if _, err := runRoot(t, deps, "backstage",
		"--vault-secret", "secret/abcd/{env}/backstage", "--env", "prod"); err != nil {
		t.Fatalf("backstage --env prod: %v", err)
	}
	if got.BaseURL != "https://prod.example.com" || got.Token != "prod-token" {
		t.Fatalf("config = %+v, want the prod secret", got)
	}
}

func TestBackstageVaultSecretDefaultsToDev(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(envSecrets(), &got)

	if _, err := runRoot(t, deps, "backstage",
		"--vault-secret", "secret/abcd/{env}/backstage"); err != nil {
		t.Fatalf("backstage: %v", err)
	}
	if got.BaseURL != "https://dev.example.com" {
		t.Fatalf("config = %+v, want the dev secret", got)
	}
}

func TestBackstageRejectsAnUnknownEnv(t *testing.T) {
	var got backstage.Config
	deps := vaultBackedDeps(envSecrets(), &got)

	_, err := runRoot(t, deps, "backstage", "--env", "homolog")
	if err == nil || !strings.Contains(err.Error(), "homolog") {
		t.Fatalf("error = %v", err)
	}
	if got.BaseURL != "" {
		t.Error("a client was built with an invalid environment")
	}
}
