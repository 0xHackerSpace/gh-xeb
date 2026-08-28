package vault

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// These tests drive the real SDK against a stand-in Vault over HTTP, so the
// response decoding is actually exercised. The fakes used by the command tests
// bypass all of it.

type recordedRequest struct {
	method string
	path   string
	token  string
	body   map[string]interface{}
}

func newStandInVault(t *testing.T, recorder *[]recordedRequest) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	record := func(r *http.Request) {
		if recorder == nil {
			return
		}
		var body map[string]interface{}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		*recorder = append(*recorder, recordedRequest{
			method: r.Method,
			path:   r.URL.Path,
			token:  r.Header.Get("X-Vault-Token"),
			body:   body,
		})
	}

	write := func(w http.ResponseWriter, payload interface{}) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}

	mux.HandleFunc("/v1/sys/health", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		write(w, map[string]interface{}{
			"initialized": true, "sealed": false, "standby": false,
			"version": "1.17.2", "cluster_name": "vault-cluster-abc",
		})
	})

	mux.HandleFunc("/v1/auth/token/lookup-self", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		write(w, map[string]interface{}{
			"data": map[string]interface{}{
				"display_name": "github-IanOliv",
				"policies":     []string{"default", "platform-ro"},
				"ttl":          2761200,
				"renewable":    true,
				"entity_id":    "8f2a1c04",
				"accessor":     "acc-123",
			},
		})
	})

	mux.HandleFunc("/v1/sys/internal/ui/mounts", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		write(w, map[string]interface{}{
			"data": map[string]interface{}{
				"secret": map[string]interface{}{
					"kv/":        map[string]interface{}{"type": "kv", "description": "key/value"},
					"cubbyhole/": map[string]interface{}{"type": "cubbyhole"},
				},
				"auth": map[string]interface{}{
					"github/": map[string]interface{}{"type": "github"},
				},
			},
		})
	})

	mux.HandleFunc("/v1/sys/capabilities-self", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		write(w, map[string]interface{}{
			"data": map[string]interface{}{
				"kv/":             []string{"read", "list"},
				"cubbyhole/":      []string{"create", "read"},
				"kv/data/prod/db": []string{"read"},
			},
		})
	})

	mux.HandleFunc("/v1/auth/github/login", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		write(w, map[string]interface{}{
			"auth": map[string]interface{}{
				"client_token": "hvs.minted-for-this-run",
				"policies":     []string{"default", "platform-ro"},
			},
		})
	})

	// Catch-all so an unexpected path is still recorded; without it the mux's
	// own 404 handler swallows the request and a test cannot see what was
	// actually asked for.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.WriteHeader(http.StatusNotFound)
		write(w, map[string]interface{}{"errors": []string{"no handler for " + r.URL.Path}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// isolate points the SDK at the stand-in server and makes sure no real
// credential on this machine can influence the test.
func isolate(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("HOME", t.TempDir()) // so ~/.vault-token cannot be found
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("VAULT_NAMESPACE", "")
}

func TestClientAgainstStandInVault(t *testing.T) {
	srv := newStandInVault(t, nil)
	isolate(t, srv)
	t.Setenv("VAULT_TOKEN", "existing-token")

	ctx := context.Background()
	c, err := New(ctx, Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Run("health", func(t *testing.T) {
		h, err := c.Health(ctx)
		if err != nil {
			t.Fatalf("Health: %v", err)
		}
		if h.Version != "1.17.2" || h.Sealed || !h.Initialized {
			t.Errorf("unexpected health: %+v", h)
		}
		if h.ClusterName != "vault-cluster-abc" {
			t.Errorf("cluster name not decoded: %q", h.ClusterName)
		}
	})

	t.Run("token", func(t *testing.T) {
		tok, err := c.Token(ctx)
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if tok.DisplayName != "github-IanOliv" {
			t.Errorf("display name: %q", tok.DisplayName)
		}
		if len(tok.Policies) != 2 {
			t.Errorf("policies: %v", tok.Policies)
		}
		// ttl arrives as a JSON number; the SDK accessor must turn it into
		// seconds rather than leaving a json.Number behind.
		if tok.TTL != 2761200 {
			t.Errorf("ttl: %d, want 2761200", tok.TTL)
		}
		if !tok.Renewable {
			t.Error("renewable should be true")
		}
	})

	t.Run("mounts carry capabilities and are sorted", func(t *testing.T) {
		mounts, err := c.Mounts(ctx)
		if err != nil {
			t.Fatalf("Mounts: %v", err)
		}
		if len(mounts) != 2 {
			t.Fatalf("expected only secret engines, got %d: %+v", len(mounts), mounts)
		}
		if mounts[0].Path != "cubbyhole/" || mounts[1].Path != "kv/" {
			t.Errorf("not sorted: %+v", mounts)
		}
		if got := mounts[1].Capabilities; len(got) != 2 || got[0] != "read" {
			t.Errorf("capabilities not attached to kv/: %v", got)
		}
		if mounts[1].Description != "key/value" {
			t.Errorf("description not decoded: %q", mounts[1].Description)
		}
	})

	t.Run("capabilities", func(t *testing.T) {
		caps, err := c.Capabilities(ctx, "kv/data/prod/db")
		if err != nil {
			t.Fatalf("Capabilities: %v", err)
		}
		if len(caps) != 1 || caps[0] != "read" {
			t.Errorf("got %v", caps)
		}
	})
}

func TestClientLogsInWithGitHubWhenNoTokenExists(t *testing.T) {
	var seen []recordedRequest
	srv := newStandInVault(t, &seen)
	isolate(t, srv)

	ctx := context.Background()
	c, err := New(ctx, Options{
		GitHubToken: func() (string, error) { return "gho_secret", nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if len(seen) != 1 || seen[0].path != "/v1/auth/github/login" {
		t.Fatalf("expected a login request, saw %+v", seen)
	}
	if seen[0].body["token"] != "gho_secret" {
		t.Errorf("the GitHub token should be the login payload, got %v", seen[0].body)
	}

	// The minted token must be used for everything afterwards.
	if _, err := c.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	last := seen[len(seen)-1]
	if last.token != "hvs.minted-for-this-run" {
		t.Errorf("subsequent calls should carry the minted token, got %q", last.token)
	}

	tok, err := c.Token(ctx)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AuthPath != "auth/github" {
		t.Errorf("the token should record where it came from, got %q", tok.AuthPath)
	}
}

func TestClientHonoursACustomAuthPath(t *testing.T) {
	var seen []recordedRequest
	srv := newStandInVault(t, &seen)
	isolate(t, srv)

	// Only auth/github/login is registered, so a different path 404s. That is
	// enough to prove the path is used rather than hard-coded.
	_, err := New(context.Background(), Options{
		AuthPath:    "github-corp",
		GitHubToken: func() (string, error) { return "gho_secret", nil },
	})
	if err == nil {
		t.Fatal("expected the unregistered auth path to fail")
	}
	if len(seen) != 1 || seen[0].path != "/v1/auth/github-corp/login" {
		t.Errorf("expected a request to the custom path, saw %+v", seen)
	}
}

func TestClientWithoutAnyTokenReturnsSentinel(t *testing.T) {
	srv := newStandInVault(t, nil)
	isolate(t, srv)

	_, err := New(context.Background(), Options{})
	if err != ErrNoToken {
		t.Fatalf("want ErrNoToken, got %v", err)
	}
}

func TestClientPrefersTokenOnDiskOverLoggingIn(t *testing.T) {
	var seen []recordedRequest
	srv := newStandInVault(t, &seen)
	isolate(t, srv)

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeVaultToken(t, home, "token-from-disk\n")

	ctx := context.Background()
	c, err := New(ctx, Options{
		GitHubToken: func() (string, error) {
			t.Error("must not log in when ~/.vault-token exists")
			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := c.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if seen[0].token != "token-from-disk" {
		t.Errorf("the on-disk token should be used verbatim, got %q", seen[0].token)
	}
}
