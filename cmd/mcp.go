package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// MCPTool represents a Cobra command as an MCP tool definition.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// discoverTools recursively walks the Cobra command tree and returns all
// executable commands (those with RunE set) as MCP tools.
func discoverTools(cmd *cobra.Command, prefix string) []MCPTool {
	var tools []MCPTool

	// Process subcommands of this command
	for _, subcmd := range cmd.Commands() {
		if subcmd.Hidden {
			continue
		}

		// Build the full tool name
		var toolPrefix string
		if prefix != "" {
			toolPrefix = prefix + "_"
		}
		name := toolPrefix + subcmd.Name()

		// Only expose commands with RunE (executable commands)
		if subcmd.RunE != nil {
			tool := MCPTool{
				Name:        name,
				Description: subcmd.Short,
				InputSchema: buildInputSchema(subcmd),
			}
			tools = append(tools, tool)
		}

		// Recursively process subcommands
		tools = append(tools, discoverTools(subcmd, name)...)
	}

	return tools
}

// toolName builds a stable, collision-free name for a tool.
// Example: "vault_get", "backstage_entities", "doctor"
func toolName(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "_" + name
}

// buildInputSchema constructs a JSON Schema for a command's flags and arguments.
func buildInputSchema(cmd *cobra.Command) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}

	properties := schema["properties"].(map[string]interface{})

	// Add flags
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		// Skip hidden flags and help
		if flag.Hidden || flag.Name == "help" {
			return
		}

		prop := map[string]interface{}{
			"description": flag.Usage,
		}

		// Infer type from the flag's type string
		switch flag.Value.Type() {
		case "stringSlice":
			prop["type"] = "array"
			prop["items"] = map[string]interface{}{"type": "string"}
		case "bool":
			prop["type"] = "boolean"
		case "int":
			prop["type"] = "integer"
		case "float64":
			prop["type"] = "number"
		default:
			prop["type"] = "string"
		}

		properties[flag.Name] = prop
	})

	// Note: Cobra/pflag doesn't have a standard way to mark flags as required,
	// so we don't populate the required array. Commands that need required flags
	// handle validation themselves.

	return schema
}

// newMcpCmd creates the MCP server command.
func newMcpCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP server exposing the CLI as tools",
		Long: `mcp starts a Model Context Protocol (MCP) server via stdio, exposing
all CLI commands as discoverable, type-safe tools that AI clients can invoke.

The server reads tool requests from stdin and writes responses to stdout,
reserving stdout exclusively for the MCP protocol. Diagnostics go to stderr.

This allows clients like Claude to use gh xeb's capabilities directly without
parsing shell output.`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runMCPServer(c.Context(), c.ErrOrStderr())
		},
	}
}

// runMCPServer implements the MCP stdio protocol handler.
// For now, this is a placeholder that discovers and lists tools.
// A full implementation would integrate with an MCP library.
func runMCPServer(ctx context.Context, stderr io.Writer) error {
	// Build the command tree to discover tools
	root := NewRootCmd(DefaultDeps())
	tools := discoverTools(root, "")

	// For now, output the tool list as JSON to stderr (real implementation
	// would use proper MCP framing on stdout). This proves discovery works.
	toolList := map[string]interface{}{
		"tools": tools,
		"count": len(tools),
	}

	data, err := json.MarshalIndent(toolList, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tools: %w", err)
	}

	// Write to stderr for now (debugging)
	fmt.Fprintf(stderr, "Discovered %d tools:\n%s\n", len(tools), string(data))

	// TODO: Implement full MCP stdio protocol
	// - Read tool requests from stdin
	// - Execute requested tools with captured output
	// - Return structured MCP responses on stdout
	// - Handle errors and validation

	return nil
}

// executeToolCommand runs a named tool and returns its output.
// This will be used by the MCP protocol handler to execute tools.
func executeToolCommand(ctx context.Context, root *cobra.Command, toolName string, args []string) (stdout, stderr string, err error) {
	// Find the command in the tree by reconstructing from the tool name
	parts := strings.Split(toolName, "_")
	if len(parts) == 0 {
		return "", "", fmt.Errorf("invalid tool name: %s", toolName)
	}

	// Navigate the command tree
	cmd := root
	for _, part := range parts {
		found := false
		for _, subcmd := range cmd.Commands() {
			if subcmd.Name() == part {
				cmd = subcmd
				found = true
				break
			}
		}
		if !found {
			return "", "", fmt.Errorf("command not found: %s", toolName)
		}
	}

	// Capture output
	var outBuf, errBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)

	// Execute the command
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		return outBuf.String(), errBuf.String(), err
	}

	return outBuf.String(), errBuf.String(), nil
}
