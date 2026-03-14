#!/usr/bin/env bash
# install-macos.sh — STUB: macOS native messaging host registration
#
# Not yet implemented. When implemented, this script will:
#
#   1. Copy the binary to ~/Library/Application Support/sig-stuff/ (or similar)
#   2. Write the Firefox manifest to:
#        ~/Library/Application Support/Mozilla/NativeMessagingHosts/dev.sig_stuff.native.json
#   3. Write the Chrome manifest to:
#        ~/Library/Application Support/Google/Chrome/NativeMessagingHosts/dev.sig_stuff.native.json
#
# macOS also requires the binary to be code-signed (ad-hoc signing is sufficient for
# local development): codesign --sign - /path/to/sig-stuff-native
#
# Note: Chrome on macOS looks in both the user and system NativeMessagingHosts dirs.

echo "macOS install not yet implemented." >&2
exit 1
