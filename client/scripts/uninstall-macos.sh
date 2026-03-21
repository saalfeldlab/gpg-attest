#!/usr/bin/env bash
# uninstall-macos.sh — remove gpg-attest native messaging host from macOS
set -euo pipefail

INSTALL_PATH="$HOME/Library/Application Support/gpg-attest/gpg-attest"
MANIFEST_NAME="org.gpg_attest.client.json"

remove() {
  local path="$1"
  if [[ -e "$path" ]]; then
    rm -f "$path"
    echo "Removed: $path"
  fi
}

remove "$INSTALL_PATH"
remove "$HOME/Library/Application Support/Mozilla/NativeMessagingHosts/$MANIFEST_NAME"
remove "$HOME/Library/Application Support/Google/Chrome/NativeMessagingHosts/$MANIFEST_NAME"
remove "$HOME/Library/Application Support/Chromium/NativeMessagingHosts/$MANIFEST_NAME"

echo "Uninstall complete."
