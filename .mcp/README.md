# MCP Configuration for gh-xeb

This directory contains Model Context Protocol (MCP) configuration for the `gh-xeb` GitHub CLI extension, allowing AI clients like Claude to discover and invoke CLI tools with full type safety.

## Files

- **`config.json`** — MCP server configuration and metadata
- **`tools.json`** — Complete tool catalog with schemas and descriptions
- **`README.md`** — This file

## Setup

### Prerequisites

1. Install the `gh-xeb` extension:
   ```sh
   gh extension install 0xHackerSpace/gh-xeb
   ```

2. Authenticate with GitHub:
   ```sh
   gh auth login
   ```

3. (Optional) Set up Vault access:
   ```sh
   export VAULT_ADDR=https://vault.example.com
   export VAULT_TOKEN=s.xxxxxxxxxxxxxx
   # OR let gh-xeb use GitHub auth to Vault
   ```

4. (Optional) Set up Backstage:
   ```sh
   export BACKSTAGE_BASE_URL=https://backstage.example.com
   export BACKSTAGE_TOKEN=xxxx
   ```

### For Claude Users

In Claude Code settings or via claude.ai, add an MCP server pointing to `gh-xeb`:

```json
{
  "mcpServers": {
    "gh-xeb": {
      "command": "gh",
      "args": ["xeb", "mcp"],
      "disabled": false
    }
  }
}
```

Claude will discover all tools automatically and make them available in conversations.

## Available Tools

### Vault Tools

- **`vault`** — Show Vault server info, token details, and visible mounts
- **`vault_token`** — Token details (policies, TTL, entity)
- **`vault_mounts`** — Visible secret engines
- **`vault_can <path>`** — Check capabilities at a path
- **`vault_get <path>`** — Read a secret (masked by default)

### Backstage Tools

- **`backstage`** — Show catalog overview
- **`backstage_entities`** — List entities with filters
- **`backstage_ofertas`** — List scaffolder templates
- **`backstage_get <ref>`** — Fetch one entity
- **`backstage_repo`** — Entities for current repository
- **`backstage_create <template>`** — Run a template ⚠️ side-effecting

### Other Tools

- **`harness_install <agent>`** — Generate agent skill
- **`doctor`** — Check GitHub and Vault readiness
- **`whoami`** — Current GitHub user
- **`repo`** — Resolved repository

## Tool Execution Flow

When Claude (or another MCP client) invokes a tool:

1. **Request** — Tool name, arguments, and flag values arrive via MCP
2. **Validation** — Arguments are validated against the tool's schema
3. **Execution** — The same command path as CLI invocation runs
4. **Capture** — stdout, stderr, and exit status are captured
5. **Response** — Output is returned to the client as structured MCP response

## Security Notes

- **Credentials are never logged or printed** — Tokens are passed through without exposure
- **Use `--mask` with `vault_get`** — Hides secret values in output (default: true)
- **Side effects are marked** — `backstage_create` is the only write operation
- **Tool schemas don't expose defaults** — Prevents leaking sensitive values

## Example Invocations

### List Vault resources
```
Tool: vault
Arguments: {}
Options: --json
```

### Check Vault capabilities
```
Tool: vault_can
Arguments: {path: "kv/data/prod/db"}
```

### Query Backstage components
```
Tool: backstage_entities
Arguments: {kind: ["component"], type: ["service"]}
Options: --json
```

### Get GitHub user
```
Tool: whoami
Arguments: {}
```

## Troubleshooting

### Tools not discovered
- Ensure `gh extension list` shows `gh-xeb` installed
- Run `gh xeb doctor` to check authentication
- Check stderr output from MCP server startup

### Authentication errors
- GitHub: Run `gh auth login` first
- Vault: Ensure `VAULT_ADDR` is set and accessible
- Backstage: Set `BACKSTAGE_BASE_URL` and `BACKSTAGE_TOKEN`

### "Missing credentials" errors
- For Vault: Try `vault login` or ensure GitHub auth to Vault is configured
- For Backstage: Ensure a token or Vault secret is configured
- Use `gh xeb doctor` to diagnose

## References

- [MCP Specification](https://modelcontextprotocol.io/)
- [gh-xeb Documentation](../README.md)
- [gh-xeb ADR-0023: MCP Server Command](../docs/adr/0023-mcp-server-command.md)
