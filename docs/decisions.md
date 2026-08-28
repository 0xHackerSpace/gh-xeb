# Decision log

Every decision taken on this project, newest last. Substantial ones get an ADR;
the rest are recorded here in a line or two so the reasoning is not lost.

## Recorded as ADRs

| # | Decision | Date | Status |
| --- | --- | --- | --- |
| [0001](adr/0001-precompiled-go-extension.md) | Build the extension as a precompiled Go binary | 2026-08-28 | Accepted |
| [0002](adr/0002-cobra-command-tree.md) | Use cobra for the command tree | 2026-08-28 | Accepted |
| [0003](adr/0003-internal-gh-wrapper.md) | Depend on narrow interfaces in `internal/gh`, not on go-gh directly | 2026-08-28 | Accepted (seam amended by 0009) |
| [0004](adr/0004-error-handling-and-output.md) | Commands return errors and write to cobra's streams | 2026-08-28 | Accepted |
| [0005](adr/0005-agents-md-single-source.md) | `AGENTS.md` is the single source of truth for AI assistants | 2026-08-28 | Accepted |
| [0006](adr/0006-release-via-gh-extension-precompile.md) | Release with `cli/gh-extension-precompile` and an explicit build script | 2026-08-28 | Accepted |
| [0007](adr/0007-command-name-from-repo-name.md) | Accept `gh cli-extension` as the command name | 2026-08-28 | Accepted |
| [0008](adr/0008-go-version-pinned-in-go-mod.md) | Pin the Go toolchain in `go.mod` and let `GOTOOLCHAIN` fetch it | 2026-08-28 | Accepted |
| [0009](adr/0009-inject-dependencies.md) | Inject dependencies through a `Deps` struct | 2026-08-28 | Accepted |
| [0010](adr/0010-vault-official-sdk.md) | Use `hashicorp/vault/api` when the extension talks to Vault | 2026-08-28 | Accepted (not yet applied) |
| [0011](adr/0011-doctor-scope.md) | `doctor` reports Vault readiness without contacting Vault | 2026-08-28 | Accepted |
| [0012](adr/0012-doctor-json-output.md) | `doctor --json` emits identity attributes as a stable contract | 2026-08-28 | Accepted |

## Smaller decisions, no ADR

Recorded because the reasoning is not visible in the code, not because the
decision was hard.

**2026-08-28 — `make` as the task runner.** Every task has one name that works
the same locally and in CI (`make check` is literally what `ci.yml` runs). No
new tooling to install. `make help` lists the targets from the `## ` comments.

**2026-08-28 — `lint` is `gofmt -l` plus `go vet`, nothing else.** No
golangci-lint yet. Starting with the standard tools means zero configuration to
argue about and no lint debt on day one. See the open decisions below.

**2026-08-28 — Nine release platforms.** darwin, linux and windows across
amd64/arm64, plus linux/386, linux/arm and windows/386. This is the list
`cli/gh-extension-precompile` builds by default for Go extensions; matching it
means nobody's platform silently disappears.

**2026-08-28 — CI verifies `go mod tidy` produces no diff.** An untidy
`go.mod`/`go.sum` is how a dependency change sneaks in unreviewed.

**2026-08-28 — `.vscode/settings.json` and `extensions.json` are committed.**
They turn on Copilot's instruction files for the workspace and enable
format-on-save with `gofmt`. Everything else under `.vscode/` is gitignored, so
personal settings stay personal.

**2026-08-28 — Table-driven tests with `t.Run` subtests are the default style.**
Consistent with the standard library, and a new case is one line.

**2026-08-28 — The Claude permission allowlist splits by blast radius.**
Read-only and build commands (`go build/test/vet`, `gofmt`, `make`, read-only
`gh` and `git`) run without a prompt. Anything that publishes or mutates state
outside the working tree (`git push`, `git tag`, `gh release create`,
`gh pr create`, `gh extension install`) stays in `ask`. `go clean -modcache` is
denied outright — it wipes a machine-wide cache and is never worth an agent
doing unprompted.

**2026-08-28 — Dependabot runs weekly with Go updates grouped into one PR.**
A single dependency PR per week rather than one per module.

**2026-08-28 — Installed locally as a symlink** (`gh extension install .`)
rather than from a release. `~/.local/share/gh/extensions/gh-cli-extension`
points at the checkout, so `make build` alone updates the installed extension.

**2026-08-28 — The `doctor` report layout is pinned by a golden test.**
`TestDoctorReport` compares the whole report against an expected string. It is
deliberately brittle: column alignment and section order are the feature, so a
change to either should be a conscious edit, not a silent regression.

**2026-08-28 — Warnings do not affect `doctor`'s exit code, failures do.**
A missing `read:org` scope or an unset `VAULT_ADDR` is something to fix, not a
broken environment. Only a check that means the extension cannot work — no
authentication, an API call that errored — exits non-zero.

**2026-08-28 — `doctor` never prints the token.** `internal/gh.Auth` carries
the host, the source and a boolean, not the secret. Nothing in this extension
needs to read a token value, so nothing can leak one into a terminal, a CI log,
or a bug report.

**2026-08-28 — `doctor` gathers facts once and renders twice.** `gather()`
returns a `facts` value; the text report and the JSON payload are both derived
from it. Two renderers over one gathering pass means the human output and the
machine output can never describe different runs.

**2026-08-28 — The JSON payload never contains `null`.** Every slice is
initialised empty, so `jq '.identity.teams[]'` works on an account with no
teams. Go serialises a nil slice as `null`, so this is one forgotten
initialiser away from breaking; a test enforces it.

## Open decisions

Not decided yet. Listed so they are not forgotten rather than to be resolved
now.

**Rename the repository?** `gh cli-extension` is redundant. Renaming is cheap
today and expensive after the first published release — see
[ADR-0007](adr/0007-command-name-from-repo-name.md).

**Add a licence.** The repository has none, which by default means nobody may
use, copy, or distribute it. Any public extension needs one before its first
release.

**Adopt golangci-lint?** Worth it once there is enough code for `gofmt` and
`go vet` to stop being sufficient. The cost is a config file and an initial
backlog of findings.

**Publish and add the `gh-extension` topic?** The topic is what makes an
extension discoverable on GitHub. It should wait until the naming and licence
questions above are settled.

**A GraphQL example subcommand.** `go-gh` exposes a GraphQL client too, and the
repository exists to explore the extension API — this is the obvious next
thing to try.

**Vault commands proper.** `doctor` stops at the GitHub boundary
([ADR-0011](adr/0011-doctor-scope.md)). Probing a live Vault server and
generating policies from team membership are the next commands, and the client
choice is already settled ([ADR-0010](adr/0010-vault-official-sdk.md)).
