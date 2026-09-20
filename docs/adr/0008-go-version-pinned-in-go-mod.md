# ADR-0008: Pin the Go toolchain in `go.mod` and let `GOTOOLCHAIN` fetch it

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`go-gh/v2 v2.13.0` declares `go >= 1.25.0`, so `go get` set our `go` directive
to `1.25.0`. The development machine has Go 1.21.0 installed. With the default
`GOTOOLCHAIN=auto`, Go 1.21 noticed the requirement and downloaded Go 1.26.7
to run the build.

The tempting "fix" is to lower the `go` directive back to 1.21 — which does not
work, because the dependency genuinely requires 1.25, and would only produce a
confusing failure later.

## Decision

Let `go.mod` be the single declaration of the required toolchain. Do not lower
the `go` directive to match an older local install.

CI resolves the version from the same file (`go-version-file: go.mod` in
`actions/setup-go`), so the workflow and the developer machine can never
disagree.

## Consequences

- One place defines the Go version; bumping it is a one-line change that CI
  picks up automatically.
- A developer with an older Go still builds successfully — the first build just
  pauses to download a toolchain (a few hundred MB, once).
- It fails for anyone who has set `GOTOOLCHAIN=local`, with an error that names
  the required version. That is the correct failure.
- Upgrading `go-gh` can silently raise our minimum Go version. Watch the `go`
  directive in Dependabot PRs.

## Consequences observed during setup

A stale entry in the module cache produced a `checksum mismatch` /
`SECURITY ERROR` on `github.com/inconshreveable/mousetrap`. It was a corrupt
local cache, not a compromised module: `go clean -modcache` followed by a fresh
`go get` resolved it and the checksums then matched `sum.golang.org`. Worth
recognising before assuming an attack.

## Alternatives considered

- **Lower the `go` directive to 1.21.** Does not compile — the dependency
  needs 1.25.
- **Add an explicit `toolchain` line.** Redundant here; the `go` directive
  already drives the download.
- **Pin an older `go-gh`.** Gives up current API and fixes for a local
  convenience.

## Revisit when

Go's toolchain-download behaviour changes, or the project needs to support an
environment with no network access at build time.
