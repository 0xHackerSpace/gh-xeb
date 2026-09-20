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
| [0007](adr/0007-command-name-from-repo-name.md) | Accept `gh xeb` as the command name | 2026-08-28 | Accepted |
| [0008](adr/0008-go-version-pinned-in-go-mod.md) | Pin the Go toolchain in `go.mod` and let `GOTOOLCHAIN` fetch it | 2026-08-28 | Accepted |
| [0009](adr/0009-inject-dependencies.md) | Inject dependencies through a `Deps` struct | 2026-08-28 | Accepted |
| [0010](adr/0010-vault-official-sdk.md) | Use `hashicorp/vault/api` when the extension talks to Vault | 2026-08-28 | Accepted (applied by 0014) |
| [0011](adr/0011-doctor-scope.md) | `doctor` reports Vault readiness without contacting Vault | 2026-08-28 | Accepted |
| [0012](adr/0012-doctor-json-output.md) | `doctor --json` emits identity attributes as a stable contract | 2026-08-28 | Accepted |
| [0013](adr/0013-cmd-at-repo-root.md) | Move the command tree from `internal/cmd` to `cmd` | 2026-08-28 | Accepted |
| [0014](adr/0014-vault-command.md) | `vault` reports the resources a token can reach | 2026-08-28 | Accepted (extended by 0017) |
| [0015](adr/0015-vault-in-memory-login.md) | Log in to Vault in memory, never writing a token to disk | 2026-08-28 | Accepted |
| [0016](adr/0016-backstage-catalog-command.md) | `backstage` reads the software catalog through a hand-written client | 2026-09-01 | Accepted (amended by 0020) |
| [0017](adr/0017-vault-get-masked-by-default.md) | `vault get` reads secret values but masks them by default | 2026-09-01 | Accepted |
| [0018](adr/0018-backstage-config-from-vault.md) | `backstage` can take its address and token from a Vault secret | 2026-09-01 | Accepted |
| [0019](adr/0019-backstage-ofertas.md) | `backstage ofertas` reshapes Templates into an offer catalogue | 2026-09-01 | Accepted (extended by 0020) |
| [0020](adr/0020-backstage-create.md) | `backstage create` runs a template, and validates before it does | 2026-09-01 | Accepted |
| [0021](adr/0021-rename-to-xeb.md) | Rename the extension to `gh xeb` | 2026-09-01 | Accepted |
| [0022](adr/0022-harness-install.md) | `harness install` generates the agent skill from the live command tree | 2026-09-01 | Accepted |
| [0023](adr/0023-mcp-server-command.md) | `mcp` subcommand exposes the CLI as an MCP server via stdio | 2026-09-20 | Proposed |

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
rather than from a release. `~/.local/share/gh/extensions/gh-xeb`
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

**2026-08-28 — `internal/vault` is tested against a stand-in HTTP server, not
only a fake client.** The command tests use a fake `vault.Client`, which
bypasses every `map[string]interface{}` assertion in the decoding layer — the
part most likely to break. `server_test.go` drives the real SDK against an
httptest server so that decoding is actually exercised.

**2026-08-28 — `gh.Token()` is the only place the GitHub secret is exposed.**
It exists for one caller, the Vault login. Everything that prints uses
`gh.CurrentAuth`, which carries the host, the source and a boolean and
structurally cannot leak the token.

**2026-08-28 — Errors carry no advice where the caller knows better.**
`gh.Token()` returns "no GitHub token for github.com" and lets the Vault
command say what to do about it, rather than both appending "run: gh auth
login" and printing it twice.

**2026-09-01 — `encodeJSON` in `cmd/json.go` is shared; `doctor` keeps its
own.** Every `--json` flag except `doctor`'s goes through one indented encoder.
`doctor` renders a fixed document that two golden tests pin as a public
contract, so it stays separate rather than being generalised into the shared
path.

**2026-09-01 — `backstage entities` treats an unset flag as absent, not as
"field exists".** `Filter.Add(key)` with no values is a real catalog filter
meaning the field is present, so passing an empty flag slice straight through
would have made a bare `backstage entities` ask for entities that have a kind
*and* a type *and* an owner. A test caught it; `addValues` now skips empty
flags.

