# ADR-0023: MCP server command for programmatic access

- **Status:** Proposed
- **Date:** 2026-09-20
- **Deciders:** Ian Oliveira

## Context

The `gh xeb` CLI provides a rich set of commands for querying Vault, Backstage,
and the GitHub API. Currently, these are only accessible through shell invocation,
which limits integration with tools that expect a standardized API.

The Model Context Protocol (MCP) is an emerging standard for AI tools and other
clients to discover and invoke capabilities in a structured way, with request/response
validation, error handling, and type safety built in.

Implementing an MCP server within `gh xeb` would allow:
- Claude and other AI models to use the CLI's functionality directly
- Clients to discover available commands without parsing help text
- Type-safe invocation with JSON schemas derived from Cobra flags
- Better integration with workflows that mix shell and programmatic access
- A single entrypoint for both humans (CLI) and machines (MCP)

This aligns with the design principle of keeping the CLI testable and injectable
(`Deps` struct), making it natural to wrap the same command tree for MCP.

## Decision

Implement `gh xeb mcp` as a subcommand that:

1. **Traverses the Cobra command tree** recursively to discover all executable
   commands, starting from `cmd.NewRootCmd(deps)`.

2. **Exposes commands as MCP tools** with:
   - A stable, collision-free name (e.g., `vault_get`, `backstage_entities`)
   - The command's short description and full help text
   - A JSON schema for inputs derived from the command's flags and arguments
   - Categorization by scope (vault, backstage, harness, etc.)

3. **Maps Cobra flags to JSON schema properties:**
   - Required arguments (cobra.ExactArgs) → required properties
   - Optional flags → optional properties with appropriate types
   - StringSlice flags → array types
   - Boolean flags → boolean types
   - Enums (where documented) → enum constraints
   - Descriptions from flag usage strings

4. **Executes tools by invoking the same Cobra command**, preserving:
   - Dependency injection (using the same `Deps` that was passed in)
   - Input validation (Cobra's existing validation runs)
   - Error handling (errors are returned as structured MCP error responses)
   - Output capture (stdout and stderr are recorded separately)

5. **Reserves stdout for the MCP protocol** by:
   - Redirecting command output to a buffer instead of os.Stdout
   - Returning the captured output as part of the MCP response
   - Logging all diagnostic output through the MCP transport itself

6. **Protects sensitive information** by:
   - Never including token values in responses or logs
   - Masking credentials in command output (reusing `vault get --mask`)
   - Documenting which tools have side effects (e.g., `backstage create`)

7. **Implements the MCP stdio transport** using an established Go library
   (e.g., `github.com/mark3labs/mcp-go` or similar) to:
   - Read tool requests from stdin
   - Write tool responses to stdout
   - Handle protocol framing and error cases
   - Log diagnostics to stderr

## Consequences

**Positive:**
- AI models and other clients gain full access to the CLI's capabilities
- The command tree is automatically exposed; new commands become tools without
  extra work
- The same validation, error handling, and Deps injection apply to both CLI and
  MCP invocations, reducing duplication and bugs
- The MCP server can be tested alongside the CLI using the same test helpers
- A clear, structured API replaces ad-hoc shell parsing

**Negative:**
- Adds a new dependency (MCP library) and transport complexity
- Commands with interactive prompts or TUI output cannot be meaningfully exposed
  (mitigated by the convention that extensions never prompt interactively)
- Side-effecting commands (like `backstage create`) are discoverable and could be
  invoked by accident through clients; requires clear documentation
- The Cobra tree must remain compatible with both CLI and MCP (e.g., no commands
  can assume Stdout is a terminal)

**To live with:**
- MCP tool names must be stable across releases; renaming a command is a breaking
  change for MCP clients
- The MCP server must be maintained alongside the CLI; changes to flags or help
  text affect the generated schema
- Some commands may not translate cleanly to tools (e.g., those designed for human
  reading); these are acceptable limitations

## Alternatives considered

### 1. Separate REST API server
Start a separate HTTP server that wraps the CLI. Would be more widely compatible
but adds operational complexity (port management, separate service lifecycle,
authentication). Dismissed: the goal is tight CLI integration, not a standalone service.

### 2. Shell command wrapping in clients
Keep the CLI as-is and have clients parse `gh xeb --help` and shell output.
Simpler for us, but fragile (parsing breaks on output format changes) and forces
clients to implement validation. Dismissed: errors and edge cases are invisible
to clients.

### 3. MCP plugin vs. embedded server
Instead of `gh xeb mcp`, build a separate `mcp-xeb` binary that shells out to `gh xeb`.
Cleaner separation, but loses the ability to share Deps and validation. Dismissed:
the whole point is to avoid the overhead of subprocess invocation and ensure
consistency.

### 4. Lazy tool discovery
Instead of traversing the tree at startup, discover tools on demand. Slightly
faster startup, but complicates testing and bookkeeping. Dismissed: the tree is
small and startup cost is negligible.

## Revisit when

- The MCP standard changes in a way that breaks compatibility with the current
  implementation
- A significant portion of CLI commands become unable to be expressed as MCP tools
  (e.g., due to interactive prompts becoming mandatory)
- The MCP library we choose becomes unmaintained or unsuitable
