---
applyTo: ".github/workflows/*.yml"
---

# Workflow conventions

- Pin actions to a major version tag (`@v4`, `@v5`).
- Set the least `permissions` a job needs: `contents: read` for CI,
  `contents: write` only for the release job that uploads assets.
- Resolve the Go version from `go.mod` (`go-version-file: go.mod`) so the
  toolchain is defined in exactly one place.
- Do not change the release job away from `cli/gh-extension-precompile` — it is
  what produces the assets `gh extension install` expects.
