#!/bin/sh
set -e

VERSION="${VERSION:-v1.3.0}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
NONINTERACTIVE="${NONINTERACTIVE:-0}"

print_header() {
  printf '%s\n' '  ____             __'
  printf '%s\n' ' / __ \____  _____/ /_____ ___  ____ _____'
  printf '%s\n' '/ / / / __ \/ ___/ //_/ _ `__ \/ __ `/ __ \'
  printf '%s\n' '/ /_/ / /_/ / /__/ ,< /  __/ / / /_/ / / / /'
  printf '%s\n' '\____/\____/\___/_/|_|\___/_/ /_/\__,_/_/ /_/'
  printf '\n'
  printf '%s\n' 'Dockman installer'
}

normalize_platform() {
  OS=$(uname | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$OS" in
    linux)
      case "$ARCH" in
        x86_64) ARCH="amd64" ;;
        aarch64|arm64) ARCH="arm64" ;;
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
      echo "Unsupported OS: $OS"
      exit 1
      ;;
  esac
}

confirm_install() {
  if [ "$NONINTERACTIVE" = "1" ]; then
    return 0
  fi

  if [ ! -t 0 ] || [ ! -t 1 ]; then
    return 0
  fi

  printf 'Install Dockman %s to %s/dockman? [Y/n] ' "$VERSION" "$INSTALL_DIR"
  read answer
  case "$answer" in
    ""|y|Y|yes|YES)
      return 0
      ;;
    *)
      echo "Installation cancelled."
      exit 0
      ;;
  esac
}

install_binary() {
  URL="https://github.com/jonace-mpelule/dockman/releases/download/$VERSION/dockman-$OS-$ARCH"
  TARGET="$INSTALL_DIR/dockman"

  print_header
  printf 'Version      %s\n' "$VERSION"
  printf 'Platform     %s/%s\n' "$OS" "$ARCH"
  printf 'Install to   %s\n' "$TARGET"
  printf 'Download     %s\n' "$URL"
  printf '\n'

  confirm_install

  mkdir -p "$INSTALL_DIR"
  echo "Downloading release binary..."
  curl -fsSL "$URL" -o "$TARGET"
  chmod +x "$TARGET"

  printf '\nInstalled dockman %s to %s\n' "$VERSION" "$TARGET"
}

normalize_platform
install_binary
