package cmd

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
)

// healthyREST is the fixture for a fully configured account: a classic token
// carrying read:org, one org, one team.
func healthyREST() fakeREST {
	return fakeREST{
		bodies: map[string]string{
			"user":       `{"login":"IanOliv","name":"Ian","id":12345678}`,
			"user/orgs":  `[{"login":"0xHackerSpace"}]`,
			"user/teams": `[{"slug":"platform","organization":{"login":"0xHackerSpace"}}]`,
		},
		header: http.Header{"X-Oauth-Scopes": []string{"repo, read:org, gist"}},
	}
}

func healthyDeps() Deps {
	deps := testDeps()
	deps.NewRESTClient = fakeClient(healthyREST())
	return deps
}

// TestDoctorReport pins the exact layout of the report. It is a golden test on
// purpose: the alignment and the section order are the feature.
func TestDoctorReport(t *testing.T) {
	out, err := runRoot(t, healthyDeps(), "doctor")
	if err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}

	want := strings.Join([]string{
		"GitHub",
		"  ok   gh CLI            2.4.0",
		"  ok   authentication    github.com (keyring)",
		"  ok   identity          @IanOliv (id 12345678)",
		"  ok   token scopes      repo, read:org, gist",
		"",
		"Vault readiness",
		"  ok   read:org scope    present",
		"       (required by Vault's GitHub auth method)",
		"  ok   org membership    0xHackerSpace",
		"  ok   team membership   0xHackerSpace/platform",
		"  warn VAULT_ADDR        not set",
		"       (export VAULT_ADDR=https://vault.example.com:8200)",
		"",
		"7 passed, 1 warning",
		"",
	}, "\n")

	if out != want {
		t.Errorf("report mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestDoctorVaultAddrSet(t *testing.T) {
	deps := healthyDeps()
	deps.Getenv = func(key string) string {
		if key == "VAULT_ADDR" {
			return "https://vault.example.com:8200"
		}
		return ""
	}

	out, err := runRoot(t, deps, "doctor")
	if err != nil {
		t.Fatalf("doctor returned error: %v", err)
	}
	if !strings.Contains(out, "ok   VAULT_ADDR        https://vault.example.com:8200") {
		t.Errorf("VAULT_ADDR should be reported as ok, got:\n%s", out)
	}
	if !strings.Contains(out, "8 passed\n") {
		t.Errorf("expected all checks to pass, got:\n%s", out)
	}
}

func TestDoctorWarnsOnMissingReadOrgScope(t *testing.T) {
	rest := healthyREST()
	rest.header = http.Header{"X-Oauth-Scopes": []string{"repo, gist"}}

	deps := testDeps()
	deps.NewRESTClient = fakeClient(rest)

	out, err := runRoot(t, deps, "doctor")
	if err != nil {
		t.Fatalf("a warning must not fail the command, got: %v", err)
	}
	if !strings.Contains(out, "warn read:org scope    missing") {
		t.Errorf("expected a warning for the missing scope, got:\n%s", out)
	}
	if !strings.Contains(out, "gh auth refresh -s read:org") {
		t.Errorf("expected an actionable hint, got:\n%s", out)
	}
}

func TestDoctorWarnsOnFineGrainedToken(t *testing.T) {
	rest := healthyREST()
	rest.header = http.Header{} // fine-grained tokens report no classic scopes

	deps := testDeps()
	deps.NewRESTClient = fakeClient(rest)

	out, _ := runRoot(t, deps, "doctor")
	if !strings.Contains(out, "warn token scopes      none reported") {
		t.Errorf("expected a warning for a token with no scopes, got:\n%s", out)
	}
}

func TestDoctorFailsWithoutAuthentication(t *testing.T) {
	deps := healthyDeps()
	deps.CurrentAuth = func() gh.Auth {
		return gh.Auth{Host: "github.com", Source: "none", HasToken: false}
	}

	out, err := runRoot(t, deps, "doctor")
	if err == nil {
		t.Fatal("doctor must exit non-zero when a check fails")
	}

	var silent SilentError
	if !errors.As(err, &silent) {
		t.Errorf("the report already explains the failure, so the error should be silent; got %T", err)
	}
	if !strings.Contains(out, "fail authentication    no token for github.com") {
		t.Errorf("expected an authentication failure, got:\n%s", out)
	}
	if !strings.Contains(out, "1 failed") {
		t.Errorf("expected the summary to count the failure, got:\n%s", out)
	}
}

func TestDoctorReportsAPIFailuresPerCheck(t *testing.T) {
	deps := testDeps()
	deps.NewRESTClient = fakeClient(fakeREST{
		errs: map[string]error{
			"user":       errors.New("401 Unauthorized"),
			"user/orgs":  errors.New("401 Unauthorized"),
			"user/teams": errors.New("401 Unauthorized"),
		},
	})

	out, err := runRoot(t, deps, "doctor")
	if err == nil {
		t.Fatal("expected a non-zero exit when API calls fail")
	}
	// One failing call must not abort the whole report.
	for _, want := range []string{"fail identity", "fail org membership", "fail team membership", "VAULT_ADDR"} {
		if !strings.Contains(out, want) {
			t.Errorf("report should still contain %q, got:\n%s", want, out)
		}
	}
}

func TestDoctorWarnsWhenGhBinaryMissing(t *testing.T) {
	deps := healthyDeps()
	deps.CLIVersion = func() (string, error) { return "", errors.New("exec: \"gh\": not found") }

	out, err := runRoot(t, deps, "doctor")
	if err != nil {
		t.Fatalf("a missing gh binary is a warning, not a failure: %v", err)
	}
	if !strings.Contains(out, "warn gh CLI            not found on PATH") {
		t.Errorf("expected a warning for the missing binary, got:\n%s", out)
	}
}

func TestDoctorDoesNotRepeatTheSameErrorOnEveryLine(t *testing.T) {
	clientErr := errors.New("authentication token not found for host github.com")

	deps := testDeps()
	deps.NewRESTClient = func() (gh.RESTClient, error) { return nil, clientErr }

	out, err := runRoot(t, deps, "doctor")
	if err == nil {
		t.Fatal("expected a non-zero exit when the client cannot be built")
	}
	if n := strings.Count(out, clientErr.Error()); n != 1 {
		t.Errorf("the root cause should appear exactly once, appeared %d times:\n%s", n, out)
	}
	if !strings.Contains(out, "fail token scopes      not checked (no API access)") {
		t.Errorf("dependent checks should say why they were skipped, got:\n%s", out)
	}
}
