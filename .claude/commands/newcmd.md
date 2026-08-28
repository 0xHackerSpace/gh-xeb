---
description: Scaffold a new gh cli-extension subcommand end to end
argument-hint: <subcommand-name> [one-line description]
---

Add a new subcommand named `$1` to this extension. Description: $ARGUMENTS

Follow the "Adding a subcommand" checklist in AGENTS.md exactly:

1. `cmd/$1.go` with a `new<Name>Cmd(deps Deps) *cobra.Command`
   constructor — `Short`, `Args`, and `RunE` set, output through
   `cmd.OutOrStdout()`.
2. Register it in `NewRootCmd` in `cmd/root.go`.
3. If it needs GitHub data, go through `internal/gh` — add a narrow interface
   there and a field on `Deps`, rather than depending on go-gh's concrete
   types.
4. `cmd/$1_test.go` built from `testDeps()` and `runRoot` in
   `helpers_test.go`. No network access in tests.
5. Add the command to the table in `README.md`.

Then run `make check` and report the result.
