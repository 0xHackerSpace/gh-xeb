# Architecture Decision Records

An ADR records one decision: the context that forced it, what we chose, and
what we now have to live with. It is written once, at the time of the decision,
and then left alone — an ADR is a historical record, not documentation that
gets refreshed.

When a decision changes, write a new ADR and mark the old one
`Superseded by ADR-XXXX`. Do not edit the original's decision or rationale;
only its status line changes.

## Writing one

Copy [`0000-template.md`](0000-template.md) to
`NNNN-short-title-in-kebab-case.md` using the next free number.

Keep it short. The parts that earn their space are **Consequences** (including
the bad ones) and **Revisit when** — those are what a reader six months from
now actually needs.

Not every choice needs an ADR. Write one when the decision is hard to reverse,
when it constrains future work, or when the reason would otherwise be
invisible in the code.

## Index

| # | Decision | Status |
| --- | --- | --- |
| [0001](0001-precompiled-go-extension.md) | Build the extension as a precompiled Go binary | Accepted |
| [0002](0002-cobra-command-tree.md) | Use cobra for the command tree | Accepted |
| [0003](0003-internal-gh-wrapper.md) | Depend on narrow interfaces in `internal/gh`, not on go-gh directly | Accepted (seam amended by 0009) |
| [0004](0004-error-handling-and-output.md) | Commands return errors and write to cobra's streams | Accepted |
| [0005](0005-agents-md-single-source.md) | `AGENTS.md` is the single source of truth for AI assistants | Accepted |
| [0006](0006-release-via-gh-extension-precompile.md) | Release with `cli/gh-extension-precompile` and an explicit build script | Accepted |
| [0007](0007-command-name-from-repo-name.md) | Accept `gh cli-extension` as the command name | Accepted |
| [0008](0008-go-version-pinned-in-go-mod.md) | Pin the Go toolchain in `go.mod` and let `GOTOOLCHAIN` fetch it | Accepted |
| [0009](0009-inject-dependencies.md) | Inject dependencies through a `Deps` struct | Accepted |
| [0010](0010-vault-official-sdk.md) | Use `hashicorp/vault/api` when the extension talks to Vault | Accepted (applied by 0014) |
| [0011](0011-doctor-scope.md) | `doctor` reports Vault readiness without contacting Vault | Accepted |
| [0012](0012-doctor-json-output.md) | `doctor --json` emits identity attributes as a stable contract | Accepted |
| [0013](0013-cmd-at-repo-root.md) | Move the command tree from `internal/cmd` to `cmd` | Accepted |
| [0014](0014-vault-command.md) | `vault` reports the resources a token can reach | Accepted |
| [0015](0015-vault-in-memory-login.md) | Log in to Vault in memory, never writing a token to disk | Accepted |
