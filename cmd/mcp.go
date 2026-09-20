package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
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

// runMCPServer implements the MCP stdio protocol handler via JSON-RPC 2.0.
func runMCPServer(ctx context.Context, stderr io.Writer) error {
	// Create root once for tool discovery
	root := NewRootCmd(DefaultDeps())
	tools := discoverTools(root, "")

	// Server stores deps for creating fresh roots on each command execution
	server := &mcpServer{
		deps:  DefaultDeps(),
		tools: tools,
		log:   stderr,
	}
	return server.run(ctx)
}

// mcpServer handles MCP protocol messages over stdin/stdout.
type mcpServer struct {
	deps  Deps
	tools []MCPTool
	log   io.Writer
}

// mcpRequest and mcpResponse represent JSON-RPC 2.0 protocol frames.
type mcpRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	ID      interface{}            `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// run starts the MCP server loop.
func (s *mcpServer) run(ctx context.Context) error {
	scanner := newJSONRPCReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout)

	for {
		req := &mcpRequest{}
		if err := scanner.Scan(req); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("reading request: %w", err)
		}

		resp := s.handleRequest(ctx, req)
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(s.log, "error encoding response: %v\n", err)
		}
	}

	return nil
}

// handleRequest routes MCP method calls.
func (s *mcpServer) handleRequest(ctx context.Context, req *mcpRequest) *mcpResponse {
	resp := &mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = s.handleInitialize()
	case "tools/list":
		resp.Result = s.handleListTools()
	case "tools/call":
		result, err := s.handleCallTool(ctx, req.Params)
		if err != nil {
			resp.Error = &mcpError{Code: -32603, Message: err.Error()}
		} else {
			resp.Result = result
		}
	default:
		resp.Error = &mcpError{Code: -32601, Message: "method not found"}
	}

	return resp
}

// handleInitialize returns server capabilities.
func (s *mcpServer) handleInitialize() interface{} {
	return map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "gh-xeb",
			"version": version,
		},
	}
}

// handleListTools returns the tool catalog.
func (s *mcpServer) handleListTools() interface{} {
	tools := make([]map[string]interface{}, len(s.tools))
	for i, t := range s.tools {
		tools[i] = map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		}
	}
	return map[string]interface{}{
		"tools": tools,
	}
}

// handleCallTool executes a tool and returns the output.
func (s *mcpServer) handleCallTool(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	name, ok := params["name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing 'name' parameter")
	}

	// Extract arguments from params (all other keys)
	args := []string{}
	for k, v := range params {
		if k == "name" {
			continue
		}

		// Handle different value types
		switch val := v.(type) {
		case string:
			args = append(args, "--"+k, val)
		case bool:
			if val {
				args = append(args, "--"+k)
			}
		case float64:
			args = append(args, "--"+k, fmt.Sprintf("%v", val))
		case []interface{}:
			for _, item := range val {
				args = append(args, "--"+k, fmt.Sprintf("%v", item))
			}
		}
	}

	// Create a fresh root for this command execution
	root := NewRootCmd(s.deps)
	stdout, stderr, err := executeToolCommand(ctx, root, name, args)

	result := map[string]interface{}{
		"stdout": stdout,
	}
	if stderr != "" {
		result["stderr"] = stderr
	}
	if err != nil {
		result["error"] = err.Error()
	}

	return result, nil
}

// jsonRPCReader reads newline-delimited JSON from a reader.
type jsonRPCReader struct {
	r *bufio.Scanner
}

func newJSONRPCReader(r io.Reader) *jsonRPCReader {
	return &jsonRPCReader{r: bufio.NewScanner(r)}
}

func (jr *jsonRPCReader) Scan(v interface{}) error {
	if !jr.r.Scan() {
		if err := jr.r.Err(); err != nil {
			return err
		}
		return io.EOF
	}
	return json.Unmarshal(jr.r.Bytes(), v)
}

// executeToolCommand runs a named tool and returns its output.
// Uses the same approach as tests: creates a root, sets buffers, executes.
func executeToolCommand(ctx context.Context, root *cobra.Command, toolName string, args []string) (stdout, stderr string, err error) {
	// Tool name is either a single command (doctor) or nested (vault_get).
	// We need to pass the full command path as args to Execute.
	parts := strings.Split(toolName, "_")

	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)

	// Build the full argument list: [subcommand, subsubcommand, ..., --flag, value, ...]
	fullArgs := append(parts, args...)
	root.SetArgs(fullArgs)

	// Execute the command
	if err := root.ExecuteContext(ctx); err != nil {
		stderr = errBuf.String()
		if stderr == "" {
			stderr = err.Error()
		}
	}

	return outBuf.String(), errBuf.String(), err
}
