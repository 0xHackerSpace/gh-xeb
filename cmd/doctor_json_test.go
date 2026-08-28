package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-cli-extension/internal/gh"
)

// TestDoctorJSONPayload pins the JSON contract. Other tooling consumes this
// document, so a field rename or removal must fail here first.
func TestDoctorJSONPayload(t *testing.T) {
	out, err := runRoot(t, healthyDeps(), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json returned error: %v", err)
	}

	want := `{
  "ghVersion": "2.4.0",
  "identity": {
    "host": "github.com",
    "authenticated": true,
    "tokenSource": "keyring",
    "login": "IanOliv",
    "id": 12345678,
    "scopes": [
      "repo",
      "read:org",
      "gist"
    ],
    "orgs": [
      "0xHackerSpace"
    ],
    "teams": [
      {
        "org": "0xHackerSpace",
        "slug": "platform"
      }
    ]
  },
  "vault": {
    "addr": "",
    "readOrgScope": true
  },
  "checks": [
    {
      "section": "GitHub",
      "status": "ok",
      "name": "gh CLI",
      "detail": "2.4.0"
    },
    {
      "section": "GitHub",
      "status": "ok",
      "name": "authentication",
      "detail": "github.com (keyring)"
    },
    {
      "section": "GitHub",
      "status": "ok",
      "name": "identity",
      "detail": "@IanOliv (id 12345678)"
    },
    {
      "section": "GitHub",
      "status": "ok",
      "name": "token scopes",
      "detail": "repo, read:org, gist"
    },
    {
      "section": "Vault readiness",
      "status": "ok",
      "name": "read:org scope",
      "detail": "present",
      "note": "required by Vault's GitHub auth method"
    },
    {
      "section": "Vault readiness",
      "status": "ok",
      "name": "org membership",
      "detail": "0xHackerSpace"
    },
    {
      "section": "Vault readiness",
      "status": "ok",
      "name": "team membership",
      "detail": "0xHackerSpace/platform"
    },
    {
      "section": "Vault readiness",
      "status": "warn",
      "name": "VAULT_ADDR",
      "detail": "not set",
      "note": "export VAULT_ADDR=https://vault.example.com:8200"
    }
  ],
  "summary": {
    "passed": 7,
    "warnings": 1,
    "failed": 0
  }
}
`
	if out != want {
		t.Errorf("JSON payload mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

// TestDoctorJSONUsesEmptyArrays guards against nil slices serialising as null,
// which breaks `jq '.identity.teams[]'` for consumers.
func TestDoctorJSONUsesEmptyArrays(t *testing.T) {
	rest := fakeREST{
		bodies: map[string]string{
			"user":       `{"login":"nobody","id":1}`,
			"user/orgs":  `[]`,
			"user/teams": `[]`,
		},
		header: http.Header{},
	}
	deps := testDeps()
	deps.NewRESTClient = fakeClient(rest)

	out, err := runRoot(t, deps, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json returned error: %v", err)
	}
	for _, field := range []string{`"scopes": []`, `"orgs": []`, `"teams": []`} {
		if !strings.Contains(out, field) {
			t.Errorf("expected %s in payload, got:\n%s", field, out)
		}
	}
	if strings.Contains(out, "null") {
		t.Errorf("payload must not contain null, got:\n%s", out)
	}
}

func TestDoctorJSONStillExitsNonZeroOnFailure(t *testing.T) {
	deps := healthyDeps()
	deps.CurrentAuth = func() gh.Auth {
		return gh.Auth{Host: "github.com", Source: "none", HasToken: false}
	}

	out, err := runRoot(t, deps, "doctor", "--json")
	if err == nil {
		t.Fatal("--json must not swallow the non-zero exit")
	}

	var payload jsonPayload
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output must stay valid JSON even when checks fail: %v", err)
	}
	if payload.Summary.Failed == 0 {
		t.Error("summary should report the failure")
	}
	if payload.Identity.Authenticated {
		t.Error("identity.authenticated should be false")
	}
}

func TestDoctorJSONReportsMissingReadOrgScope(t *testing.T) {
	rest := healthyREST()
	rest.header = http.Header{"X-Oauth-Scopes": []string{"repo, gist"}}

	deps := testDeps()
	deps.NewRESTClient = fakeClient(rest)

	out, err := runRoot(t, deps, "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json returned error: %v", err)
	}

	var payload jsonPayload
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if payload.Vault.ReadOrgScope {
		t.Error("vault.readOrgScope should be false when the scope is absent")
	}
}

func TestDoctorJSONAndTextDescribeTheSameRun(t *testing.T) {
	deps := healthyDeps()

	text, err := runRoot(t, deps, "doctor")
	if err != nil {
		t.Fatalf("text run failed: %v", err)
	}
	raw, err := runRoot(t, deps, "doctor", "--json")
	if err != nil {
		t.Fatalf("json run failed: %v", err)
	}

	var payload jsonPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Every check in the JSON must also be in the rendered report.
	for _, c := range payload.Checks {
		if !strings.Contains(text, c.Name) {
			t.Errorf("check %q is in the JSON but not in the text report", c.Name)
		}
	}
	if got, want := len(payload.Checks), 8; got != want {
		t.Errorf("expected %d checks, got %d", want, got)
	}
}

func TestDoctorJSONOmitsHumanReport(t *testing.T) {
	out, err := runRoot(t, healthyDeps(), "doctor", "--json")
	if err != nil {
		t.Fatalf("doctor --json returned error: %v", err)
	}
	if strings.Contains(out, "7 passed") {
		t.Errorf("--json must not also print the human summary, got:\n%s", out)
	}
}

func TestDoctorRejectsPositionalArgs(t *testing.T) {
	// `--json login` would be a natural mistake for anyone used to gh core,
	// where --json takes a field list. It must fail loudly, not silently.
	if _, err := runRoot(t, healthyDeps(), "doctor", "--json", "login"); err == nil {
		t.Fatal("expected an error for an unexpected positional argument")
	}
}
