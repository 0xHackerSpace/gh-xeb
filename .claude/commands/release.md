---
description: Prepare a tagged release of the extension
argument-hint: <version, e.g. v0.1.0>
allowed-tools: Bash(make check:*), Bash(./script/build.sh:*), Bash(git log:*), Bash(git status:*), Bash(git diff:*), Read
---

Prepare release $1.

1. Confirm the working tree is clean and we are on `main` up to date with origin.
2. Run `make check`.
3. Run `./script/build.sh $1` and confirm every expected platform binary lands
   in `dist/`, then remove `dist/`.
4. Summarise what changed since the previous tag from `git log`.
5. Print the exact `git tag -a $1` and `git push origin $1` commands for me to
   run — do not tag or push yourself.

Pushing the tag triggers `.github/workflows/release.yml`, which attaches the
precompiled binaries via `cli/gh-extension-precompile`.
