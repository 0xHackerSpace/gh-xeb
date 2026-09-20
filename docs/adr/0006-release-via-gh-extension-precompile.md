# ADR-0006: Release with `cli/gh-extension-precompile` and an explicit build script

- **Status:** Accepted
- **Date:** 2026-08-28
- **Deciders:** @IanOliv

## Context

`gh extension install OWNER/REPO` looks for release assets named
`gh-<extension>-<goos>-<goarch>[.exe]`. If the release has no asset matching
the user's platform, the install fails. So a precompiled extension
([ADR-0001](0001-precompiled-go-extension.md)) is only installable if every
release carries the full matrix of binaries.

GitHub publishes `cli/gh-extension-precompile`, an action that does exactly
this, and can either build Go projects itself or delegate to a script.

## Decision

`.github/workflows/release.yml` runs on `v*` tags and calls
`cli/gh-extension-precompile@v2` with `build_script_override:
script/build.sh`.

`script/build.sh` receives the tag as `$1`, cross-compiles the nine supported
platforms into `dist/`, and bakes the tag into the binary with
`-ldflags "-X .../internal/cmd.version=$TAG"`.

## Consequences

- Tagging is the entire release process: `git push origin v0.1.0`.
- Asset naming is guaranteed correct, because the action defines the contract
  and the script follows it.
- `gh cli-extension version` reports the released tag rather than
  `(devel)`, because the ldflags injection happens in the same script.
- The script is runnable locally (`./script/build.sh v0.0.1-test`), so a
  release can be rehearsed without tagging.
- The build matrix is now duplicated in two places: `script/build.sh` (the
  real one) and the `cross-compile` job in `ci.yml` (which only checks that
  each target compiles). Keep them in sync or CI stops covering a platform we
  ship.
- The module path appears in the ldflags string in both `script/build.sh` and
  the `Makefile`. Renaming the module breaks version injection silently — the
  build still succeeds and just reports the wrong version.

## Alternatives considered

- **Let the action build Go itself** (no `build_script_override`). Fewer
  moving parts, but no control over `-ldflags`, so the version could not be
  injected, and the platform list would not be ours to choose.
- **GoReleaser.** More capable than we need, and its default asset naming does
  not match what `gh extension install` expects without configuration.
- **Build manually and upload.** Works once, forgotten by the third release.

## Revisit when

We need signed or notarised binaries, or SBOM/provenance attestations.
