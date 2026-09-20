# ADR-0018: `backstage` can take its address and token from a Vault secret

- **Status:** Accepted
- **Date:** 2026-09-01
- **Deciders:** @IanOliv
- **Builds on:** [ADR-0016](0016-backstage-catalog-command.md) and
  [ADR-0017](0017-vault-get-masked-by-default.md).

## Context

`backstage` needs an address and a bearer token.
[ADR-0016](0016-backstage-catalog-command.md) took the token from
`BACKSTAGE_TOKEN` only, on the grounds that a command-line argument is visible
in shell history and in `ps`. That rules out the worst option but leaves the
credential sitting in a shell profile, an `.envrc`, or a CI variable — copied
by hand to every machine that needs it, rotated nowhere.

The extension already talks to Vault, which is where that credential should
live. `internal/vault` can already read a KV secret
([ADR-0017](0017-vault-get-masked-by-default.md)). Nothing joined the two.

## Decision

`--vault-secret <path>` (or `BACKSTAGE_VAULT_SECRET`) reads the Backstage
address and token from a Vault KV secret. The fields are `url` and `token`,
with `base_url` and `api_token` accepted as aliases. Either may be absent.

Precedence, first match winning:

| | Order |
| --- | --- |
| url | `--url`, then the Vault secret, then `BACKSTAGE_BASE_URL`, `BACKSTAGE_URL` |
| token | the Vault secret, then `BACKSTAGE_TOKEN` |

Vault is reached through the existing `Deps.NewVaultClient`, so the whole chain
from [ADR-0015](0015-vault-in-memory-login.md) applies unchanged — including
the GitHub login fallback, which means `gh auth login` alone can be enough to
query a catalog the machine holds no credential for. `--vault-auth-path`
covers a GitHub auth method mounted somewhere other than `auth/github`.

**The wiring lives in `cmd/`.** `internal/backstage` does not import
`internal/vault`, and neither package knows the other exists.

## Consequences

- **A credential can stop living in the environment.** One Vault secret, read
  at the moment of use, rotated in one place. This is the first time the two
  services the extension talks to are joined rather than merely coexisting.
- **The two packages stay independent** because the join is in the command
  layer. A future `internal/backstage` consumer is not dragged into the Vault
  SDK, and the Backstage tests need no Vault fake.
- **Vault is contacted only when asked.** Without the flag or the environment
  variable, no Vault client is constructed and the command behaves exactly as
  before. A test asserts this: an accidental unconditional lookup would add a
  Vault dependency to every catalog query and break every user without one.
- **`--url` beats the Vault secret, but the token has no flag to compete
  with**, so Vault wins over `BACKSTAGE_TOKEN`. The asymmetry is deliberate:
  overriding the address to point at a staging instance should not silently
  cost you the credential. It is also the surprising part of this decision, and
  the only one worth checking the help text for.
- **Failure now has two origins.** A `backstage` command can fail because the
  catalog is unreachable or because Vault is. Both errors name the path or the
  URL they were working on, so the two are distinguishable without a flag.
- **Latency.** Reading the secret costs the mount preflight plus the read —
  two round trips to Vault before the first catalog request, and a GitHub login
  on top when no Vault token exists.
- **The token is passed from Vault to an outbound HTTP header without ever
  being rendered.** It appears in no output, no error and no log. The
  diagnostics for a malformed secret name fields, never values, and a test
  asserts a field value cannot reach the error text.

## Alternatives considered

- **Always try Vault when `BACKSTAGE_TOKEN` is unset.** Rejected: it makes
  every catalog query depend on a Vault server most users of the command will
  not have, and it turns a missing environment variable into a confusing Vault
  error.
- **A general `--from-vault key=path#field` mechanism** for any flag. More
  powerful and much harder to explain; there are two values to resolve, not an
  open set.
- **Configurable field names via flags.** Two more flags to document for a
  problem two aliases solve.
- **Caching the resolved token on disk.** Directly against
  [ADR-0015](0015-vault-in-memory-login.md), which chose not to install
  credentials on a machine. The whole point is that the token lives in Vault.
- **Reusing `vault get` by shelling out to ourselves.** Absurd, but it is what
  a shell script would do, and it is worth recording that the in-process client
  exists precisely so this does not have to.

## Revisit when

A third value needs resolving this way, or a second command wants the same
mechanism. Two call sites is the point at which the general
`--from-vault` design rejected above starts being worth its explanation.
