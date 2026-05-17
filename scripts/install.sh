#!/usr/bin/env sh
# Install lockie from GitHub Releases (pre-built binary).
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/ujjalsharma100/lockie/main/scripts/install.sh | sh
#   curl -fsSL ... | sh -s -- --force
#   LOCKIE_VERSION=v0.1.0 curl -fsSL ... | sh
set -eu

REPO="${LOCKIE_REPO:-ujjalsharma100/lockie}"
FORCE=0

for arg in "$@"; do
  case "$arg" in
    --force|-f) FORCE=1 ;;
    -h|--help)
      echo "Usage: install.sh [--force]"
      echo "  LOCKIE_VERSION=v0.1.0  pin release (default: latest)"
      echo "  LOCKIE_INSTALL_DIR=... override install directory"
      exit 0
      ;;
    *)
      echo "install.sh: unknown argument: $arg" >&2
      exit 1
      ;;
  esac
done

detect_os() {
  os=$(uname -s)
  case "$os" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *)
      echo "install.sh: unsupported OS: $os (supported: darwin, linux)" >&2
      echo "  Windows: use scripts/install.ps1" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  arch=$(uname -m)
  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "install.sh: unsupported architecture: $arch (supported: amd64, arm64)" >&2
      exit 1
      ;;
  esac
}

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "install.sh: required command not found: $1" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd tar

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "install.sh: sha256sum or shasum required for checksum verification" >&2
    exit 1
  fi
}

OS=$(detect_os)
ARCH=$(detect_arch)

if [ -n "${LOCKIE_VERSION:-}" ]; then
  VERSION="${LOCKIE_VERSION#v}"
else
  need_cmd grep
  api="https://api.github.com/repos/${REPO}/releases/latest"
  json=$(curl -fsSL "$api") || {
    echo "install.sh: failed to fetch latest release from $api" >&2
    exit 1
  }
  VERSION=$(printf '%s' "$json" | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' | head -n1 | sed 's/.*"\([^"]*\)"$/\1/' | sed 's/^v//')
  if [ -z "$VERSION" ]; then
    echo "install.sh: could not parse latest release tag from GitHub API" >&2
    exit 1
  fi
fi

TAG="v${VERSION#v}"
ARCHIVE="lockie_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${TAG}"
URL="${BASE}/${ARCHIVE}"

if [ "$FORCE" -eq 0 ] && command -v lockie >/dev/null 2>&1; then
  existing=$(command -v lockie)
  echo "install.sh: lockie already on PATH at $existing" >&2
  echo "  Re-run with --force to replace, or remove it first." >&2
  exit 1
fi

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "Downloading $URL"
curl -fsSL "$URL" -o "$TMP/$ARCHIVE"

CHECKSUMS_URL="${BASE}/checksums.txt"
if curl -fsSL "$CHECKSUMS_URL" -o "$TMP/checksums.txt" 2>/dev/null; then
  expected=$(grep " ${ARCHIVE}\$" "$TMP/checksums.txt" | awk '{print $1}')
  if [ -n "$expected" ]; then
    actual=$(sha256_file "$TMP/$ARCHIVE")
    if [ "$actual" != "$expected" ]; then
      echo "install.sh: checksum mismatch for $ARCHIVE" >&2
      echo "  expected: $expected" >&2
      echo "  actual:   $actual" >&2
      exit 1
    fi
  fi
else
  echo "install.sh: warning: could not verify checksum (checksums.txt missing?)" >&2
fi

tar -xzf "$TMP/$ARCHIVE" -C "$TMP"
BIN="$TMP/lockie"
if [ ! -f "$BIN" ]; then
  echo "install.sh: archive did not contain lockie binary" >&2
  exit 1
fi
chmod 755 "$BIN"

install_dir() {
  if [ -n "${LOCKIE_INSTALL_DIR:-}" ]; then
    echo "$LOCKIE_INSTALL_DIR"
    return
  fi
  if [ -n "${XDG_BIN_HOME:-}" ]; then
    echo "$XDG_BIN_HOME"
    return
  fi
  if [ -d "$HOME/.local/bin" ] || mkdir -p "$HOME/.local/bin" 2>/dev/null; then
    echo "$HOME/.local/bin"
    return
  fi
  echo "/usr/local/bin"
}

DEST_DIR=$(install_dir)
mkdir -p "$DEST_DIR"
DEST="$DEST_DIR/lockie"

if [ "$DEST_DIR" = "/usr/local/bin" ] && [ ! -w "$DEST_DIR" ]; then
  need_cmd sudo
  sudo install -m 755 "$BIN" "$DEST"
else
  install -m 755 "$BIN" "$DEST"
fi

case ":${PATH:-}:" in
  *:"$DEST_DIR":*) ;;
  *)
    echo ""
    echo "Add lockie to your PATH (if needed):"
    echo "  export PATH=\"$DEST_DIR:\$PATH\""
    ;;
esac

echo ""
echo "lockie $VERSION installed at $DEST"
echo "Next steps:"
echo "  lockie version"
echo "  lockie install cursor --scope user"
echo "  lockie install claude-code --scope user"
echo "  lockie status"
echo ""
echo "Manual E2E checklist: https://github.com/${REPO}/blob/main/test/e2e/README.md"
