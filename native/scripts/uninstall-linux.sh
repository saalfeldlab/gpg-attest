#!/usr/bin/env bash
# uninstall-linux.sh — remove sig-stuff native messaging host from Linux
set -euo pipefail

INSTALL_PATH="$HOME/.local/bin/sig-stuff-native"
MANIFEST_NAME="dev.sig_stuff.native.json"

remove() {
  local path="$1"
  if [[ -e "$path" ]]; then
    rm -f "$path"
    echo "Removed: $path"
  fi
}

remove "$INSTALL_PATH"
remove "$HOME/.mozilla/native-messaging-hosts/$MANIFEST_NAME"
remove "$HOME/.config/chromium/NativeMessagingHosts/$MANIFEST_NAME"
remove "$HOME/.config/google-chrome/NativeMessagingHosts/$MANIFEST_NAME"

echo "Uninstall complete."
