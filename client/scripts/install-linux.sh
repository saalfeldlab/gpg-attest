#!/usr/bin/env bash
# install-linux.sh — install gpg-attest native messaging host on Linux
# Usage: install-linux.sh [/path/to/binary]
#   Defaults to ./build/gpg-attest if no argument given.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BINARY="${1:-$REPO_ROOT/build/gpg-attest}"
if [[ ! -f "$BINARY" ]]; then
  echo "error: binary not found at $BINARY" >&2
  echo "Run 'make build' first." >&2
  exit 1
fi

INSTALL_DIR="$HOME/.local/bin"
INSTALL_PATH="$INSTALL_DIR/gpg-attest"

# 1. Install binary
mkdir -p "$INSTALL_DIR"
cp -f "$BINARY" "$INSTALL_PATH"
chmod +x "$INSTALL_PATH"
echo "Installed binary: $INSTALL_PATH"

# 2. Helper: write a manifest with the real binary path substituted
write_manifest() {
  local src="$1"
  local dest_dir="$2"
  mkdir -p "$dest_dir"
  sed "s|BINARY_PATH_PLACEHOLDER|$INSTALL_PATH|g" "$src" > "$dest_dir/org.gpg_attest.client.json"
  echo "Wrote manifest:  $dest_dir/org.gpg_attest.client.json"
}

FIREFOX_MANIFEST="$REPO_ROOT/manifests/firefox/org.gpg_attest.client.json"
CHROME_MANIFEST="$REPO_ROOT/manifests/chrome/org.gpg_attest.client.json"

# 3. Firefox
write_manifest "$FIREFOX_MANIFEST" "$HOME/.mozilla/native-messaging-hosts"

# 4. Chromium
write_manifest "$CHROME_MANIFEST" "$HOME/.config/chromium/NativeMessagingHosts"

# 5. Google Chrome (separate profile dir from Chromium)
CHROME_DIR="$HOME/.config/google-chrome/NativeMessagingHosts"
if [[ "$CHROME_DIR" != "$HOME/.config/chromium/NativeMessagingHosts" ]]; then
  write_manifest "$CHROME_MANIFEST" "$CHROME_DIR"
fi

echo ""
echo "Done. Reload your browser extension to pick up the new host."
