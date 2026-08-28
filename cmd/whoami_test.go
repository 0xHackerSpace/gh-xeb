package cmd

import (
	"errors"
	"testing"
)

func TestWhoami(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"with display name", `{"login":"octocat","name":"The Octocat"}`, "You are @octocat (The Octocat).\n"},
		{"without display name", `{"login":"octocat"}`, "You are @octocat (no display name).\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDeps()
			deps.NewRESTClient = fakeClient(fakeREST{bodies: map[string]string{"user": tt.body}})

			out, err := runRoot(t, deps, "whoami")
			if err != nil {
				t.Fatalf("whoami returned error: %v", err)
			}
			if out != tt.want {
				t.Errorf("got %q, want %q", out, tt.want)
			}
		})
	}
}

func TestWhoamiPropagatesAPIError(t *testing.T) {
	deps := testDeps()
	deps.NewRESTClient = fakeClient(fakeREST{
		errs: map[string]error{"user": errors.New("401 Unauthorized")},
	})

	if _, err := runRoot(t, deps, "whoami"); err == nil {
		t.Fatal("expected an error when the API call fails")
	}
}
