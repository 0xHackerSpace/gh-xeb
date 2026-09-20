# ADR-0009: Inject dependencies through a `Deps` struct

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv
- **Amends:** [ADR-0003](0003-internal-gh-wrapper.md) — replaces its seam
  mechanism; its interface rule still stands.

## Context

[ADR-0003](0003-internal-gh-wrapper.md) made commands testable with a
package-level seam (`var newRESTClient = gh.NewRESTClient`) that tests replaced
and restored with `t.Cleanup`. It set an explicit trigger: *"Revisit when a
third distinct seam appears."*

`doctor` reached that trigger immediately. It needs the REST client, the auth
state, the `gh` binary version, and `os.Getenv` — four dependencies, on top of
the repository resolver `repo` already uses. Five package-level `var`s, each
saved and restored by hand in every test, is exactly the mutable global state
ADR-0003 flagged as the cost of the seam approach.

## Decision

`internal/cmd` defines a `Deps` struct holding every function through which the
command tree reaches the outside world. `DefaultDeps()` wires the real
implementations; `NewRootCmd(deps Deps)` takes it and passes it to each
subcommand constructor.

Tests build a `Deps` from `testDeps()` and override only the fields the test
cares about.

## Consequences

- No mutable global state left in `internal/cmd`. Tests can run in parallel
  safely, which the seam approach explicitly could not support.
- A command's dependencies are visible in its constructor signature instead of
  being discovered by reading its body.
- `testDeps()` gives every test a working, offline default, so a test that only
  cares about one dependency says so in one line.
- Adding a dependency means touching `Deps` and `DefaultDeps()`, which is a
  deliberate, reviewable step — the point, not an inconvenience.
- Commands that need nothing (`version`) still take no parameter, so the struct
  is not ceremony where it buys nothing.
- `Deps` will grow. If it reaches the point where most commands ignore most
  fields, split it per command group rather than letting it become a
  service locator.

## Alternatives considered

- **Keep the package-level seams.** Rejected by ADR-0003's own trigger, and
  five of them would make parallel tests impossible.
- **An interface per command** (`type doctorEnv interface { ... }`). More
  precise, but five near-identical interfaces for one small package is more
  ceremony than the problem deserves.
- **Pass a context-carried container.** Hides dependencies from signatures,
  which is the problem being solved.

## Revisit when

`Deps` grows past roughly eight fields, or most commands stop using most of it.
