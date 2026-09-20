# gh-xeb

A [GitHub CLI](https://cli.github.com) extension, written in Go, used as a
playground for exploring what `gh` extensions can do.

The repository name has to start with `gh-`, so the extension is invoked as
`gh xeb`.

## Install

```sh
gh extension install 0xHackerSpace/gh-xeb
```

Or from a local checkout:

```sh
make install
```

## Usage

```sh
gh xeb <command>
```

| Command | Description |
| --- | --- |
| `vault` | Show the Vault resources your token can reach: server, token, visible mounts. Subcommands `token`, `mounts`, `can <path>`, `get <path>`. |
| `backstage` | Query a Backstage software catalog and run its templates. Subcommands `entities`, `ofertas`, `create`, `get <ref>`, `repo`. |
| `harness` | Teach a coding agent how to use this extension. Subcommand `install <agent>`. |
| `mcp` | Run an MCP server exposing all CLI commands as discoverable tools via stdio. |
| `doctor` | Check the GitHub identity attributes Vault's GitHub auth method consumes, and whether this shell is pointed at a Vault server. |
| `whoami` | Print the authenticated GitHub user (REST call through go-gh). |
| `repo` | Print the repository resolved from the current directory. |
| `version` | Print the extension version. |

### `vault`

Answers "what can I actually get at?" for the Vault token you are using.

```
$ gh xeb vault
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
gh xeb vault token              # policies and lifetime
gh xeb vault mounts             # visible secret engines
gh xeb vault can kv/data/prod/db  # capabilities at one path
```

```
$ gh xeb vault can kv/data/prod/api
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
is read-only. `vault get` reads a secret's fields but masks the values unless
you ask for them with `--reveal` or `--field`.

### `backstage`

Reads a [Backstage](https://backstage.io) Software Catalog: what exists, who
owns it, what it depends on, and which entity describes the repository you are
standing in.

```
$ gh xeb backstage
Backstage  https://backstage.acme.dev

Catalog (434 entities)
  User        190
  Component   128
  API         54
  Resource    31
  Group       22
  System      9
```

`entities` narrows the catalog down. Repeating a flag means *or*; different
flags mean *and*.

```
$ gh xeb backstage entities --kind component --limit 4
component:default/payments       service   production     group:default/payments
component:default/payments-web   website   production     group:default/payments
component:default/ledger         service   experimental   group:default/platform
component:default/docs-site      website   -              -

Showing 4 of 41; --all for the rest
```

`--filter` takes the catalog's own syntax for anything the flags do not cover.
A bare key with no `=` matches entities where the field merely exists.

```sh
gh xeb backstage entities --filter 'relations.ownedBy=group:default/platform'
gh xeb backstage entities --filter 'metadata.annotations.backstage.io/techdocs-ref'
```

`ofertas` answers a different question: not "what is in the catalog" but "what
can I ask the portal to create, and what will it need from me?" It lists every
`Template` with the inputs each one asks for, flattened out of its
`spec.parameters`.

```
$ gh xeb backstage ofertas
node-typescript-api  Node.js + TypeScript API
  Serviço HTTP em Node.js com TypeScript e Express, já com testes (vitest), lint, Dockerfile multi-stage, GitHub Actions e TechDocs.

  * name          Nome único do componente (kebab-case)
  * owner         Time responsável pelo serviço
  * system        System ao qual o serviço pertence
    description   O que este serviço faz
    nodeVersion   Versão do Node
    port          Porta HTTP
  * repoUrl       Localização do repositório

terraform-module  Módulo Terraform
  Módulo Terraform reutilizável com main/variables/outputs, exemplo executável, validação de fmt/validate/tflint no CI e registro como Resource no catálogo.

  * name               Sem o prefixo "terraform-" (ex.- "s3-bucket")
  * owner              Owner
    description        O que este módulo provisiona
    system             System ao qual a infraestrutura pertence
  * provider           Provider principal
    terraformVersion   Versão mínima do Terraform
  * repoUrl            Localização do repositório

* required
```

`--json` gives the structure, with `fields` mapped out of the parameter schema:

```json
{
  "count": 4,
  "templates": [
    {
      "name": "terraform-module",
      "title": "Módulo Terraform",
      "description": "Módulo Terraform reutilizável com main/variables/outputs, ...",
      "fields": [
        { "name": "name",     "description": "Sem o prefixo \"terraform-\"", "required": true },
        { "name": "owner",    "description": "Owner",                        "required": true },
        { "name": "provider", "description": "Provider principal",           "required": true },
        { "name": "repoUrl",  "description": "Localização do repositório",   "required": true },
        { "name": "description",      "description": "O que este módulo provisiona",    "required": false },
        { "name": "system",           "description": "System ao qual a infra pertence", "required": false },
        { "name": "terraformVersion", "description": "Versão mínima do Terraform",      "required": false }
      ]
    }
  ]
}
```

A field with no `description` in the schema falls back to its form label. Order
is the order of the form pages; within a page, required fields come first and
then alphabetical — the schema's own order inside a page is not recoverable
once the entity is decoded ([ADR-0019](docs/adr/0019-backstage-ofertas.md)).
Aliases: `offerings`, `templates`.

#### Running a template

`create` submits a template to the scaffolder. This is the one command in the
tree that changes something outside your machine: a successful run creates a
repository and registers it in the catalog, and interrupting the command does
not undo any of it.

```sh
gh xeb backstage create terraform-module   --field name=s3-bucket   --field owner=group:default/guests   --field provider=aws   --field 'repoUrl=github.com?owner=acme&repo=terraform-s3-bucket'
```

Every value is checked against the template's own schema **before** anything is
sent, so a mistake costs you nothing:

```
$ gh xeb backstage create terraform-module --field nome=x
gh xeb: terraform-module has no parameter "nome" (it takes: description, name, owner, provider, repoUrl, system, terraformVersion)

$ gh xeb backstage create terraform-module --field name=x
gh xeb: terraform-module requires "owner", "provider", "repoUrl"
```

`--dry-run` goes one step further and shows exactly what would be submitted:

```
$ gh xeb backstage create terraform-module --field ... --dry-run
template:default/terraform-module

Would submit:
  name       s3-bucket
  owner      group:default/guests
  provider   aws
  repoUrl    github.com?owner=acme&repo=terraform-s3-bucket

Nothing was sent. Drop --dry-run to run it.
```

Two things that catch people out:

- **`repoUrl` is not a URL.** Backstage's repository picker encodes it as
  `github.com?owner=acme&repo=payments`. Quote it — the `&` is a shell
  operator.
- **Types come from the schema.** `--field port=8080` is sent as a number and
  `--field includeAdr=true` as a boolean, because that is what the template
  declares. A parameter expecting an array or an object is refused: run those
  from the portal.

By default the run is followed and its log printed, ending in the template's
output links; the command exits non-zero if the run fails.

| Flag | |
| --- | --- |
| `--dry-run` | Validate and print what would be sent, without sending it |
| `--no-wait` | Submit and print the task id |
| `--timeout` | How long to follow the run, default 10m. The task outlives it |
| `--json` | `id`, `status`, `url`, `log`, `output` |

Scaffolder secrets are not supported: they would mean accepting a credential on
the command line ([ADR-0020](docs/adr/0020-backstage-create.md)).

#### Everything else

`get` takes a reference — `[<kind>:][<namespace>/]<name>`, namespace defaulting
to `default`.

```
$ gh xeb backstage get component:payments
component:default/payments  Payments API

  kind          Component
  namespace     default
  type          service
  lifecycle     production
  owner         group:default/payments
  system        system:default/billing
  description   Charges cards and reconciles settlements
  tags          go, tier-1, pci
  source        url:https://github.com/acme/payments/tree/main/
  repository    acme/payments

Relations
  dependsOn      resource:default/payments-db
                 component:default/ledger
  ownedBy        group:default/payments
  partOf         system:default/billing
  providesApis   api:default/payments-v2
```

`repo` resolves the current repository the same way `gh` does and finds the
entities annotated with it:

```
$ gh xeb backstage repo
0xHackerSpace/gh-xeb

component:default/gh-xeb   tool   experimental   group:default/platform
```

The link is the `github.com/project-slug` annotation, which Backstage's GitHub
integrations write when they discover a `catalog-info.yaml`. An entity
registered without it will not be found even though it describes this
repository — the command says so rather than claiming the repository is
unregistered:

```
$ gh xeb backstage repo
0xHackerSpace/gh-xeb

Not in the catalog: no entity is annotated github.com/project-slug=0xHackerSpace/gh-xeb
```

#### Configuration and authentication

| Variable | Meaning |
| --- | --- |
| `BACKSTAGE_BASE_URL` | Backstage app address, e.g. `https://backstage.example.com`. `--url` overrides it. |
| `BACKSTAGE_URL` | Fallback if the above is unset. |
| `BACKSTAGE_TOKEN` | Bearer token. Optional — catalogs that allow anonymous reads work without one. |

Give the app's address, not the API path: `/api/catalog` is appended per
request. A bare host is assumed to be `https`.

##### Taking both out of the environment, with Vault

`--vault-secret` reads the address and the token from a Vault KV secret
instead, so neither has to live in your shell:

```sh
gh xeb backstage --vault-secret secret/backstage
```

The secret should carry a `url` field, a `token` field, or both — `base_url`
and `api_token` are accepted too:

```sh
vault kv put secret/backstage   url=https://backstage.example.com   token=<the catalog token>
```

Vault is reached exactly as the `vault` command reaches it: `VAULT_ADDR`,
`VAULT_TOKEN`, `~/.vault-token`, and failing those a login through Vault's
GitHub auth method with your `gh` credentials. So being logged in to `gh` can
be enough to query a catalog you hold no local credential for. Use
`--vault-auth-path` if that method is not mounted at `auth/github`.

Set `BACKSTAGE_VAULT_SECRET` to skip the flag. Where each value comes from,
first match winning:

| | Order |
| --- | --- |
| url | `--url`, then the Vault secret, then `BACKSTAGE_BASE_URL`, `BACKSTAGE_URL` |
| token | the Vault secret, then `BACKSTAGE_TOKEN` |

The token goes straight from Vault into the catalog request. It is never
printed, never logged, and never written to disk. A secret carrying neither
field is an error that names the fields it *does* carry — field names only,
never values.

There is deliberately **no `--token` flag**. A secret passed as a command-line
argument is visible in your shell history and in `ps` to every other user on
the machine. For a one-off, prefix the invocation instead:

```sh
BACKSTAGE_TOKEN=... gh xeb backstage repo
```

Every subcommand is read-only. Entities are not created through this API —
they are registered by committing a `catalog-info.yaml` and letting Backstage's
discovery find it.

### `harness`

Coding agents are how this extension mostly gets driven, and an agent that does
not know it exists will reach for `vault` and `curl` instead. `harness install`
writes a skill file into the place an agent looks for one.

```sh
gh xeb harness install claude            # this project only
gh xeb harness install copilot --global  # everywhere
```

| agent | project | `--global` |
| --- | --- | --- |
| `claude` | `.claude/skills/xeb/SKILL.md` | `~/.claude/skills/xeb/SKILL.md` |
| `copilot` | `.github/skills/xeb/SKILL.md` | `~/.copilot/skills/xeb/SKILL.md` |

```
$ gh xeb harness install claude
Wrote /home/you/work/api/.claude/skills/xeb/SKILL.md
  Project scope: commit it to share with the repository.
```

The command list inside the skill is **generated from the live command tree**,
so it cannot drift from the binary that wrote it — adding a subcommand and
reinstalling is the whole update procedure. Alongside it are the things an
agent cannot infer from `--help`: that tokens never go on the command line,
that `backstage create` is the only command which changes anything outside the
machine, and that secret values are masked deliberately rather than by
accident.

An existing file is never replaced without `--force`. `--dry-run` prints the
destination and the document without writing; `--path` writes somewhere else
entirely, for when an agent moves the goalposts
([ADR-0022](docs/adr/0022-harness-install.md)).

### `doctor`

HashiCorp Vault's GitHub auth method authenticates a user with a GitHub token
and maps their organisation and team membership to policies. That only works if
the token carries the `read:org` scope and the user is in a mapped org and team
— none of which is visible until a login fails. `doctor` reports all of it:

```
$ gh xeb doctor
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
gh xeb doctor --json | jq -r '.identity.teams[] | .org + "/" + .slug'

# gate a script on the scope Vault needs
gh xeb doctor --json | jq -e '.vault.readOrgScope' >/dev/null
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
make build   # compile ./gh-xeb
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
gh repo edit 0xHackerSpace/gh-xeb --add-topic gh-extension
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
