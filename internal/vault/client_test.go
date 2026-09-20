package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatTTL(t *testing.T) {
	tests := []struct {
		seconds int
		want    string
	}{
		{2761200, "767h"},
		{3600, "1h"},
		{300, "5m"},
		{45, "45s"},
		{0, "none (root or non-expiring)"},
		{-1, "none (root or non-expiring)"},
	}

	for _, tt := range tests {
		if got := FormatTTL(tt.seconds); got != tt.want {
			t.Errorf("FormatTTL(%d) = %q, want %q", tt.seconds, got, tt.want)
		}
	}
}

func TestStringSlice(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
	}{
		{"already typed", []string{"a", "b"}, 2},
		{"decoded from JSON", []interface{}{"a", "b", "c"}, 3},
		{"mixed types drop non-strings", []interface{}{"a", 1, "b"}, 2},
		{"nil", nil, 0},
		{"wrong type", "not a slice", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSlice(tt.input)
			if got == nil {
				t.Fatal("stringSlice must never return nil; it is serialised to JSON")
			}
			if len(got) != tt.want {
				t.Errorf("got %v, want %d entries", got, tt.want)
			}
		})
	}
}

func TestStringAndBoolFields(t *testing.T) {
	m := map[string]interface{}{"name": "octo", "renewable": true, "count": 3}

	if got := stringField(m, "name"); got != "octo" {
		t.Errorf("stringField = %q", got)
	}
	if got := stringField(m, "missing"); got != "" {
		t.Errorf("a missing key should be empty, got %q", got)
	}
	if got := stringField(m, "count"); got != "" {
		t.Errorf("a non-string value should be empty, got %q", got)
	}
	if got := stringField(nil, "name"); got != "" {
		t.Errorf("a nil map should be empty, got %q", got)
	}
	if !boolField(m, "renewable") {
		t.Error("boolField should read a bool")
	}
	if boolField(m, "missing") {
		t.Error("a missing key should be false")
	}
}

func TestSortMountsIsStable(t *testing.T) {
	// Vault returns mounts from a map, so the order is random per call.
	mounts := []Mount{{Path: "kv/"}, {Path: "cubbyhole/"}, {Path: "database/"}}
	sortMounts(mounts)

	want := []string{"cubbyhole/", "database/", "kv/"}
	for i, w := range want {
		if mounts[i].Path != w {
			t.Errorf("position %d: got %q, want %q", i, mounts[i].Path, w)
		}
	}
}

func TestTokenFromDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := tokenFromDisk(); got != "" {
		t.Errorf("no file should mean no token, got %q", got)
	}

	// The vault CLI writes the token with a trailing newline.
	path := filepath.Join(home, ".vault-token")
	if err := os.WriteFile(path, []byte("hvs.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := tokenFromDisk(); got != "hvs.example" {
		t.Errorf("got %q, want the trimmed token", got)
	}
}

// writeVaultToken drops a ~/.vault-token into home, the way `vault login` does.
func writeVaultToken(t *testing.T, home, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".vault-token"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
