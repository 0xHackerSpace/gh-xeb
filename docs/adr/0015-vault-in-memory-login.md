# ADR-0015: Log in to Vault in memory, never writing a token to disk

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`vault` ([ADR-0014](0014-vault-command.md)) needs a Vault token. The `vault`
CLI resolves one from `VAULT_TOKEN`, then `~/.vault-token`, and writes that file
on `vault login`.

This extension can do something neither of those does: it already holds a GitHub
credential, and Vault's GitHub auth method accepts exactly that. So when no
Vault token exists it can log in by itself, with no user action.

The question was what to do with the token it mints.

## Decision

Resolve in this order:

1. `VAULT_TOKEN`
2. `~/.vault-token`
3. log in through Vault's GitHub auth method with the `gh` token

A token from step 3 lives in memory for the invocation and is **not** written to
`~/.vault-token`.

The GitHub token is resolved lazily, through a function on `Options`, so the
common path — a Vault token already present — never pays to fetch a GitHub
credential it will not use. That matters because resolving it can shell out to
the system keyring.

`gh.Token()` is the single place the raw GitHub token is exposed, and it exists
for this one caller. `gh.CurrentAuth`, used by everything that prints, carries
the host, the source and a boolean, and structurally cannot leak the secret.

## Consequences

- The extension never installs a credential on a machine. Nothing to clean up,
  nothing left behind on a shared or ephemeral host, and no chance of
  overwriting a `~/.vault-token` the user put there deliberately.
- Steps 1 and 2 mean it cooperates with the `vault` CLI rather than competing:
  if the user has already run `vault login`, that token is used.
- Every invocation that reaches step 3 mints a new token, so it is slower, and
  accessors accumulate in Vault until they expire. A user running the command in
  a loop would notice both. `vault login` is the documented way out, and the
  help text says so.
- `--auth-path` exists because the GitHub auth method is not always mounted at
  `auth/github`.
- Logging in is a write: it creates a real credential in Vault. Everything else
  the command does is read-only, so a command that looks like a query does have
  one side effect on the server. The long help says this outright rather than
  burying it.

## Alternatives considered

- **Write to `~/.vault-token` like the CLI does.** One login instead of many,
  and the token is reusable by the `vault` CLI afterwards. Rejected: a query
  command should not silently install a credential, and clobbering a file
  another tool owns is worse than being slow.
- **Never log in; require an existing token.** Strictly read-only and the
  simplest to reason about. Rejected: it throws away the one integration that
  makes this extension worth having.
- **Cache in memory across invocations.** Not possible without a daemon.

## Revisit when

The per-invocation login becomes a real annoyance, at which point an explicit
opt-in flag to persist the token — never the default — is the answer.
