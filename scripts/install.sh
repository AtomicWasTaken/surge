#!/bin/bash
# install.sh - Install surge binary
set -e

REPO="AtomicWasTaken/surge"

# Detect latest release
LATEST=$(gh release list --limit 1 --json tagName --jq '.[0].tagName' 2>/dev/null || echo "latest")

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
[ "$ARCH" = "x86_64" ] && ARCH="amd64"
[ "$ARCH" = "arm64" ] && ARCH="arm64"

EXT=""
if [ "$OS" = "windows" ]; then
  EXT=".exe"
fi

URL="https://github.com/$REPO/releases/download/$LATEST/surge-${OS}-${ARCH}${EXT}"

# Check if curl is available
if ! command -v curl &> /dev/null; then
  echo "curl is required but not installed"
  exit 1
fi

echo "Installing surge from $URL"
curl -sSL "$URL" -o /tmp/surge
chmod +x /tmp/surge

# Install to user-local bin or system-wide
if [ -w /usr/local/bin ]; then
  mv /tmp/surge /usr/local/bin/surge
  echo "Installed to /usr/local/bin/surge"
else
  mkdir -p "$HOME/.local/bin"
  mv /tmp/surge "$HOME/.local/bin/surge"
  echo "Installed to $HOME/.local/bin/surge"
  echo "Make sure $HOME/.local/bin is in your PATH"
fi
