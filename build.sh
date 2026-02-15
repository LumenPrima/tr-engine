#!/usr/bin/env bash
set -euo pipefail

VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
echo "Building tr-engine ${VERSION}..."
go build -ldflags "-X main.version=${VERSION}" -o tr-engine.exe ./cmd/tr-engine
echo "Done: tr-engine.exe (${VERSION})"
