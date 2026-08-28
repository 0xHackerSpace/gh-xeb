# Project memory

Durable context that is true about this project but not derivable from the code
or the git history. Read this first when picking the project back up, or when
starting a session with an AI assistant.

For *how* to write code here, see [`AGENTS.md`](../AGENTS.md). For *why* the
architecture is the way it is, see [`decisions.md`](decisions.md) and
[`adr/`](adr/).

Last updated: 2026-08-28.

## What this is, and what it is for

`0xHackerSpace/gh-cli-extension` is a **playground for exploring the GitHub CLI
extension API**, not a tool with a user problem to solve. The original README
said so plainly: "a repo para explorar o desenvolvimento de extensões para a gh
cli".

That framing decides a lot of arguments. Subcommands exist to demonstrate a
capability (`whoami` = REST via go-gh, `repo` = repository resolution), not
because someone needs them. Breadth of the extension API beats depth of any
single feature. There are no users to keep compatibility with yet.

## State as of 2026-08-28

Scaffolding is complete and verified end to end:

- Four working subcommands: `doctor`, `whoami`, `repo`, `version`.
- `doctor` is the first command written for the project's actual purpose:
  feeding GitHub identity attributes to HashiCorp Vault. It stops at the GitHub
  boundary on purpose — it reads `VAULT_ADDR` from the environment but never
  opens a connection to a Vault server ([ADR-0011](adr/0011-doctor-scope.md)).
  Verified against the real account on 2026-08-28: 7 passed, 1 warning.
- `doctor --json` emits those attributes as a structured document. That payload
  is a public contract other tooling consumes
  ([ADR-0012](adr/0012-doctor-json-output.md)) — treat a field rename as a
  breaking change.
- `make check` passes (gofmt, `go vet`, `go test`).
- `./script/build.sh` produces all nine platform binaries, with the version
  correctly injected via ldflags.
- CI, release, and Copilot-setup workflows exist and their YAML parses, but
  **none of them has ever run** — nothing has been pushed. First push is the
  first real test of the workflows.
- The extension is installed locally and working:
  `gh cli-extension whoami` → `You are @IanOliv (Ian Gabriel Oliveira de Sousa).`
- Nothing is committed. The whole scaffold is uncommitted in the working tree,
  on `main`, over the single `61c0a27 Initial commit`.

## Where the Vault work is heading

The extension exists to surface GitHub identity attributes — user, orgs, teams,
token scopes — for Vault, whose GitHub auth method maps org and team membership
to policies. That single fact explains most of `doctor`'s design: `read:org` is
checked because Vault needs it, and teams are listed because Vault maps them.

Two things are already decided but not yet built:

- Commands that talk to a live Vault server (probe the mount, generate policies
  from team membership) — see the open decisions in `decisions.md`.
- When that lands, it uses the official `hashicorp/vault/api` SDK, accepting
  roughly forty indirect dependencies and a jump from ~7 MB to ~20 MB in the
  binary ([ADR-0010](adr/0010-vault-official-sdk.md)). Do not quietly
  substitute hand-rolled HTTP calls — that option was considered and rejected.

## The development machine

Observed on 2026-08-28, WSL2 on Linux. These caused real friction, so they are
worth knowing before debugging something that is not actually broken.

- **Local Go is 1.21.0, `go.mod` requires 1.25.** This is fine and intentional
  — `GOTOOLCHAIN=auto` downloads what is needed. Do not "fix" it by lowering
  the `go` directive; `go-gh` genuinely requires 1.25. See
  [ADR-0008](adr/0008-go-version-pinned-in-go-mod.md).
- **`gh` is 2.4.0, from March 2022.** Old enough that newer extension
  subcommands (`gh extension create --precompiled=go`, for instance) may be
  missing. `gh extension install .` does work. Worth upgrading.
- The extension is installed as a **symlink to the checkout**, not a copy:
  `~/.local/share/gh/extensions/gh-cli-extension` → the repo directory. So
  `make build` updates the installed extension, and **`make clean` breaks
  `gh cli-extension` until the next build** (it deletes the binary `gh`
  executes).

## Gotchas found the hard way

**A module checksum mismatch was a corrupt local cache, not an attack.**
`go get` failed with `checksum mismatch` and a `SECURITY ERROR` banner on
`github.com/inconshreveable/mousetrap`. The downloaded hash disagreed with the
known-good one from `sum.golang.org`. `go clean -modcache` plus a fresh `go get`
fixed it and the checksums then matched. Recognise this before assuming
something sinister — but do verify the expected hash against `sum.golang.org`
rather than assuming it every time.

**The extension name is load-bearing in six places.** The repo name, the binary
at the repo root, the release asset names, the Go module path, and the ldflags
version path in both `Makefile` and `script/build.sh`. The ldflags one is the
dangerous one: get it wrong and the build still succeeds, silently reporting the
wrong version.

**`doctor` has two golden tests, both meant to be brittle.**
`TestDoctorReport` pins the text layout; `TestDoctorJSONPayload` pins the JSON
document. Both fail on purpose when the output changes — update the expected
value deliberately instead of loosening the assertion.

**The release build matrix is duplicated.** `script/build.sh` builds the real
artifacts; the `cross-compile` job in `ci.yml` only checks each target still
compiles. Add a platform to one and not the other and CI stops covering
something we ship.

## Conventions that are easy to get wrong

Full list in [`AGENTS.md`](../AGENTS.md); these are the ones an assistant
reliably breaks:

- No `os.Exit` or `log.Fatal` outside `main.go`.
- Print through `cmd.OutOrStdout()`, never `os.Stdout`.
- Tests never touch the network — start from `testDeps()` and override only
  what the test needs. There are no package-level seams any more
  ([ADR-0009](adr/0009-inject-dependencies.md)).
- Never add an interactive prompt.
- Never print or log a token. `internal/gh.Auth` deliberately carries the host,
  the source and a boolean, not the secret.

## Open threads

Tracked in full under "Open decisions" in [`decisions.md`](decisions.md). The
two with a deadline attached:

1. **Repo rename**, cheap only until the first published release.
2. **Licence**, required before anyone can legally use a published release.
