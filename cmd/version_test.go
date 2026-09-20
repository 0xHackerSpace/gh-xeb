package cmd

import "testing"

func TestVersionCommandPrintsResolvedVersion(t *testing.T) {
	original := version
	version = "v9.9.9"
	t.Cleanup(func() { version = original })

	out, err := runRoot(t, testDeps(), "version")
	if err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	if out != "v9.9.9\n" {
		t.Errorf("got %q, want %q", out, "v9.9.9\n")
	}
}

func TestResolveVersionFallsBackWhenUnset(t *testing.T) {
	original := version
	version = ""
	t.Cleanup(func() { version = original })

	if resolveVersion() == "" {
		t.Error("resolveVersion() must never return an empty string")
	}
}
