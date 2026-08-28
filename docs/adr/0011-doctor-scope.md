# ADR-0011: `doctor` reports Vault readiness without contacting Vault

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`doctor` is the first command written for the extension's actual purpose:
feeding GitHub identity attributes into HashiCorp Vault. Vault's GitHub auth
method authenticates a user with a GitHub token and maps their organisation and
team membership to policies — which means it needs the token to carry the
`read:org` scope, and needs the user to actually be in a mapped org and team.

Those are precisely the things that are invisible until authentication fails
with an unhelpful error. A diagnostic command is the right shape for them.

The open question was where the command stops: at the GitHub boundary, or does
it also probe the Vault server?

## Decision

`doctor` performs eight read-only checks in two sections:

**GitHub** — `gh` CLI version, authentication host and token source, the
authenticated identity, and the token's OAuth scopes.

**Vault readiness** — whether `read:org` is present, the visible organisation
memberships, the visible team memberships, and whether `VAULT_ADDR` is set.

It never opens a connection to a Vault server. `VAULT_ADDR` is read from the
environment and reported, nothing more.

Failures exit non-zero; warnings do not. A failing check returns a
`SilentError`, because the report has already explained the problem and the
entrypoint should not print it twice.

## Consequences

- `doctor` works with no Vault server reachable — on a laptop, in CI, before
  Vault is provisioned at all. Its most useful moment is precisely when Vault
  is not yet set up.
- No new dependency, so the binary stays around 7 MB. See
  [ADR-0010](0010-vault-official-sdk.md) for when that changes.
- The report is honest about what it does not know: it can say the token lacks
  `read:org`, but not whether the Vault server has the GitHub auth method
  enabled or how it maps teams to policies. Users may read more assurance into
  a green report than it offers — the command's help text says so explicitly.
- The exact report layout is pinned by a golden test. Changing a column width
  or a section order fails the test on purpose: the alignment is the feature,
  not an accident.
- `SilentError`, added in [ADR-0004](0004-error-handling-and-output.md) with no
  user, now has one.

## Alternatives considered

- **Also probe the Vault server** (health, seal state, `auth/github/` mount,
  team-to-policy mapping). Genuinely more useful when Vault is reachable, and
  the obvious next step. Rejected for now: it pulls in the client dependency
  and makes the command useless in exactly the offline situation where a
  diagnostic helps most.
- **GitHub-only health check, no Vault awareness.** Simpler, but leaves the
  `read:org` requirement — the single most common cause of a failed Vault
  GitHub login — undiagnosed.

## Revisit when

Vault communication lands. `doctor` should then grow a third section, ideally
skipped gracefully when `VAULT_ADDR` is unset rather than failing.
