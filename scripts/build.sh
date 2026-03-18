#!/bin/bash
# build.sh - Cross-platform build script
set -e

VERSION=${VERSION:-$(git describe --tags 2>/dev/null || echo "dev")}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}
DATE=${DATE:-$(date -u +%Y-%m-%d)}

LDFLAGS="-X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE"

mkdir -p dist

echo "Building surge v$VERSION (commit: $COMMIT, date: $DATE)"

# Linux
echo "Building for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags="$LDFLAGS" -o dist/surge-linux-amd64 ./cmd/surge

echo "Building for linux/arm64..."
GOOS=linux GOARCH=arm64 go build -ldflags="$LDFLAGS" -o dist/surge-linux-arm64 ./cmd/surge

# macOS
echo "Building for darwin/amd64..."
GOOS=darwin GOARCH=amd64 go build -ldflags="$LDFLAGS" -o dist/surge-darwin-amd64 ./cmd/surge

echo "Building for darwin/arm64..."
GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o dist/surge-darwin-arm64 ./cmd/surge

# Windows
echo "Building for windows/amd64..."
GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -o dist/surge-windows-amd64.exe ./cmd/surge

echo ""
echo "Build complete. Binaries in dist/:"
ls -la dist/
