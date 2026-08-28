# gh-cli-extension

A [GitHub CLI](https://cli.github.com) extension, written in Go, used as a
playground for exploring what `gh` extensions can do.

The repository name has to start with `gh-`, so the extension is invoked as
`gh cli-extension`.

## Install

```sh
gh extension install 0xHackerSpace/gh-cli-extension
```

Or from a local checkout:

```sh
make install
```

## Usage

```sh
gh cli-extension <command>
```

| Command | Description |
| --- | --- |
| `vault` | Show the Vault resources your token can reach: server, token, visible mounts. Subcommands `token`, `mounts`, `can <path>`. |
| `doctor` | Check the GitHub identity attributes Vault's GitHub auth method consumes, and whether this shell is pointed at a Vault server. |
| `whoami` | Print the authenticated GitHub user (REST call through go-gh). |
| `repo` | Print the repository resolved from the current directory. |
| `version` | Print the extension version. |

### `vault`

Answers "what can I actually get at?" for the Vault token you are using.

```
$ gh cli-extension vault
Vault  https://vault.example.com:8200
       v1.17.2, unsealed (vault-prod)

Token
  display name   github-IanOliv
  policies       default, platform-ro
  ttl            767h (renewable)
  entity         8f2a1c04

Mounts (3 visible)
  database/  database  read
  kv/        kv-v2     read, list
  platform/  kv-v2     read
```

Three subcommands drill in, and every one accepts `--json`:

```sh
gh cli-extension vault token              # policies and lifetime
gh cli-extension vault mounts             # visible secret engines
gh cli-extension vault can kv/data/prod/db  # capabilities at one path
```

```
$ gh cli-extension vault can kv/data/prod/api
kv/data/prod/api
  create   no
  read     no
  update   no
  patch    no
  delete   no
  list     no

  also: deny
```

`can` resolves Vault's precedence rules rather than dumping the raw list:
`deny` overrides everything, `root` permits everything. The raw capabilities
are still in the `--json` output so you can see why.

The capabilities shown beside each mount are for the **mount path**. For a KV v2
engine the secrets live under `<mount>data/<path>` and may differ — use
`vault can` for the precise answer.

#### Configuration and authentication

Standard `VAULT_*` environment variables apply (`VAULT_ADDR`,
`VAULT_NAMESPACE`, the TLS settings), exactly as for the `vault` CLI.

The Vault token is resolved in the same order the `vault` CLI uses, with one
addition:

1. `VAULT_TOKEN`
2. `~/.vault-token`
3. a login through Vault's GitHub auth method, using your `gh` credentials

Step 3 is what ties this extension together: if you are authenticated with
`gh`, `vault` works with no Vault login at all. That token lives only for the
invocation and **is never written to disk**, so nothing is left behind — at the
cost of a fresh login, and a new accessor in Vault, on every run. Run
`vault login` yourself if you would rather reuse one token.

Use `--auth-path` if the GitHub auth method is not mounted at `auth/github`.

Logging in creates a real credential in Vault; everything else the command does
is read-only. Secret *values* are never read or printed.

### `doctor`

HashiCorp Vault's GitHub auth method authenticates a user with a GitHub token
and maps their organisation and team membership to policies. That only works if
the token carries the `read:org` scope and the user is in a mapped org and team
— none of which is visible until a login fails. `doctor` reports all of it:

```
$ gh cli-extension doctor
GitHub
  ok   gh CLI            2.4.0
  ok   authentication    github.com (keyring)
  ok   identity          @IanOliv (id 12345678)
  ok   token scopes      repo, read:org, gist

Vault readiness
  ok   read:org scope    present
       (required by Vault's GitHub auth method)
  ok   org membership    0xHackerSpace
  ok   team membership   0xHackerSpace/platform
  warn VAULT_ADDR        not set
       (export VAULT_ADDR=https://vault.example.com:8200)

7 passed, 1 warning
```

It is read-only and **never contacts a Vault server** — `VAULT_ADDR` is read
from the environment and reported, nothing more. So it works before Vault is
provisioned, which is when it helps most. It also never prints your token.

Failures exit non-zero; warnings do not.

#### Machine-readable output

`--json` emits the identity attributes for other tooling to consume, instead of
the rendered report:

```sh
# every team, as Vault's GitHub auth method sees them
gh cli-extension doctor --json | jq -r '.identity.teams[] | .org + "/" + .slug'

# gate a script on the scope Vault needs
gh cli-extension doctor --json | jq -e '.vault.readOrgScope' >/dev/null
```

```json
{
  "ghVersion": "2.4.0",
  "identity": {
    "host": "github.com",
    "authenticated": true,
    "tokenSource": "keyring",
    "login": "IanOliv",
    "id": 12345678,
    "scopes": ["repo", "read:org", "gist"],
    "orgs": ["0xHackerSpace"],
    "teams": [{ "org": "0xHackerSpace", "slug": "platform" }]
  },
  "vault": { "addr": "", "readOrgScope": true },
  "checks": [{ "section": "GitHub", "status": "ok", "name": "gh CLI", "detail": "2.4.0" }],
  "summary": { "passed": 7, "warnings": 1, "failed": 0 }
}
```

The exit code is the same in both modes, and the document stays valid and
complete when checks fail — so a CI job can read both the payload and the
status. Empty collections are always `[]`, never `null`.

This payload is a **public contract**: fields may be added, but renaming or
removing one is a breaking change. See
[ADR-0012](docs/adr/0012-doctor-json-output.md).

Note that unlike `gh` core, `--json` takes no field list; pipe to `jq` to
filter.

## Development

Requires the Go toolchain pinned in `go.mod` (Go 1.25). With the default
`GOTOOLCHAIN=auto`, an older local Go downloads the right version on first
build.

```sh
make build   # compile ./gh-cli-extension
make test    # go test ./...
make lint    # gofmt check + go vet
make check   # lint + test — the same gate CI runs
make help    # list all targets
```

Layout:

```
main.go                  entrypoint; maps errors to exit codes
cmd/                     cobra command tree, one file per subcommand
internal/gh/             thin go-gh wrapper + the interfaces commands depend on
internal/vault/          thin hashicorp/vault/api wrapper, same pattern
script/build.sh          cross-compiles release binaries into ./dist
```

Conventions for new code — including the checklist for adding a subcommand —
live in [`AGENTS.md`](AGENTS.md).

## Releasing

Push a `v*` tag:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

`.github/workflows/release.yml` runs
[`cli/gh-extension-precompile`](https://github.com/cli/gh-extension-precompile),
which calls `script/build.sh` and attaches the per-platform binaries to the
GitHub release. `gh extension install` then downloads the matching one.

To make the extension discoverable, add the `gh-extension` topic to the
repository:

```sh
gh repo edit 0xHackerSpace/gh-cli-extension --add-topic gh-extension
```

## Documentation

- [`docs/memory.md`](docs/memory.md) — what this repo is for, current state,
  environment quirks and gotchas. Start here.
- [`docs/decisions.md`](docs/decisions.md) — the full decision log, including
  the questions still open.
- [`docs/adr/`](docs/adr/) — Architecture Decision Records.
- [`AGENTS.md`](AGENTS.md) — coding conventions.

## AI coding assistants

Both assistants read the same conventions:

- **Claude Code** — [`CLAUDE.md`](CLAUDE.md) imports `AGENTS.md`;
  `.claude/settings.json` allow-lists the safe Go/`gh` commands and
  `.claude/commands/` provides `/check`, `/newcmd` and `/release`.
- **GitHub Copilot** — [`.github/copilot-instructions.md`](.github/copilot-instructions.md)
  plus path-scoped rules in `.github/instructions/`.

Edit `AGENTS.md` when a convention changes; the other files point at it.
