# ADR-0001: Build the extension as a precompiled Go binary

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`gh` supports two kinds of extension. An *interpreted* extension is an
executable script at the repository root (bash is the documented default). A
*precompiled* extension ships per-platform binaries attached to a GitHub
release, which `gh extension install` downloads by filename convention.

This repository exists to explore what extensions can do, which means the code
will grow past "one API call and a `jq` filter": subcommands, flags, structured
output, tests.

## Decision

Write the extension in Go and distribute it as precompiled binaries.

## Consequences

- Real tests are possible (`go test`), which a bash extension effectively
  cannot have.
- `go-gh` gives first-class access to the user's `gh` auth, REST and GraphQL
  clients, and repository resolution — no reparsing of `gh` output.
- Users get a single static binary with no runtime dependency (`CGO_ENABLED=0`).
- The cost is release machinery: binaries must be cross-compiled and attached
  to a release, or `gh extension install` finds nothing. See
  [ADR-0006](0006-release-via-gh-extension-precompile.md).
- Installing from a local checkout now requires a build step first; the binary
  at the repo root is what `gh` actually executes.

## Alternatives considered

- **Bash script.** Zero build, zero release pipeline, and the documented
  starting point. Rejected: no test story, and the argument/flag handling we
  want would become unmaintainable in shell.
- **Another compiled language via `--precompiled=other`.** Would require
  hand-writing `script/build.sh` anyway, without `go-gh`'s auth handling.

## Revisit when

Never, realistically — reverting means rewriting the whole extension.
