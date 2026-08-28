# ADR-0003: Depend on narrow interfaces in `internal/gh`, not on go-gh directly

- **Status:** Accepted — the package-level seam mechanism is superseded by
  [ADR-0009](0009-inject-dependencies.md); the rule to depend on narrow
  interfaces in `internal/gh` still stands.
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`api.DefaultRESTClient()` returns a concrete `*api.RESTClient` that talks to
github.com. A command that calls it directly can only be tested by hitting the
network — which is slow, requires credentials, and fails in CI.

Extensions also have a second way to reach GitHub: shelling out to the `gh`
binary via `gh.Exec`. It is tempting because it is one line, but it means
parsing human-formatted output and paying a process spawn per call.

## Decision

`internal/gh` owns the go-gh dependency and exposes:

- `RESTClient`, an interface containing only the methods this extension uses
  (`Get` today);
- `NewRESTClient()`, returning that interface;
- `CurrentRepo()`, wrapping `repository.Current()` in a local `Repo` type.

Commands depend on those. Each command that needs a client reads it through a
package-level seam (`var newRESTClient = gh.NewRESTClient`) which tests replace
and restore with `t.Cleanup`.

Prefer go-gh over `gh.Exec`. Shell out only for behaviour that genuinely exists
only in the `gh` binary.

## Consequences

- Tests run offline, in milliseconds, with no credentials. `whoami_test.go`
  fakes a REST response with a five-line struct.
- Adding a go-gh capability means widening a small interface deliberately,
  which keeps the surface we depend on visible in one file.
- A package-level `var` seam is mutable global state. It is acceptable because
  the scope is one package and every test restores it, but it does not survive
  parallel tests that stub different clients — do not add `t.Parallel()` to
  those.
- Slight indirection: `internal/gh.Repo` duplicates `repository.Repository`.

## Alternatives considered

- **Call go-gh from commands directly.** Simplest to write, untestable without
  the network.
- **Pass the client as a constructor parameter** (`newWhoamiCmd(client)`).
  Cleaner in principle, but every command would need it threaded through
  `NewRootCmd`, and most commands never touch the API. Reconsider if the seam
  count grows past two or three.

## Revisit when

A third distinct seam appears, or a test needs `t.Parallel()`. At that point
switch to constructor injection.
