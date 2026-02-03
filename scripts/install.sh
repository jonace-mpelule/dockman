#!/bin/sh
set -e

VERSION="v1.0.0"
OS=$(uname | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Map uname architecture to Go arch
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
fi

URL="https://github.com/jonace-mpelule/dockman/releases/download/$VERSION/dockman-$OS-$ARCH"
curl -L "$URL" -o /usr/local/bin/dockman
chmod +x /usr/local/bin/dockman

echo "Installed dockman $VERSION!"
