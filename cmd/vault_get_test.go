package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xHackerSpace/gh-xeb/internal/vault"
)

func vaultWithSecret() fakeVault {
	v := healthyVault()
	v.secrets = map[string]vault.Secret{
		"secret/prod/db": {
			Path:      "secret/prod/db",
			Mount:     "secret/",
			MountType: "kv-v2",
			Version:   3,
			Data:      map[string]string{"password": "s3cr3t-value", "username": "app", "note": ""},
		},
	}
	return v
}

func TestVaultGetMasksByDefault(t *testing.T) {
	out, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/prod/db")
	if err != nil {
		t.Fatalf("vault get returned error: %v", err)
	}

	want := strings.Join([]string{
		"secret/prod/db  kv-v2, version 3",
		"",
		"  note       (empty)",
		"  password   ******** (12 chars)",
		"  username   ******** (3 chars)",
		"",
		"  --reveal to print values, --field <key> for one",
		"",
	}, "\n")

	if out != want {
		t.Errorf("mismatch\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
	if strings.Contains(out, "s3cr3t-value") {
		t.Error("the value leaked into the default output")
	}
}

func TestVaultGetReveal(t *testing.T) {
	out, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/prod/db", "--reveal")
	if err != nil {
		t.Fatalf("vault get --reveal returned error: %v", err)
	}
	if !strings.Contains(out, "s3cr3t-value") {
		t.Errorf("--reveal must print the value, got:\n%s", out)
	}
	if strings.Contains(out, "********") {
		t.Errorf("--reveal must not mask, got:\n%s", out)
	}
	if strings.Contains(out, "--reveal to print") {
		t.Errorf("the hint is pointless once revealed, got:\n%s", out)
	}
}

func TestVaultGetFieldIsRawForPiping(t *testing.T) {
	out, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/prod/db", "--field", "password")
	if err != nil {
		t.Fatalf("vault get --field returned error: %v", err)
	}
	// Exactly the value and a newline: anything else breaks a pipe.
	if out != "s3cr3t-value\n" {
		t.Errorf("got %q, want %q", out, "s3cr3t-value\n")
	}
}

func TestVaultGetUnknownFieldListsWhatExists(t *testing.T) {
	_, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/prod/db", "--field", "nope")
	if err == nil {
		t.Fatal("expected an error for an unknown field")
	}
	for _, want := range []string{"note", "password", "username"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should list the available fields, got: %v", err)
		}
	}
}

func TestVaultGetRejectsFieldWithReveal(t *testing.T) {
	_, err := runRoot(t, vaultDeps(vaultWithSecret()),
		"vault", "get", "secret/prod/db", "--field", "password", "--reveal")
	if err == nil {
		t.Fatal("--field with --reveal is contradictory and should be rejected")
	}
}

func TestVaultGetJSONMasksUnlessRevealed(t *testing.T) {
	type payload struct {
		Path   string            `json:"path"`
		Masked bool              `json:"masked"`
		Data   map[string]string `json:"data"`
	}

	out, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/prod/db", "--json")
	if err != nil {
		t.Fatalf("vault get --json returned error: %v", err)
	}
	var masked payload
	if err := json.Unmarshal([]byte(out), &masked); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !masked.Masked {
		t.Error("the payload must declare that it is masked")
	}
	if strings.Contains(out, "s3cr3t-value") {
		t.Error("a value leaked into the masked JSON")
	}

	out, err = runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/prod/db", "--json", "--reveal")
	if err != nil {
		t.Fatalf("vault get --json --reveal returned error: %v", err)
	}
	var revealed payload
	if err := json.Unmarshal([]byte(out), &revealed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if revealed.Masked {
		t.Error("masked should be false once revealed")
	}
	if revealed.Data["password"] != "s3cr3t-value" {
		t.Errorf("value not present: %v", revealed.Data)
	}
}

func TestVaultGetRequiresAPath(t *testing.T) {
	if _, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get"); err == nil {
		t.Fatal("expected an error when no path is given")
	}
}

func TestVaultGetSurfacesMissingSecret(t *testing.T) {
	if _, err := runRoot(t, vaultDeps(vaultWithSecret()), "vault", "get", "secret/nope"); err == nil {
		t.Fatal("expected an error for a path with no secret")
	}
}
