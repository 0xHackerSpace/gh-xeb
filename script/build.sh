#!/usr/bin/env bash
# Cross-compiles release binaries for `cli/gh-extension-precompile`.
#
# The action invokes this script with the release tag as $1 and expects the
# binaries to land in ./dist named gh-<ext>-<goos>-<goarch>[.exe].
set -euo pipefail

TAG="${1:-dev}"
BINARY="gh-xeb"
LDFLAGS="-s -w -X github.com/0xHackerSpace/gh-xeb/cmd.version=${TAG}"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/386"
  "linux/amd64"
  "linux/arm"
  "linux/arm64"
  "windows/386"
  "windows/amd64"
  "windows/arm64"
)

rm -rf dist
mkdir -p dist

for platform in "${PLATFORMS[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi

  echo "building ${goos}/${goarch}"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "dist/${BINARY}-${goos}-${goarch}${ext}" .
done

echo "artifacts:"
ls -1 dist
