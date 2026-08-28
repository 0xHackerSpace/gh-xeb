package gh

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{"release build", "gh version 2.62.0 (2024-11-14)\nhttps://github.com/cli/cli/releases/latest\n", "2.62.0"},
		{"distro build", "gh version 2.4.0+dfsg1 (2022-03-23 Ubuntu 2.4.0+dfsg1-2)\n", "2.4.0+dfsg1"},
		{"unexpected shape falls back to the line", "something else entirely\n", "something else entirely"},
		{"empty output", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseVersion(tt.output); got != tt.want {
				t.Errorf("parseVersion(%q) = %q, want %q", tt.output, got, tt.want)
			}
		})
	}
}

func TestDescribeSource(t *testing.T) {
	tests := map[string]string{
		"oauth_token": "gh config",
		"gh":          "keyring",
		"default":     "none",
		"GH_TOKEN":    "GH_TOKEN",
	}

	for source, want := range tests {
		if got := describeSource(source); got != want {
			t.Errorf("describeSource(%q) = %q, want %q", source, got, want)
		}
	}
}

func TestRepoString(t *testing.T) {
	r := Repo{Host: "github.com", Owner: "0xHackerSpace", Name: "gh-cli-extension"}
	if got, want := r.String(), "0xHackerSpace/gh-cli-extension"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
