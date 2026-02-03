#!/bin/sh
set -e

VERSION="v1.0.2"

OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  linux)
    case "$ARCH" in
      x86_64) ARCH="amd64" ;;
      aarch64) ARCH="arm64" ;;
      *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    ;;
  darwin)
    case "$ARCH" in
      x86_64) ARCH="amd64" ;;
      arm64) ARCH="arm64" ;;
      *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    ;;
  *)
    echo "Unsupported OS: $OS"; exit 1
    ;;
esac

URL="https://github.com/jonace-mpelule/dockman/releases/download/$VERSION/dockman-$OS-$ARCH"
echo "Downloading $URL..."
curl -L "$URL" -o /usr/local/bin/dockman
chmod +x /usr/local/bin/dockman

echo "dockman $VERSION installed successfully!"
