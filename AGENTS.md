# AGENTS.md

Shared instructions for AI coding assistants working in this repository.
Claude Code (`CLAUDE.md`) and GitHub Copilot (`.github/copilot-instructions.md`)
both point here, so this file is the single source of truth. Update it here
first, not in the pointer files.

Background lives in [`docs/`](docs/):
[`docs/memory.md`](docs/memory.md) for project context and known gotchas,
[`docs/decisions.md`](docs/decisions.md) and [`docs/adr/`](docs/adr/) for why
the architecture is the way it is. Read `docs/memory.md` before a non-trivial
change.

## What this project is

A GitHub CLI extension written in Go. It is installed with:

```
gh extension install 0xHackerSpace/gh-xeb
```

and invoked as `gh xeb <subcommand>`.

Two naming rules come from the `gh` CLI itself and must not be changed casually:

- The repository name must start with `gh-`. Ours is `gh-xeb`, so the
  extension's invocation name is everything after the prefix: `xeb`.
- The built binary at the repo root must be named exactly `gh-xeb`,
  and release assets must be named
  `gh-xeb-<goos>-<goarch>[.exe]`. `gh extension install` looks the
  binaries up by that convention; a rename breaks installation silently.

## Layout

```
main.go                     Thin entrypoint: calls cmd.Execute, maps errors to exit codes.
cmd/                        Cobra command tree. One file per subcommand.
  deps.go                   Deps struct: every way the tree reaches the outside world.
  root.go                   NewRootCmd(deps) wires subcommands together.
  whoami.go                 Example REST call through go-gh.
  repo.go                   Resolves the current repository like gh does.
  doctor.go                 GitHub identity + Vault-readiness diagnostics, text and JSON.
  vault.go                  Vault resource queries: overview, token, mounts, can, get.
  backstage.go              Backstage catalog: entities, ofertas, create, get, repo.
  harness.go                Generates the agent SKILL.md from the live command tree.
  json.go                   The shared --json encoder; doctor keeps its own.
  version.go                Version reporting; overridden via -ldflags at release.
  helpers_test.go           fakeREST, testDeps and runRoot, shared by every test.
internal/gh/client.go       Thin wrapper over go-gh; defines the RESTClient interface.
internal/vault/client.go    Thin wrapper over hashicorp/vault/api; defines Client.
  server_test.go            Drives the real SDK against an httptest stand-in Vault.
internal/backstage/         Backstage Software Catalog API client (read-only).
  client.go                 Transport, config, errors; defines Client.
  entity.go                 Entity model, references, query and filter building.
  server_test.go            Drives the client against an httptest stand-in catalog.
script/build.sh             Cross-compiles release binaries into ./dist.
docs/                       Project memory, decision log, and ADRs.
.github/workflows/ci.yml    fmt + tidy + vet + race tests + cross-compile.
.github/workflows/release.yml  Tag push -> cli/gh-extension-precompile.
```

## Commands

| Task | Command |
| --- | --- |
| Build the extension binary | `make build` |
| Run tests | `make test` |
| Formatting + vet | `make lint` |
| Everything CI runs | `make check` |
| Install this working copy as a local `gh` extension | `make install` |
| Cross-compile release artifacts | `./script/build.sh v0.0.0-dev` |

Go is pinned by `go.mod` (currently Go 1.25). If the local toolchain is older,
`GOTOOLCHAIN=auto` (the default) downloads the right one automatically.

## Conventions

- **Never call `os.Exit` or `log.Fatal` outside `main.go`.** Commands return
  errors; `main` decides the exit code. This keeps every command testable.
- **Write to `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, never to `os.Stdout`
  directly.** Tests execute the command tree against a buffer.
- **Wrap errors with context using `%w`**: `fmt.Errorf("fetching user: %w", err)`.
  Do not prefix messages with "error:" — the entrypoint adds the prefix.
- **Do not call Vault, Backstage or GitHub from tests.** For Vault, prefer the httptest
  stand-in in `internal/vault/server_test.go` when the decoding layer is in
  play — the `fakeVault` used by command tests bypasses every
  `map[string]interface{}` assertion, which is where the bugs live.
- **Do not call the GitHub API from tests.** Start from `testDeps()` and
  override only the fields the test needs, as `doctor_test.go` does. A command
  reaches the outside world only through the `Deps` struct it is constructed
  with — never through a package-level variable, and never through go-gh's
  concrete types. New capability means a new narrow interface in `internal/gh`
  plus a field on `Deps`.
- **Never print a token.** `gh.CurrentAuth` and `vault.Token` carry the host,
  the source and booleans, not secrets, so display code structurally cannot
  leak one. `gh.Token()` is the single exception and exists for one caller: the
  Vault GitHub login. Do not widen its use.
- **Prefer go-gh over shelling out to `gh`.** Use `gh.NewRESTClient()` for REST
  and `repository.Current()` for repo resolution. Only use `gh.Exec` when a
  behaviour genuinely only exists in the `gh` binary.
- **Never prompt interactively.** Extensions run in pipes and CI; take input
  from flags and arguments.
- `internal/gh` stays internal: it is an implementation detail and nothing
  outside this module may import it. `cmd/` is importable by other modules,
  so treat its exported names (`Deps`, `NewRootCmd`, `Execute`, `SilentError`)
  as a public surface — see [ADR-0013](docs/adr/0013-cmd-at-repo-root.md).

### Adding a subcommand

1. Create `cmd/<name>.go` with a `new<Name>Cmd(deps Deps)
   *cobra.Command` constructor — a constructor, not a package-level variable,
   so tests get a fresh command each run. Omit the parameter only if the
   command touches nothing outside the process, as `version` does.
2. Set `Short`, `Args`, and `RunE` (never `Run`).
3. Register it in `NewRootCmd` in `cmd/root.go`.
4. If it needs something not already on `Deps`, add the field there and wire it
   in `DefaultDeps()`.
5. Add `cmd/<name>_test.go` using `testDeps()` and `runRoot` from
   `helpers_test.go`.
6. Document the command in `README.md`.

## Releasing

Push a `v*` tag. `.github/workflows/release.yml` runs
`cli/gh-extension-precompile`, which invokes `script/build.sh` with the tag and
attaches every binary in `dist/` to the GitHub release. The tag is baked into
the binary via `-ldflags -X .../cmd.version`.

## Recording decisions

When you make a choice that is hard to reverse or that constrains future work,
add an ADR in `docs/adr/` (copy `docs/adr/0000-template.md`, take the next
number) and add a row to the index in `docs/adr/README.md` and
`docs/decisions.md`. Smaller choices whose reasoning is invisible in the code go
straight into the "Smaller decisions" section of `docs/decisions.md`.

Never edit an existing ADR's Context or Decision — supersede it with a new one
and change only the old status line.

## Things to leave alone unless explicitly asked

- The `doctor --json` payload. It is a public contract that other tooling
  consumes: adding a field is fine, renaming or removing one is a breaking
  change. Two golden tests pin the text report and the JSON document; if you
  change either, update the expected value deliberately rather than loosening
  the assertion.
- The `gh-` repository name and binary naming scheme.
- The release artifact naming in `script/build.sh`.
- `go.mod`'s module path — it is referenced by the `-ldflags` version injection
  in both the `Makefile` and `script/build.sh`.
