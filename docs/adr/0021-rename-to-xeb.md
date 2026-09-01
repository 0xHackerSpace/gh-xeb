# ADR-0021: Rename the extension to `gh xeb`

- **Status:** Accepted
- **Date:** 2026-09-01
- **Deciders:** @IanOliv
- **Supersedes:** [ADR-0007](0007-command-name-from-repo-name.md)

## Context

[ADR-0007](0007-command-name-from-repo-name.md) kept the repository as
`gh-cli-extension`, accepting the redundant `gh cli-extension` because renaming
was the user's call rather than a technical necessity. It named its own
trigger: *"Revisit when: before the first published release, or before adding
the `gh-extension` topic — those are the last cheap moments to rename."*

That moment is now. Nothing has been pushed, no release exists, no one has the
extension installed except this machine, and no other module imports `cmd/`.
Every cost ADR-0007 listed is at its minimum.

The extension has also stopped being what ADR-0007 described. It is no longer a
playground demonstrating the extension API: it queries Vault, reads a Backstage
catalog, and runs scaffolder templates. `cli-extension` names the category; the
tool needs a name of its own.

## Decision

Rename to `xeb`.

| | Before | After |
| --- | --- | --- |
| Repository | `0xHackerSpace/gh-cli-extension` | `0xHackerSpace/gh-xeb` |
| Invocation | `gh cli-extension` | `gh xeb` |
| Module path | `github.com/0xHackerSpace/gh-cli-extension` | `github.com/0xHackerSpace/gh-xeb` |
| Binary | `gh-cli-extension` | `gh-xeb` |
| Release assets | `gh-cli-extension-<goos>-<goarch>` | `gh-xeb-<goos>-<goarch>` |

**Existing ADRs keep the old name.** They are historical records and the rule
in [`adr/README.md`](README.md) says only a status line may change. An ADR that
says `gh cli-extension` is not wrong; it is describing what was true when it
was written. Only this one and ADR-0007's status line mention the change.

## Consequences

- **The name was load-bearing in six places**, exactly as ADR-0007 warned: the
  repository, the binary at the repo root, the release asset names, the Go
  module path, the `-ldflags` version path in both `Makefile` and
  `script/build.sh`, and the install instructions. All six moved together, and
  `./script/build.sh v0.0.0-test` reporting the tag back is the check that the
  ldflags path is still right — it is the one that fails silently.
- **Two steps are outside this repository and are not done by this commit**:
  renaming the GitHub repository, and renaming the local checkout directory.
  The module path here is already `gh-xeb`, so until the GitHub rename happens
  `go get` against the remote would not resolve.
- **The installed extension breaks until it is reinstalled.** `gh` discovers
  extensions by directory and binary name, so the existing symlink at
  `~/.local/share/gh/extensions/gh-cli-extension` points at a binary that no
  longer exists. `gh extension remove cli-extension` then
  `gh extension install .` fixes it.
- **`cmd/` is a public surface** ([ADR-0013](0013-cmd-at-repo-root.md)), so the
  module path change is a breaking change for any importer. There are none —
  which is the entire reason to do this now rather than later.
- **The old name survives in the ADR history**, deliberately. A reader who
  greps for `cli-extension` will find it in `docs/adr/` and nowhere else, which
  is the correct answer to "what was this called before?"

## Alternatives considered

- **Keep `cli-extension`.** ADR-0007's position, and it was right at the time.
  What changed is that the tool acquired a purpose, and that the last cheap
  moment to rename is about to pass.
- **A descriptive name** (`gh-hackerspace`, `gh-platform`). Says more to a
  stranger, and locks the tool to a scope it may outgrow. `xeb` says nothing,
  which for a tool that already spans GitHub, Vault and Backstage is the point.
- **Renaming the ADRs too, for consistency.** Rejected: it would rewrite the
  record to say something that was never true, which is what the never-edit
  rule exists to prevent.

## Revisit when

Never, ideally. A second rename after publication costs users their installed
extension and the command they have in their shell history.
