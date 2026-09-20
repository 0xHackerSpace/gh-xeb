# ADR-0017: `vault get` reads secret values but masks them by default

- **Status:** Accepted
- **Date:** 2026-09-01
- **Deciders:** @IanOliv
- **Extends:** [ADR-0014](0014-vault-command.md), whose *Revisit when* named
  this exact trigger: "Browsing or reading secrets is wanted, which is the
  point where displaying sensitive material needs its own decision."

## Context

Everything the `vault` command did until now was metadata: the server, the
token's policies, the visible mounts, the capabilities at a path. Nothing it
printed was sensitive, so nothing it printed needed a decision.

Reading a KV secret breaks that. The natural implementation — fetch the secret,
print the fields — puts credentials in three places at once: the terminal
scrollback, the shell's session log where one exists, and, when the command is
run in a pipeline, the CI job's stored output. All three outlive the moment the
user wanted the value for. The `vault` CLI itself prints values unmasked, so
matching it is a defensible default and also the one that leaks.

There is a second, unrelated problem. `vault kv get secret/prod/db` takes a
*logical* path, but the API path is `secret/data/prod/db` — and only for KV v2.
For KV v1 there is no `data/` segment. A command cannot know which to send
without knowing the engine version.

## Decision

`vault get <path>` reads a KV secret and prints the field names with each value
replaced by a mask that keeps only its length:

```
secret/prod/db  kv-v2, version 3

  password   ******** (24 chars)
  username   ******** (8 chars)

  --reveal to print values, --field <key> for one
```

Two ways out of the mask, and no third:

- `--reveal` prints every value. An explicit request.
- `--field <key>` prints one value raw and unadorned, for piping. Asking for a
  named field *is* the request, so it is never masked; combining it with
  `--reveal` is an error rather than a no-op.

`--json` masks unless `--reveal` is given, and the payload carries a `masked`
boolean so a consumer can tell which it received.

The path is resolved by asking `sys/internal/ui/mounts/<path>` which engine
backs it and whether it is KV v2 — the same preflight the `vault` CLI performs
— and rewriting to the `data/` form only when it is. A path that already
carries `data/` is passed through unchanged.

## Consequences

- **The safe thing is what happens when you type the short command.** Getting a
  secret into a terminal now takes a deliberate flag, and the flag is visible
  in the shell history of whoever did it.
- **The mask still leaks the length**, deliberately. It distinguishes an empty
  field from a populated one and a passphrase from a fingerprint, which is most
  of why people look at a secret they already know the value of. For a
  short-alphabet secret — a 4-digit PIN — the length is close to the whole
  secret. Accepted: Vault is not where 4-digit PINs live.
- **We diverge from the `vault` CLI**, which prints values. Someone who
  transfers muscle memory across will be surprised once, then read the hint
  line printed under the table.
- **`--field` is the piping path and is unmasked**, so a script written against
  it silently keeps working if the mask ever changes. That is the intent.
- **A v2 secret literally named `data` is unreachable** through the logical
  path, because the rewrite cannot distinguish `secret/data` the secret from
  `secret/data/` the API prefix. The `vault` CLI has the same ambiguity, and
  the API path still works as an escape hatch.
- **Every read costs two round trips**: the mount preflight, then the secret.
  Caching the mount table would remove the first, and is not worth the
  invalidation problem for a command that runs once per invocation.
- Non-string values are re-encoded as JSON rather than formatted with Go's
  `%v`, which would print `map[a:b]` and be useless to paste anywhere.

## Alternatives considered

- **Print values, like the `vault` CLI.** Rejected: the CLI's default was set
  in 2015 for an interactive operator at a workstation, and this command will
  be run in CI by people who never chose that default.
- **A `--mask` flag, off by default.** Same leak with a longer name. A safety
  default that must be opted *into* is not a safety default.
- **Masking without the length** (a fixed `********`). Cheaper to reason about,
  but throws away the one piece of information that makes a masked read useful
  at all.
- **Redacting only fields whose name looks secret** (`password`, `token`,
  `key`). Rejected outright: a heuristic that is right 95% of the time is a
  security control that fails silently in the 5%, and it teaches users to trust
  it.
- **Guessing the KV version from the path** instead of the preflight. Works
  until someone mounts KV v1 at `kv/` and KV v2 at `secret/`, which is the
  common layout during a migration.

## Revisit when

Writing a secret is wanted. That is a different decision again: it is the first
mutating operation on a secret engine, and it needs an answer for where the
value comes from, since it must not come from a command-line argument.
