package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestMcpToolDiscovery(t *testing.T) {
	root := NewRootCmd(testDeps())
	tools := discoverTools(root, "")

	if len(tools) == 0 {
		t.Fatal("expected tools to be discovered, got none")
	}

	// Check that some known commands are present
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	// These are top-level commands that should always be present
	expected := []string{"doctor", "vault", "backstage", "harness", "whoami"}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("expected tool %q to be discovered, but it wasn't", name)
		}
	}
}

func TestMcpToolNames(t *testing.T) {
	tests := []struct {
		prefix string
		name   string
		want   string
	}{
		{"", "doctor", "doctor"},
		{"vault", "get", "vault_get"},
		{"backstage", "entities", "backstage_entities"},
	}

	for _, tt := range tests {
		got := toolName(tt.prefix, tt.name)
		if got != tt.want {
			t.Errorf("toolName(%q, %q) = %q, want %q", tt.prefix, tt.name, got, tt.want)
		}
	}
}

func TestMcpInputSchema(t *testing.T) {
	root := NewRootCmd(testDeps())

	// Find the doctor command which has flags like --json
	var cmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "doctor" {
			cmd = c
			break
		}
	}

	if cmd == nil {
		t.Fatal("could not find doctor command for testing")
	}

	schema := buildInputSchema(cmd)

	// Schema should have basic structure
	if schema["type"] != "object" {
		t.Errorf("expected type=object, got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties to be a map")
	}

	// doctor command should have at least the --json flag
	if _, hasJSON := props["json"]; !hasJSON {
		t.Error("expected json flag to be in schema")
	}
}

func TestMcpCommand(t *testing.T) {
	deps := testDeps()
	cmd := newMcpCmd(deps)

	if cmd.Short == "" {
		t.Error("expected Short description")
	}

	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}

	// MCP command takes no args
	if cmd.Args == nil {
		t.Error("expected Args validator to be set")
	}
}

func TestMcpCommandIntegration(t *testing.T) {
	// Test that the mcp command is registered in root
	root := NewRootCmd(testDeps())

	found := false
	for _, cmd := range root.Commands() {
		if cmd.Name() == "mcp" {
			found = true
			break
		}
	}

	if !found {
		t.Error("mcp command not registered in root")
	}
}
