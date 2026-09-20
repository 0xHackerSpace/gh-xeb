# ADR-0013: Move the command tree from `internal/cmd` to `cmd`

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

The cobra command tree lived at `internal/cmd`. Go gives `internal/` a compiler-
enforced meaning: a package under it can only be imported from within the
subtree rooted at its parent, so nothing outside this module could import the
command tree. That was deliberate — [ADR-0002](0002-cobra-command-tree.md) and
[ADR-0009](0009-inject-dependencies.md) both describe it under the old path.

A flatter layout was wanted: the command tree is the bulk of the code, and
burying it one level deeper than `main.go` for a module with no external
consumers bought nothing concrete.

## Decision

`internal/cmd` moves to `cmd`. `internal/gh` stays where it is.

The move is a `git mv`, so the file history follows.

## Consequences

- `github.com/0xHackerSpace/gh-cli-extension/cmd` is now importable by any
  module. Its exported names — `Deps`, `NewRootCmd`, `Execute`, `SilentError` —
  become a public surface with the compatibility expectations that implies.
  Nothing imports it today, and there is no promise of stability yet, but the
  compiler no longer prevents someone from depending on it.
- `internal/gh` keeps its enforced privacy, which is where it matters: it wraps
  go-gh and its interfaces are shaped for this extension's convenience, not for
  reuse.
- The `-ldflags` version path changed in three places — the `Makefile`,
  `script/build.sh`, and the doc comment in `cmd/version.go`. This is the
  failure mode `docs/memory.md` warns about: a stale `-X` path still compiles
  and silently reports the wrong version. Both build paths were re-verified
  after the move.
- The layout diverges from the widespread Go convention where `cmd/<name>/`
  holds one *main* package per binary. Here `cmd` is a single library package
  and `main.go` sits at the root. A Go developer arriving from another project
  may expect `cmd/gh-cli-extension/main.go` and be briefly surprised.
- ADRs 0002, 0004, 0006 and 0009 still say `internal/cmd`. They are historical
  records of decisions taken when that was the path, and are deliberately left
  untouched — only living documents (`AGENTS.md`, `README.md`, `docs/memory.md`,
  the assistant instruction files) were updated.

## Alternatives considered

- **Keep `internal/cmd`.** Compiler-enforced privacy for free, at the cost of
  the deeper layout. Rejected as the user's explicit preference.
- **`cmd/gh-cli-extension/main.go` with the tree as a library elsewhere.** The
  conventional Go shape for multi-binary repos. Rejected: this module builds
  exactly one binary, so the extra directory is ceremony.
- **Move `internal/gh` up as well.** Rejected: nothing about that package is
  meant for outside use, and `internal/` says so in a way a comment cannot.

## Revisit when

Someone outside this module wants to import `cmd`, at which point its exported
surface needs a deliberate review rather than being public by accident.
