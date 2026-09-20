# CLAUDE.md

The project conventions live in `AGENTS.md`, shared with GitHub Copilot. Read it
before changing code.

@AGENTS.md

## Claude Code notes

- `make check` (gofmt + `go vet` + `go test`) is the gate CI enforces. Run it
  before reporting a change as done.
- `/build`, `/test` and `/newcmd` slash commands live in `.claude/commands/`.
- `go` commands are allow-listed in `.claude/settings.json`; `gh extension
  install/upgrade` and `git push` are not, and will prompt.
- The local Go toolchain may be older than `go.mod` requires. That is fine —
  `GOTOOLCHAIN=auto` fetches the pinned version on first build. Do not "fix"
  this by lowering the `go` directive in `go.mod`.
