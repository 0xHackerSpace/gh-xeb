# ADR-0019: `backstage ofertas` reshapes Templates into an offer catalogue

- **Status:** Accepted (extended by [ADR-0020](0020-backstage-create.md))
- **Date:** 2026-09-01
- **Deciders:** @IanOliv
- **Builds on:** [ADR-0016](0016-backstage-catalog-command.md).

## Context

[ADR-0016](0016-backstage-catalog-command.md) gave `backstage` three
commands that mirror the catalog API: list entities, fetch one, find the
current repository's. Each returns what Backstage returns.

`Template` entities do not fit that. What a developer wants from the portal is
"what can I ask it to create, and what will it need from me?" — and the answer
lives in `spec.parameters`, a JSON Schema fragment none of the three commands
renders. `backstage get` shows a Template's type, lifecycle, owner and
relations, which for a Template is the least interesting part of it; the
parameters appear only under `--json`, as raw schema.

The question is a product question, not an API question, and no single catalog
endpoint answers it.

## Decision

`backstage ofertas` lists every `Template` reduced to four fields:

```json
{
  "name": "terraform-module",
  "title": "Módulo Terraform",
  "description": "Módulo Terraform reutilizável com ...",
  "fields": [
    { "name": "name", "description": "Sem o prefixo \"terraform-\"", "required": true },
    { "name": "description", "description": "O que este módulo provisiona", "required": false }
  ]
}
```

`fields` is `spec.parameters` flattened across the template's form pages. Each
field carries a name, a description and whether it is required — nothing else
from the schema.

This is the first command in the tree that **reshapes** catalog data rather
than rendering it. `entities` and `get` are views onto the API; `ofertas` is a
view onto a question.

Four smaller choices inside it:

- **`description` falls back to the schema's `title`.** Many parameters carry
  only a form label. A field with no text tells the reader nothing.
- **Field order is page order, then required-first, then alphabetical.**
- **Both shapes of `spec.parameters` are accepted** — the usual array of pages,
  and the bare object a single-page template may store.
- **The envelope key is `templates`, not the `items` used by `entities`.** The
  payload is a different shape, and reusing the name would imply it is not.

## Consequences

- **The catalog's own field order within a page is lost, and cannot be
  recovered here.** `Entity.Spec` is `map[string]interface{}`
  ([ADR-0016](0016-backstage-catalog-command.md)), and a JSON object has no
  order once decoded into a map. The sort is deterministic and puts the
  mandatory fields first, which is useful, but it is not the order the form
  shows. Fixing it properly means keeping `spec` as `json.RawMessage` and
  decoding with an order-preserving reader — a change to the client, not to
  this command.
- **The visible cost of that**: in `node-typescript-api`, `repoUrl` sorts last
  because it sits on the third form page, not because it matters least.
- **Everything else in the parameter schema is dropped**: type, default, enum,
  pattern, and the `ui:field` pickers that decide whether a value is chosen
  from the catalog or typed freely. Someone building a form from this output
  has a field list, not a form. That is the intended altitude — `--json` on
  `backstage get` still returns the whole schema for anyone who needs it.
- **A malformed schema degrades rather than fails.** Pages that are not
  objects, a `properties` that is a string, a `required` that is not a list:
  each is skipped and whatever is readable survives. A template with a broken
  schema should not make the whole listing unusable.
- **The command name is Portuguese in an otherwise English tree.** It is the
  word the team uses for this, and `offerings` and `templates` are aliases. The
  help text stays English, like every other command's.
- **`backstage get` is now inconsistent with it**: `ofertas` renders parameters
  for every template, `get` renders them for none. Listed as an open decision
  rather than fixed here, because `get` serves all kinds and a Template-shaped
  special case in it is its own decision.

## Alternatives considered

- **Extending `backstage entities --kind template` with a `--fields` flag.**
  The output shape would differ from every other `entities` result, which is
  worse than a separate command that is honest about being a different thing.
- **Rendering the parameters inside `backstage get`.** Still worth doing, but
  it answers "tell me about this one" rather than "what is on offer", and it
  does not remove the need for a listing.
- **Returning the raw JSON Schema per template.** That is `backstage get
  --json` and it already exists. The value here is the reduction.
- **Naming it `templates`.** Accurate but it names the Backstage kind rather
  than what the user is looking for, and the kind is already reachable as
  `entities --kind template`. Kept as an alias.

## Revisit when

Someone needs to *run* a template. Rendering what a template asks for is one
step from submitting it, and that is a write against the scaffolder — a
different decision, with the same "where does the value come from" problem
[ADR-0017](0017-vault-get-masked-by-default.md) names for writing a secret.