**2026-09-01 — `sys/health` is read at the root namespace, everything else is
namespaced.** That endpoint exists only at the root, so sending
`VAULT_NAMESPACE` with it asks for `<namespace>/sys/health` and gets a 404
"unsupported path". The namespace is stripped for that one call via
`api.WithNamespace("")`. It surfaced on HCP Vault, where the variable is always
set, and is invisible on a dev server, where it never is.

**2026-09-01 — A scaffolder task's rendered output is only in its completion
event.** `GET /tasks/{id}` returns `spec.output` with the template's `${{ }}`
placeholders *unrendered*; printing it would show a user the template source
instead of their new repository's URL. The real values arrive in the final
event from `/tasks/{id}/events`. Found by probing a live instance.

**2026-09-01 — `repoUrl` is not a URL.** Backstage's `RepoUrlPicker` encodes it
as `github.com?owner=acme&repo=payments`. Every first attempt at
`backstage create` gets this wrong, so it is in the help text and the README.

**2026-09-01 — The ADRs keep saying `cli-extension` after the rename.** They
are historical records; an ADR written in August describing what the command
was then is not wrong. Only ADR-0007's status line changed
([ADR-0021](adr/0021-rename-to-xeb.md)). A grep for the old name now hits
`docs/adr/` and nothing else, which is the right answer to "what was this
called before?"

**2026-09-01 — Agent skill roots were read off a machine, not off docs.**
`~/.claude/skills/` and `~/.copilot/skills/` both existed with a `tfctl` skill
in them, both using the same `SKILL.md` frontmatter. That is why
[ADR-0022](adr/0022-harness-install.md) treats agents as a table of paths
rather than one renderer each. `.github/skills/` for Copilot project scope is
the one path that was inferred rather than observed.

## Open decisions

Not decided yet. Listed so they are not forgotten rather than to be resolved
now.

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

**Browsing a KV mount.** `vault ls <path>` to walk a mount is still the missing
half. Reading a value was the other half and is now decided
([ADR-0017](adr/0017-vault-get-masked-by-default.md)); walking is what remains,
and it is where the cost shows: one request per directory plus one per secret,
all of it in the audit log. `sys/capabilities-self` takes several paths per
request and is the way to keep that cheap.

**Writing a secret.** The first mutating operation on a secret engine, and the
one ADR-0017 names as its own revisit trigger. It needs an answer for where the
value comes from, since it must not be a command-line argument.

**Machine-checking the rules in the generated skill.** The command list in the
skill cannot drift — a test walks the real tree. The rules beside it are a
string constant and can. `TestBackstageHasNoTokenFlag` is the shape of the fix,
applied to one rule; the rest are unguarded
([ADR-0022](adr/0022-harness-install.md)).

**A `backstage task <id>` command.** To follow or inspect a run someone else
started, and the place scaffolder secrets would have to be solved
([ADR-0020](adr/0020-backstage-create.md)). Today a task can only be watched by
the invocation that created it.

**Taking `create` values from a file** rather than repeated `--field`. Better
once a template has a dozen parameters; worse for the two-or-three case the
command is for today.

**Rendering `spec.parameters` in `backstage get`.** `ofertas` now renders them
for every template ([ADR-0019](adr/0019-backstage-ofertas.md)); `get` still
renders them for none, which is an inconsistency a user will hit. Fixing it
means a Template-shaped special case in a command that serves all kinds.

**Preserving the field order a template's form actually uses.** `ofertas`
sorts required-first then alphabetically because `Entity.Spec` is a decoded
map and the schema's own order is gone by then. Recovering it means keeping
`spec` as `json.RawMessage` in `internal/backstage` and decoding with an
order-preserving reader — a change to the client, for a cosmetic gain.

**Should `doctor` know about Backstage?** It reports GitHub and Vault
readiness. The catalog is now a third service the extension can be configured
for, and `BACKSTAGE_BASE_URL` being unset is exactly the kind of thing `doctor`
exists to notice — but [ADR-0011](adr/0011-doctor-scope.md) drew the line at
not contacting anything, and it is worth deciding deliberately rather than
drifting.

**Generating Vault policies from GitHub team membership.** The original goal
behind the extension, and the one part still unbuilt. `doctor` surfaces the
teams and `vault` shows what a token resolves to; turning that into a policy is
a write operation and needs its own design.
