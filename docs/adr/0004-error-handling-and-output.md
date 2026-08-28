# ADR-0004: Commands return errors and write to cobra's streams

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

A CLI has two easy habits that make it untestable: calling `os.Exit`/
`log.Fatal` from wherever the failure happens, and printing to `os.Stdout`
directly. Both bypass any test harness — the first kills the test binary, the
second writes past the buffer the test is reading.

Extensions also run in pipes and in CI, so an interactive prompt is a hang, not
a question.

## Decision

Three rules, enforced by review and stated in `AGENTS.md`:

1. Only `main.go` may end the process. Commands use `RunE` and return errors.
2. Write through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never `os.Stdout`.
3. Never prompt interactively. Input comes from flags and arguments.

Errors are wrapped with context using `%w` and carry no `error:` prefix — the
entrypoint adds `gh cli-extension: `. `SilenceUsage` and `SilenceErrors` are
set on the root so cobra does not print a usage dump on a runtime failure.

## Consequences

- Every command is testable end to end against a `bytes.Buffer`, which is what
  `runRoot` does.
- Exit codes live in exactly one place and can be made richer later (distinct
  codes per failure class) without touching commands.
- Error messages compose into a readable chain instead of a bare
  `401 Unauthorized`.
- `SilentError` exists in `internal/cmd` for the case where a command has
  already reported the failure itself. Nothing uses it yet; it is there so the
  entrypoint contract is complete rather than retrofitted.

## Alternatives considered

- **`log.Fatal` on error.** Shorter by one line per call site, and gives up
  testability for the whole package.
- **Let cobra print errors.** Its default also prints full usage on a runtime
  error, which buries the actual message.

## Revisit when

We need distinct exit codes per failure class — that changes `main.go` only,
not this decision.
