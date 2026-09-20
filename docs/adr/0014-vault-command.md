# ADR-0014: `vault` reports the resources a token can reach

- **Status:** Accepted (extended by [ADR-0017](0017-vault-get-masked-by-default.md))
- **Date:** 2026-08-28
- **Deciders:** @IanOliv
- **Applies:** [ADR-0010](0010-vault-official-sdk.md), which chose the SDK
  before there was anything to use it for.

## Context

The extension's purpose is to connect GitHub identity to Vault.
[ADR-0011](0011-doctor-scope.md) built `doctor`, which stops at the GitHub
boundary and only reads `VAULT_ADDR` from the environment, and named its own
trigger: *"Revisit when Vault communication lands."* This is that.

The question a user has is "what can I actually get at?" — which Vault answers
across several endpoints: the token's own policies and lifetime, the secret
engines visible to them, and the capabilities their policies resolve to at a
given path.

## Decision

`vault` prints an overview — server, token, visible mounts — and three
subcommands drill in:

| Command | Answers |
| --- | --- |
| `vault` | Where am I connected, as whom, and what can I see? |
| `vault token` | What policies and lifetime does my token have? |
| `vault mounts` | Which secret engines are visible, with what capabilities? |
| `vault can <path>` | What may I do at exactly this path? |

All of them accept `--json`, following [ADR-0012](0012-doctor-json-output.md).

`internal/vault` wraps the SDK behind a narrow `Client` interface, as
[ADR-0003](0003-internal-gh-wrapper.md) requires, so commands are testable
without a server.

Mounts come from `sys/internal/ui/mounts`, not `sys/mounts`. The latter needs a
privileged policy; the UI endpoint is what Vault's own console calls and any
authenticated token can read it. Using `sys/mounts` would have made the command
fail for exactly the least-privileged users it is most useful to.

## Consequences

- The `hashicorp/vault/api` dependency is now real: 100 modules in the graph,
  up from 60. That cost was accepted in ADR-0010 and is now paid.
- Standard `VAULT_*` environment variables work — `VAULT_ADDR`,
  `VAULT_NAMESPACE`, the TLS settings — because the SDK reads them. This is the
  concrete payoff over hand-rolled HTTP.
- `vault can` resolves Vault's precedence rules rather than dumping the raw
  capability list: `deny` overrides everything, `root` permits everything. The
  raw list is still in the JSON so a consumer can see why.
- Capabilities shown next to each mount are for the *mount path*. For a KV v2
  engine the secrets live under `<mount>data/<path>` and may differ. The help
  text says so, and `vault can` exists for the precise answer. This is the one
  place the output could mislead.
- `doctor` and `vault` now overlap slightly: both report on Vault reachability.
  They stay separate because `doctor` must keep working with no server at all.

## Alternatives considered

- **One flat command printing everything.** Simpler, but `vault can <path>`
  takes an argument and does not fit a single report, and a utility that will
  grow wants somewhere to grow into.
- **Adding `vault ls <path>` to browse secrets.** Deferred, not rejected. It is
  the natural next command; reading a secret's *value* is a separate decision
  about putting sensitive material on a terminal.
- **`sys/mounts` for the mount list.** Rejected: privileged, and it fails for
  ordinary users.

## Revisit when

Browsing or reading secrets is wanted, which is the point where displaying
sensitive material needs its own decision.
