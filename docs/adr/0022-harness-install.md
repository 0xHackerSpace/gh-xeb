# ADR-0022: `harness install` generates the agent skill from the live command tree

- **Status:** Accepted
- **Date:** 2026-09-01
- **Deciders:** @IanOliv

## Context

Coding agents are how this extension will mostly be driven. An agent that does
not know `gh xeb` exists will shell out to `vault` and `curl` instead, and an
agent that knows the commands but not the rules will do the wrong thing
confidently: print a secret it was only asked to check, pass a token as an
argument, or run `backstage create` to "see what happens".

Both Claude Code and Copilot already read a skill file for exactly this. The
repository has been documenting itself for humans — `AGENTS.md`, `README.md`,
twenty-one ADRs — and none of it reaches an agent working in *another*
repository, which is where this extension is used.

Two things had to be decided: where the file goes, and where its content comes
from.

## Decision

`gh xeb harness install <agent> [--global]` writes a `SKILL.md`.

| agent | project | `--global` |
| --- | --- | --- |
| `claude` | `.claude/skills/xeb/SKILL.md` | `~/.claude/skills/xeb/SKILL.md` |
| `copilot` | `.github/skills/xeb/SKILL.md` | `~/.copilot/skills/xeb/SKILL.md` |

Those roots were read off a machine that has both agents installed, not taken
from documentation. Both read the same format — frontmatter with `name` and
`description`, then prose — and differ only in where they look, which is why
agents are a table rather than a rendering strategy each.

**The content is generated from the live command tree.** `renderSkill` takes
the `*cobra.Command` this process built and walks it, so the command list in
the skill is the command list in the binary that wrote it. Registering a
subcommand and reinstalling is the whole update procedure.

What a walk cannot produce is written as prose: the rules an agent cannot infer
from `--help` — never pass a token as an argument, `backstage create` is the
only command that changes anything outside the machine, secret values are
masked on purpose, `repoUrl` is not a URL.

An existing file is never replaced without `--force`; `--dry-run` prints the
destination and the document; `--path` escapes the convention.

## Consequences

- **The skill cannot drift from the binary.** A test walks the real command
  tree and fails if the generator stops seeing a command, so forgetting to
  document a subcommand for agents is impossible by construction. This is the
  reason to generate rather than write it, and it is worth more than the prose
  quality that a hand-written file would have.
- **The prose half still drifts.** The rules are a string constant. Changing
  `backstage create` to stop mutating, or adding a `--token` flag, would leave
  the skill lying — and nothing catches that. The generated half is safe, the
  written half is a normal documentation liability.
- **Two more seams on `Deps`**, `UserHomeDir` and `WorkingDir`, so a test can
  point an install at `t.TempDir()`. Without them the test suite would write
  into the developer's real home, which is the sort of thing a test does once
  before someone notices.
- **Four locations are now this project's problem.** If Claude or Copilot move
  where they look, `harness install` writes to the wrong place and says it
  succeeded. `--path` is the escape hatch, and the table is one edit.
- **Project scope writes into the user's repository**, which is a file they did
  not ask for in a directory they may be about to commit. The output says so
  and suggests committing it deliberately; refusing to overwrite is what keeps
  a second install from eating a hand-edited file.
- **`.github/skills/` for Copilot project scope is the least certain choice
  here.** `~/.copilot/skills/` was observed directly; the project root mirrors
  where this repository already keeps `copilot-instructions.md` and
  `instructions/`. If it turns out to be wrong, it is one table row.
- The skill records the version it was generated from, so a stale file is
  recognisable as stale rather than merely wrong.

## Alternatives considered

- **Ship a hand-written `SKILL.md` in the repository** and tell people to copy
  it. Better prose, and it is stale the first time a subcommand lands. The
  drift is the whole problem.
- **Generate from `--help` output** by shelling out to ourselves. Same result
  through a subprocess, and it would depend on cobra's help layout instead of
  its command tree.
- **One file per agent, written by hand per agent.** Both agents read the same
  format; the difference is a path, and encoding a path difference as two
  renderers would be inventing work.
- **`AGENTS.md` in the target repository.** Widely read, but it is the target
  project's file, describing that project. Appending a tool's manual to it is
  someone else's document to edit.
- **Making the rules a machine-checked contract** — asserting in a test that
  no `--token` flag exists anywhere, for instance. That test already exists for
  Backstage; extending the idea to every rule in the skill is the honest fix
  for the drift above, and is not done here.

## Revisit when

A third agent needs supporting, or one of the four paths stops being right.
Both are a table edit; the point of writing this down is that the next person
knows the table is the only place to look.
