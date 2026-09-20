# ADR-0020: `backstage create` runs a template, and validates before it does

- **Status:** Accepted
- **Date:** 2026-09-01
- **Deciders:** @IanOliv
- **Amends:** [ADR-0016](0016-backstage-catalog-command.md), which scoped
  `internal/backstage` to reading. That is no longer true.
- **Triggered by:** [ADR-0019](0019-backstage-ofertas.md), whose *Revisit when*
  read: "Someone needs to *run* a template."

## Context

`ofertas` shows what each template asks for. The next question is obvious and
the portal already answers it in a browser: run one.

This is the first thing the extension does that changes state in a system it
does not own. `vault get` reads. `backstage entities` reads. Even the Vault
login ([ADR-0015](0015-vault-in-memory-login.md)) only mints a token for its
own use. A scaffolder task creates a repository on GitHub, registers an entity
in the catalog, and cannot be undone by interrupting the command.

Two constraints collide here:

- **AGENTS.md forbids interactive prompts.** Extensions run in pipes and in CI.
  The usual "are you sure? [y/N]" is not available.
- **[ADR-0017](0017-vault-get-masked-by-default.md) established that the safe
  thing is what happens when you type the short command.** Applied literally,
  `create` would not create.

## Decision

`backstage create <template> [--field name=value]` submits a run.

It creates. A `create` that does not create is a worse trap than one that does:
the name is the contract, and someone who works around a surprising default
learns to pass the override reflexively, which is how the safety is lost. The
protection is placed where the mistakes actually are instead.

**Every value is checked against the template's own schema before anything is
sent.** A misspelled parameter, a missing required one, or a value of the wrong
type fails locally, naming what the template does take:

```
terraform-module has no parameter "nome" (it takes: description, name, owner,
provider, repoUrl, system, terraformVersion)
```

**`--dry-run` resolves and validates the values and prints what would be sent.**
It is the rehearsal an interactive confirmation would have given, available to
a script as well as to a person.

**Values are coerced to the type the schema declares** — boolean, integer,
number — because everything arrives from a shell as a string, and the
scaffolder rejects `"true"` where it wants `true` with a message that never
mentions quoting. `array` and `object` parameters are refused outright rather
than guessed at.

The run is followed by default, its log printed as it arrives; `--no-wait`
returns the task id. A failed run exits non-zero through `SilentError`, since
its log is already on screen.

## Consequences

- **`internal/backstage` is no longer read-only.** ADR-0016 said it was, and
  that sentence is now historical. `Scaffold` is the only method that changes
  anything and says so in its doc comment.
- **The most common accident is now impossible**, and the second most common —
  a wrong value that is nonetheless well-typed — is not. Validation is
  structural, not semantic: `--field owner=group:default/typo` passes here and
  fails in the run.
- **A failed run still costs something.** The scaffolder executes steps in
  order, so a task that dies at `publish` has already run `fetch` and whatever
  came before. Interrupting the command does not stop the task; it only stops
  us watching it.
- **The task's rendered output is only in the completion event.** The task
  object carries `spec.output` with the `${{ }}` placeholders unrendered, which
  is a trap: printing it would show a user the template source rather than
  their new repository's URL. This was found by probing a live instance, not by
  reading the docs.
- **`repoUrl` is not a URL.** The `RepoUrlPicker` encodes it as
  `github.com?owner=acme&repo=payments`. The help says so, because it is where
  everyone's first attempt fails.
- **Following a run is a poll, not a stream.** One request per second against
  `/tasks/{id}/events?after=N` plus one for the status. The scaffolder does
  offer an SSE `eventstream` endpoint; a polling loop with an `after` cursor is
  a fraction of the code and the difference is invisible for a run that takes
  minutes.
- **`--timeout` gives up watching, not running.** The message says where to
  follow the task, because the task outlives the command.
- **Secrets are sent as an empty object.** Templates that require scaffolder
  secrets cannot be run from here. Sending them would mean accepting a
  credential on the command line, which this tree refuses everywhere else.

## Alternatives considered

- **Requiring `--confirm` or `--yes`.** The nearest thing to a prompt that a
  pipe allows. Rejected: it is a flag people alias away, and it protects
  against the wrong failure — typing the command by accident is rare, getting a
  parameter wrong is common, and validation catches the second.
- **Defaulting to `--dry-run` and requiring `--run`.** Faithful to ADR-0017 but
  wrong here. `vault get` had a safe reading of its own name; `create` does
  not, and the surprise would be paid on every legitimate use.
- **Streaming with the SSE `eventstream` endpoint.** More code, an extra
  reconnection story, and no visible benefit at this cadence.
- **Skipping validation and letting the scaffolder decide.** Simpler, and it
  turns a typo into a failed task in a shared portal with a task id someone has
  to go and read. The template schema is right there.
- **Accepting a JSON file of values instead of repeated `--field`.** Better for
  many parameters, worse for the two-or-three case this command is for. Worth
  adding later; not worth choosing between now.

## Revisit when

A template that needs scaffolder secrets has to be runnable, or someone wants
to inspect and follow a task they did not start — both point at a `backstage
task <id>` command that does not exist yet.
