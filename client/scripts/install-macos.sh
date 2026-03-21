#!/usr/bin/env bash
# install-macos.sh — install gpg-attest native messaging host on macOS
# Usage: install-macos.sh [/path/to/binary]
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

INSTALL_DIR="$HOME/Library/Application Support/gpg-attest"
INSTALL_PATH="$INSTALL_DIR/gpg-attest"

# 1. Install binary
mkdir -p "$INSTALL_DIR"
cp -f "$BINARY" "$INSTALL_PATH"
chmod +x "$INSTALL_PATH"
echo "Installed binary: $INSTALL_PATH"

# 2. Ad-hoc code-sign (Chrome on macOS refuses unsigned native hosts)
if command -v codesign &>/dev/null; then
  codesign --sign - "$INSTALL_PATH"
  echo "Code-signed:     $INSTALL_PATH"
else
  echo "warning: codesign not found — skipping code signing (Xcode CLI tools required)" >&2
fi

# 3. Helper: write a manifest with the real binary path substituted
write_manifest() {
  local src="$1"
  local dest_dir="$2"
  mkdir -p "$dest_dir"
  sed "s|BINARY_PATH_PLACEHOLDER|$INSTALL_PATH|g" "$src" > "$dest_dir/org.gpg_attest.client.json"
  echo "Wrote manifest:  $dest_dir/org.gpg_attest.client.json"
}

FIREFOX_MANIFEST="$REPO_ROOT/manifests/firefox/org.gpg_attest.client.json"
CHROME_MANIFEST="$REPO_ROOT/manifests/chrome/org.gpg_attest.client.json"

# 4. Firefox
write_manifest "$FIREFOX_MANIFEST" "$HOME/Library/Application Support/Mozilla/NativeMessagingHosts"

# 5. Google Chrome
write_manifest "$CHROME_MANIFEST" "$HOME/Library/Application Support/Google/Chrome/NativeMessagingHosts"

# 6. Chromium
write_manifest "$CHROME_MANIFEST" "$HOME/Library/Application Support/Chromium/NativeMessagingHosts"

echo ""
echo "Done. Reload your browser extension to pick up the new host."
