# GitHub Copilot instructions

This repository's full conventions are documented in
[`AGENTS.md`](../AGENTS.md) at the repository root. Read it before proposing
changes; the rules below are the summary Copilot should always apply.

## Project

A GitHub CLI extension in Go, installed as
`gh extension install 0xHackerSpace/gh-xeb` and run as
`gh xeb <subcommand>`. Built on [cobra](https://github.com/spf13/cobra)
and [go-gh](https://github.com/cli/go-gh).

## Always

- Return errors from commands; only `main.go` may exit the process.
- Wrap errors with `fmt.Errorf("doing thing: %w", err)`.
- Print through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never `os.Stdout`.
- Use `RunE`, not `Run`, and set `Args` on every cobra command.
- Reach GitHub through `internal/gh`, and Vault through `internal/vault`, not
  by shelling out to the `gh` or `vault` binaries.
- Never print a token. Display code uses `gh.CurrentAuth`, which cannot carry
  a secret; `gh.Token()` exists only for the Vault login.
- Add a test for every new subcommand, using a fake client — tests must not
  touch the network.

## Never

- Never rename the binary or the release artifacts: `gh extension install`
  requires `gh-xeb` and `gh-xeb-<goos>-<goarch>[.exe]`.
- Never add interactive prompts; extensions run in pipes and CI.
- Never introduce a dependency without a clear need — this extension should stay
  installable as a single static binary (`CGO_ENABLED=0`).

## Verifying

Run `make check`. That is exactly what `.github/workflows/ci.yml` runs.
