# ADR-0016: `backstage` reads the software catalog through a hand-written client

- **Status:** Accepted (amended by [ADR-0020](0020-backstage-create.md), which added the first write)
- **Date:** 2026-09-01
- **Deciders:** @IanOliv
- **Departs from:** [ADR-0010](0010-vault-official-sdk.md) on the specific
  point of using a vendor SDK — for the reason given below.

## Context

The extension exists to connect GitHub identity to the systems around it.
[ADR-0014](0014-vault-command.md) covered Vault. Backstage is the other half of
that picture: it is where an organisation records what a repository *is* — its
owner, its lifecycle, its dependencies — and a `gh` extension is standing in a
repository already, which is exactly the context needed to ask "what does the
catalog say about this?"

Two facts shaped the decision:

- Backstage has **no official Go client**. It is a TypeScript project; the
  packages it publishes are for plugin authors, not for external callers. The
  Software Catalog is a plain JSON API over HTTP with no protocol of its own.
- The catalog API is **read-mostly by design**. Entities are not created
  through it; they are registered by committing a `catalog-info.yaml` and
  letting Backstage's discovery find it. Writing is a git operation.

## Decision

Add a `backstage` command backed by `internal/backstage`, a client written here
against `net/http` rather than a vendored SDK.

| Command | Answers |
| --- | --- |
| `backstage` | Which catalog am I talking to, and what is in it? |
| `backstage entities` | Which entities match these filters? |
| `backstage get <ref>` | Everything about one entity, with its relations. |
| `backstage repo` | Which entities describe the repository I am in? |

All of them accept `--json`, following [ADR-0012](0012-doctor-json-output.md).
The client sits behind a narrow `Client` interface reached through `Deps`, as
[ADR-0003](0003-internal-gh-wrapper.md) and
[ADR-0009](0009-inject-dependencies.md) require.

Three constraints are part of the decision, not incidental:

- **Read-only.** No subcommand writes to the catalog, because the catalog is
  not where entities are authored.
- **The token comes only from `BACKSTAGE_TOKEN`.** There is no `--token` flag.
  A secret passed as an argument is visible in shell history and in `ps` to
  every other user on the machine. This is the same rule as
  [ADR-0015](0015-vault-in-memory-login.md) applied to a different credential,
  and a test asserts the flag has not reappeared.
- **`backstage repo` matches on the `github.com/project-slug` annotation.**
  That annotation is the only durable link from a git remote back to a catalog
  entry.

## Consequences

- **No new dependencies.** The module graph is unchanged: `net/http` and
  `encoding/json` are enough. Contrast ADR-0010, where taking the SDK cost 40
  modules. That asymmetry is the whole argument — there was an SDK worth paying
  for there, and there is nothing to pay for here.
- **We now own the API surface.** Pagination cursors, the `filter` grammar, the
  error envelope and the `by-query` parameters are our code, and a Backstage
  release that changes them breaks us silently rather than at compile time.
  `server_test.go` drives the client against an httptest catalog so at least
  the encoding is pinned to something.
- **`Entity.Spec` is `map[string]interface{}`.** Its shape depends on `Kind`,
  and custom kinds are normal. Typed accessors cover the common fields; anything
  else is a map lookup with no compiler help. Modelling every kind would have
  meant rejecting entities we do not recognise, which is worse.
- **`backstage repo` returns nothing for a correctly-catalogued repository**
  whose entity was registered without the annotation. The output says which
  annotation it looked for rather than claiming the repository is unregistered.
- **`--all` can be many requests** against a large catalog. The default is one
  page and the footer says the list was truncated; `AllEntities` stops at 200
  pages rather than looping.
- **The catalog is a third external service** after GitHub and Vault. `doctor`
  does not know about it yet, so there is no single command that reports
  whether the whole toolchain is configured.
- `writeVaultJSON` became `encodeJSON` in `cmd/json.go`, since two command
  families now share it. `doctor` keeps its own writer: its payload is a pinned
  contract with a fixed shape.

## Alternatives considered

- **Vendor a community Go client.** The ones that exist are thin, unmaintained
  wrappers over the same three endpoints. Taking a dependency to avoid writing
  200 lines, and inheriting its abandonment, was the worse trade.
- **Shell out to a Backstage CLI.** There is no such tool for querying a remote
  catalog; `backstage-cli` is for plugin development.
- **A `--token` flag "for convenience".** Rejected on the security grounds
  above. Users who want it per-invocation can write
  `BACKSTAGE_TOKEN=... gh cli-extension backstage`, which keeps the value out
  of history in most shells and out of `ps` in all of them.
- **Finding the repository's entity by `backstage.io/source-location`.** That
  annotation holds a URL with variable shapes — `url:https://github.com/o/r`,
  with and without a trailing slash, sometimes pointing at a subdirectory —
  so matching it means guessing. `project-slug` is exactly `owner/name`.
- **Folding this into `doctor`.** `doctor` must keep working with nothing
  configured ([ADR-0011](0011-doctor-scope.md)); reaching a catalog is the
  opposite of that.

## Revisit when

An official Go client for the catalog appears, or the catalog needs an
authentication method other than a bearer token — a service-to-service token
with its own exchange, or OIDC — at which point hand-rolling stops being
cheaper than a dependency.
