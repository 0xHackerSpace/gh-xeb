---
applyTo: "**/*.go"
---

# Go conventions

- Format with `gofmt`; CI fails on unformatted files.
- Every exported identifier in `internal/gh` gets a doc comment starting with
  its name.
- Commands live in `cmd`, one file per subcommand, each exposing a
  `new<Name>Cmd() *cobra.Command` constructor registered in `NewRootCmd`.
- Depend on the narrow interfaces in `internal/gh` (e.g. `RESTClient`), not on
  go-gh's concrete client types, so tests can substitute fakes.
- Table-driven tests with `t.Run` subtests are the default style. Restore any
  package-level seam you override with `t.Cleanup`.
- No `panic`, `os.Exit`, or `log.Fatal` outside `main.go`.
