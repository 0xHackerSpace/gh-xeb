# ADR-0005: `AGENTS.md` is the single source of truth for AI assistants

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

This project is developed with two AI assistants, and they read different
files. Claude Code reads `CLAUDE.md`. GitHub Copilot reads
`.github/copilot-instructions.md` in the IDE and path-scoped
`.github/instructions/*.instructions.md` files.

Writing the conventions into both means they drift. The version an assistant
happens to read then contradicts the other, and neither matches the code.

## Decision

`AGENTS.md` at the repository root holds the conventions. The assistant-specific
files point at it and carry only what is genuinely specific to that assistant:

- `CLAUDE.md` imports it with `@AGENTS.md`, plus Claude-only notes (slash
  commands, the permission allowlist, the toolchain caveat).
- `.github/copilot-instructions.md` links to it and restates the always/never
  rules, because Copilot has no import syntax and does not reliably follow a
  cross-file pointer alone.
- `.github/instructions/*.instructions.md` scope rules to file globs, which
  is a Copilot-only capability.

A convention change goes into `AGENTS.md` first.

## Consequences

- One file to update and to review in a PR.
- Both assistants get the same conventions, and so does a human reading the
  repo.
- The Copilot file duplicates roughly a dozen lines by necessity. This is the
  known drift risk and the accepted cost of Copilot having no include
  mechanism — keep that section short so it stays cheap to re-sync.
- `AGENTS.md` is also the emerging cross-vendor convention, so a third
  assistant likely needs no new file.

## Alternatives considered

- **Symlink `CLAUDE.md` → `AGENTS.md`.** Breaks on Windows checkouts without
  `core.symlinks`, and Copilot still needs its own file.
- **Duplicate the full content in all three.** Guaranteed drift.
- **Only `CLAUDE.md`.** Leaves Copilot with no project context at all.

## Revisit when

Copilot gains a file-include mechanism — the duplicated block in
`.github/copilot-instructions.md` can then become a pointer.
