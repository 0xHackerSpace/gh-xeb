# ADR-0012: `doctor --json` emits identity attributes as a stable contract

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

The extension exists to surface GitHub identity attributes for use with Vault.
Until now `doctor` only rendered them for a human: teams appeared as the string
`"0xHackerSpace/platform"` inside an aligned column. A program wanting the team
list had to parse the report — which makes the layout, deliberately pinned by a
golden test in [ADR-0011](0011-doctor-scope.md), an accidental API.

Three shapes were possible. `gh` core's convention is `--json field1,field2`
with companion `--jq` and `--template` flags. A `--format json` switch is the
other common CLI idiom. Or a plain boolean `--json` that emits the whole
document.

## Decision

`doctor --json` is a boolean flag that emits the entire document, pretty-printed
with two-space indent, and suppresses the human report.

The payload has four top-level keys:

- `ghVersion` — the host CLI version.
- `identity` — the GitHub attributes: `host`, `authenticated`, `tokenSource`,
  `login`, `id`, `scopes`, `orgs`, `teams` (each `{org, slug}`).
- `vault` — `addr` from `VAULT_ADDR`, and `readOrgScope` as a boolean.
- `checks` and `summary` — the diagnostic, mirroring the text report.

Gathering is separated from rendering: `gather()` collects facts once, and both
the text report and the JSON payload are derived from the same `facts` value, so
the two can never disagree about a run.

**This payload is a public interface.** Adding a field is fine; renaming or
removing one is a breaking change. A golden test pins the whole document.

## Consequences

- Consumers read `.identity.teams[]` instead of parsing a column, so the text
  layout stops being an accidental API and can be changed freely.
- `readOrgScope` is broken out as a boolean even though it is derivable from
  `scopes`, because it is the single most common cause of a failed Vault GitHub
  login and a consumer should not have to know that to check it.
- Empty collections serialise as `[]`, never `null`, so `jq '.identity.teams[]'`
  works on an account with no teams. A test enforces this — Go's default for a
  nil slice is `null`, so it is one forgotten initialiser away from breaking.
- The exit code is unchanged: failures exit non-zero in both modes, and the
  JSON stays valid and complete when checks fail, so a CI job can read the
  document and the status.
- We now maintain two golden tests for one command. That is the price of
  treating both outputs as contracts.
- No `--jq` or `--template`. Users pipe to `jq`, which they already have if they
  are consuming JSON from a CLI.

## Alternatives considered

- **`--json field1,field2` like `gh` core.** The convention users of `gh` know,
  and it would let callers ask for less. Rejected: the field-selection
  machinery is a meaningful amount of code, and a diagnostic document is small
  enough that emitting all of it and letting `jq` filter is simpler for
  everyone. The cost is that `--json login` is now a usage error rather than a
  field request — `cobra.NoArgs` rejects it loudly, and a test covers that.
- **`--format json`.** Equivalent in practice; `--json` is what a `gh` user
  reaches for first.
- **A separate `identity` subcommand emitting only the attributes.** Duplicates
  the gathering for no benefit, and splits "what am I" across two commands.

## Revisit when

A consumer needs a field the payload does not carry, or the document grows
large enough that emitting all of it is wasteful.
