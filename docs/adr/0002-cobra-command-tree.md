# ADR-0002: Use cobra for the command tree

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

The extension will have several subcommands. Something has to parse
`gh cli-extension <cmd> [flags]`, generate help, and validate arguments.

`gh` itself is built on cobra, so its help output, flag conventions, and error
phrasing are what users of an extension already expect.

## Decision

Use `github.com/spf13/cobra`. Every subcommand is a `new<Name>Cmd()
*cobra.Command` constructor registered in `NewRootCmd` in
`internal/cmd/root.go`.

Constructors, not package-level `var` commands: a package-level command is
shared mutable state, and cobra retains parsed flag values on it, so tests
leak into one another.

## Consequences

- Help, `--help` on every level, and shell completion come for free.
- Output matches what `gh` users expect.
- `NewRootCmd` returning a fresh tree makes the whole CLI testable against a
  `bytes.Buffer` — this is what the `runRoot` helper in the tests relies on.
- One direct dependency (plus `pflag`, `mousetrap`) for what a small extension
  could do with `flag`. Accepted as the cost of matching `gh`'s conventions.

## Alternatives considered

- **`flag` from the standard library.** No subcommand support; we would
  hand-roll dispatch and help text.
- **`urfave/cli`.** Fine library, but diverges from `gh`'s own idiom for no
  gain here.

## Revisit when

Never expected. If cobra were dropped, the `internal/cmd` package would be
rewritten wholesale.
