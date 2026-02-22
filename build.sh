#!/usr/bin/env bash
# Build tr-engine with version info injected via ldflags.
# Every build gets a unique buildTime, even identical rebuilds.
set -euo pipefail

PKG="main"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
BUILD_TIME="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

LDFLAGS="-X ${PKG}.version=${VERSION} -X ${PKG}.commit=${COMMIT} -X ${PKG}.buildTime=${BUILD_TIME}"

OUTPUT="tr-engine"
if [[ "${GOOS:-$(go env GOOS)}" == "windows" ]]; then
  OUTPUT="tr-engine.exe"
fi

echo "Building tr-engine ${VERSION} (${COMMIT}) at ${BUILD_TIME}"
go build -ldflags "${LDFLAGS}" -o "${OUTPUT}" ./cmd/tr-engine
echo "Done: ${OUTPUT}"
