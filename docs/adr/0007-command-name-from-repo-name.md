# ADR-0007: Accept `gh cli-extension` as the command name

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`gh` derives an extension's invocation name from its repository name: the repo
must be called `gh-<something>`, and `<something>` becomes the subcommand. This
is not configurable — it is how `gh` discovers and dispatches extensions.

This repository is `0xHackerSpace/gh-cli-extension`, so the extension is
invoked as `gh cli-extension`. That reads redundantly: "gh cli extension" says
the same word three times and describes the category rather than the tool.

## Decision

Keep the current name. Do not rename the repository as part of the initial
setup.

## Consequences

- The command stays `gh cli-extension`, which is awkward but harmless while
  the repo is a playground rather than a published tool.
- The name is load-bearing in more places than it looks: the binary at the repo
  root, the release asset names, the Go module path, the `-ldflags` version
  path in both `Makefile` and `script/build.sh`, and the install instructions.
  A rename touches all of them, plus the module path in every import.
- Renaming stays cheap only while the extension is unpublished. Once users have
  installed it, a repo rename changes the install path and the command they
  type.

## Alternatives considered

- **Rename the repository now** (e.g. `gh-hackerspace`, giving
  `gh hackerspace`). Better name, and cheapest to do today. Rejected for now
  because it is the user's call, not a technical necessity, and the repo
  already exists under this name with a remote configured.
- **Ship a differently-named binary.** Not possible; `gh` will not find it.

## Revisit when

Before the first published release, or before adding the `gh-extension` topic
that makes the repo discoverable — those are the last cheap moments to rename.
