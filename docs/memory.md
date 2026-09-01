# Project memory

Durable context that is true about this project but not derivable from the code
or the git history. Read this first when picking the project back up, or when
starting a session with an AI assistant.

For *how* to write code here, see [`AGENTS.md`](../AGENTS.md). For *why* the
architecture is the way it is, see [`decisions.md`](decisions.md) and
[`adr/`](adr/).

Last updated: 2026-09-01.

## What this is, and what it is for

`0xHackerSpace/gh-cli-extension` is a **playground for exploring the GitHub CLI
extension API**, not a tool with a user problem to solve. The original README
said so plainly: "a repo para explorar o desenvolvimento de extensões para a gh
cli".

That framing decides a lot of arguments. Subcommands exist to demonstrate a
capability (`whoami` = REST via go-gh, `repo` = repository resolution), not
because someone needs them. Breadth of the extension API beats depth of any
single feature. There are no users to keep compatibility with yet.

## State as of 2026-09-01

Scaffolding is complete and verified end to end:

- Six working subcommands: `vault` (with `token`, `mounts`, `can`, `get`),
  `backstage` (with `entities`, `get`, `repo`), `doctor`, `whoami`, `repo`,
  `version`.
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
- Work is committed on the `feat/extension-scaffold-and-doctor` branch, over
  the single `61c0a27 Initial commit` on `main`. **Nothing has been pushed**,
  so no workflow has ever run and there is no PR.

## Where the Vault work is heading (mostly arrived)

The extension exists to surface GitHub identity attributes — user, orgs, teams,
token scopes — for Vault, whose GitHub auth method maps org and team membership
to policies. That single fact explains most of `doctor`'s design: `read:org` is
checked because Vault needs it, and teams are listed because Vault maps them.

The `vault` command landed on 2026-08-28 and does talk to a live server, using
the official `hashicorp/vault/api` SDK as ADR-0010 required. The dependency
graph went from 60 to 100 modules; that was the accepted price, so do not
quietly substitute hand-rolled HTTP calls.

What is still unbuilt is the original goal: **generating Vault policies from
GitHub team membership**. `doctor` surfaces the teams, `vault` shows what a
token resolves to, and nothing yet joins them. That is a write operation and
needs its own design.

`vault get` reads a KV secret but masks the values by default, showing only
which fields exist and how long each value is
([ADR-0017](adr/0017-vault-get-masked-by-default.md)). `--reveal` prints them
all, `--field <key>` prints one raw for piping. This diverges from the `vault`
CLI, which prints values — the divergence is the decision, not an oversight.

Two things about `vault` that are easy to get wrong:

- Mounts come from `sys/internal/ui/mounts`, never `sys/mounts`. The latter
  needs a privileged policy and fails for exactly the least-privileged users
  the command helps most.
- The login fallback mints a token per invocation and never writes it to
  `~/.vault-token` ([ADR-0015](adr/0015-vault-in-memory-login.md)). That is
  deliberate, not an oversight: a query command should not install credentials
  on a machine.

## Backstage arrived on 2026-09-01

`backstage` reads a Software Catalog: `entities` to list, `ofertas` for the
scaffolder templates and the inputs each asks for, `get` for one entity, `repo`
for whatever describes the repository you are standing in
([ADR-0016](adr/0016-backstage-catalog-command.md),
[ADR-0019](adr/0019-backstage-ofertas.md)).

`backstage create` runs a template ([ADR-0020](adr/0020-backstage-create.md)).
It is **the only thing this extension does that changes state in a system it
does not own** — a successful run creates a GitHub repository. There is no
confirmation prompt because AGENTS.md forbids one; the protection is that every
value is validated against the template's schema before anything is submitted,
plus `--dry-run`.

`--vault-secret <path>` takes the address and the token out of the environment
and reads them from a Vault KV secret instead
([ADR-0018](adr/0018-backstage-config-from-vault.md)). It is the first place
the two services the extension talks to are joined, and the join lives in
`cmd/` on purpose — `internal/backstage` does not import `internal/vault`.

Six things worth knowing before changing it:

- `internal/backstage` is **hand-written over `net/http`**, not an SDK, because
  Backstage publishes no official Go client. That is the opposite call from
  ADR-0010 on Vault and it was deliberate — there was an SDK worth paying for
  there, there is nothing to pay for here. The cost is that pagination cursors,
  the `filter` grammar and the error envelope are ours to keep correct.
- The token comes **only** from `BACKSTAGE_TOKEN`; there is no `--token` flag,
  and `TestBackstageHasNoTokenFlag` fails if one reappears.
- `backstage repo` matches on the `github.com/project-slug` annotation. An
  entity registered without it will not be found even though it describes the
  repository, and the output says which annotation it looked for rather than
  claiming the repository is unregistered.

- A task's **rendered** output is only in its completion event. `GET
  /tasks/{id}` returns `spec.output` with the `${{ }}` placeholders still in
  it, so printing that would show a user the template source instead of their
  new repository's URL.
- `repoUrl` is not a URL: the picker encodes it as
  `github.com?owner=acme&repo=payments`.
- Vault is contacted **only** when `--vault-secret` or
  `BACKSTAGE_VAULT_SECRET` is set. A test fails the build if that becomes
  unconditional, which would add a Vault dependency to every catalog query.

`backstage get` still does not render `spec.parameters` even though `ofertas`
does, which is an inconsistency a user will hit. Listed as an open decision,
along with the field-order problem: `ofertas` cannot reproduce the form's own
ordering because `Entity.Spec` is a decoded map.

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
wrong version. Moving the `cmd` package on 2026-08-28 changed that path in
three files at once — always re-run `./script/build.sh v0.0.0-test` and check
the binary reports the tag back.

**The command tree is at `cmd`, not `internal/cmd`.** It moved on 2026-08-28
([ADR-0013](adr/0013-cmd-at-repo-root.md)), which means it is importable from
outside the module — unlike `internal/gh`, which stayed private. ADRs written
before that date still say `internal/cmd`; they are historical records and were
left alone on purpose.

**`doctor` has two golden tests, both meant to be brittle.**
`TestDoctorReport` pins the text layout; `TestDoctorJSONPayload` pins the JSON
document. Both fail on purpose when the output changes — update the expected
value deliberately instead of loosening the assertion.

**`sys/health` is the one Vault endpoint that is not namespaced.** It exists
only at the root namespace, so sending `VAULT_NAMESPACE` with it returns a 404
"unsupported path". `internal/vault` strips the namespace for that call alone.
This is invisible on a dev server, where the variable is never set, and breaks
immediately on HCP Vault, where it always is.

**A local Backstage refuses the catalog API with 401 unless a token is sent.**
`permission.enabled: true` plus no `backend.auth.externalAccess` is the default
for a scaffolded instance. With the guest provider enabled, a working token
comes from `GET /api/auth/guest/refresh` (`.backstageIdentity.token`) and lasts
an hour, which is enough to try a command against real data without editing
`app-config`.

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
- Never print or log a token. `internal/gh.Auth` and `internal/vault.Token`
  deliberately carry the host, the source and booleans, not secrets.
  `gh.Token()` is the one function that returns the raw GitHub token, and it
  exists solely for the Vault login.

## Open threads

Tracked in full under "Open decisions" in [`decisions.md`](decisions.md). The
two with a deadline attached:

1. **Repo rename**, cheap only until the first published release.
2. **Licence**, required before anyone can legally use a published release.
