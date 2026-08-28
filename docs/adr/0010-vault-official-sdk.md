# ADR-0010: Use `hashicorp/vault/api` when the extension talks to Vault

- **Status:** Accepted — applied on 2026-08-28 by
  [ADR-0014](0014-vault-command.md); `internal/vault` now depends on the SDK.
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

The purpose of this extension is to surface GitHub identity attributes — user,
organisations, teams, token scopes — for use with HashiCorp Vault, whose GitHub
auth method maps exactly those attributes to policies.

`doctor` ([ADR-0011](0011-doctor-scope.md)) deliberately stops at the boundary:
it reads the GitHub side and checks `VAULT_ADDR` from the environment, but does
not contact a Vault server. Commands that do contact Vault — probing a server,
reading the auth mount configuration, creating policies — are expected next, so
the client choice was settled now rather than under time pressure later.

Three options were on the table: the official `hashicorp/vault/api` SDK, hand-
rolled `net/http` calls against Vault's REST API, or shelling out to the `vault`
binary.

## Decision

Use the official `hashicorp/vault/api` SDK when Vault communication is
implemented. Add the dependency at that point, not before — an unused
dependency would be stripped by `go mod tidy` anyway.

## Consequences

- Token renewal, namespaces, retries, `VAULT_*` environment handling, and TLS
  configuration come from the SDK instead of being reimplemented, and they
  behave the way Vault users expect.
- Writing policies is where this pays off: a typed client is much safer than
  hand-built JSON against `sys/policies/acl/:name`.
- The dependency tree grows substantially — roughly forty indirect modules
  (`go-retryablehttp`, `hcl`, `go-plugin`, `go-sockaddr`, `mapstructure`, and
  friends). The binary goes from about 7 MB to about 20 MB.
- That growth is in tension with [ADR-0001](0001-precompiled-go-extension.md),
  which values a small single static binary. The trade was made knowingly: the
  binary is still a single static file with no runtime dependency, only a
  larger one, and nine platforms of it are downloaded per release.
- More dependencies means a wider supply-chain surface and more Dependabot
  traffic.

## Alternatives considered

- **Plain `net/http`.** Around sixty lines for the handful of read endpoints
  `doctor` would need, and zero new dependencies. Rejected: it stops scaling
  the moment policy writes, namespaces, or token renewal enter the picture, and
  reimplementing Vault's environment-variable and TLS conventions is exactly
  the kind of subtle work that goes wrong quietly.
- **Shell out to the `vault` CLI.** Reuses the user's existing login, but
  requires the binary on PATH (absent in CI and slim containers) and means
  parsing its output — which `AGENTS.md` rules out in favour of a library.

## Revisit when

The Vault surface turns out to be two or three read-only endpoints and nothing
more, in which case the dependency is not paying for itself.
