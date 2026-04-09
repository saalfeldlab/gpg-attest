#!/usr/bin/env bash
# build-pkg.sh — build a macOS .pkg installer for gpg-attest
# Usage: build-pkg.sh [/path/to/binary] [version]
#   Defaults: binary=./build/gpg-attest, version=dev
#
# Requires: Xcode CLI tools (pkgbuild)
# Installs to system-level paths:
#   Binary:   /usr/local/bin/gpg-attest
#   Firefox:  /Library/Application Support/Mozilla/NativeMessagingHosts/
#   Chrome:   /Library/Google/Chrome/NativeMessagingHosts/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BINARY="${1:-$REPO_ROOT/build/gpg-attest}"
VERSION="${2:-dev}"

if [[ ! -f "$BINARY" ]]; then
  echo "error: binary not found at $BINARY" >&2
  echo "Run 'make build' first." >&2
  exit 1
fi

if ! command -v pkgbuild &>/dev/null; then
  echo "error: pkgbuild not found — install Xcode CLI tools: xcode-select --install" >&2
  exit 1
fi

BUILD_DIR="$REPO_ROOT/build"
PKG_ROOT="$BUILD_DIR/pkg-root"
PKG_SCRIPTS="$BUILD_DIR/pkg-scripts"
SYSTEM_BINARY_PATH="/usr/local/bin/gpg-attest"

FIREFOX_MANIFEST_DIR="/Library/Application Support/Mozilla/NativeMessagingHosts"
CHROME_MANIFEST_DIR="/Library/Google/Chrome/NativeMessagingHosts"

FIREFOX_MANIFEST_SRC="$REPO_ROOT/manifests/firefox/org.gpg_attest.client.json"
CHROME_MANIFEST_SRC="$REPO_ROOT/manifests/chrome/org.gpg_attest.client.json"

# 1. Clean and assemble payload root
rm -rf "$PKG_ROOT" "$PKG_SCRIPTS"
mkdir -p "$PKG_ROOT/usr/local/bin"
cp "$BINARY" "$PKG_ROOT/usr/local/bin/gpg-attest"
chmod +x "$PKG_ROOT/usr/local/bin/gpg-attest"
echo "Assembled payload: $PKG_ROOT"

# 2. Write postinstall script (runs as root after pkg installs the payload)
mkdir -p "$PKG_SCRIPTS"
cat > "$PKG_SCRIPTS/postinstall" <<POSTINSTALL
#!/usr/bin/env bash
set -euo pipefail

BINARY_PATH="$SYSTEM_BINARY_PATH"
FIREFOX_DIR="$FIREFOX_MANIFEST_DIR"
CHROME_DIR="$CHROME_MANIFEST_DIR"

mkdir -p "\$FIREFOX_DIR" "\$CHROME_DIR"

# Firefox manifest
cat > "\$FIREFOX_DIR/org.gpg_attest.client.json" <<EOF
$(sed "s|BINARY_PATH_PLACEHOLDER|$SYSTEM_BINARY_PATH|g" "$FIREFOX_MANIFEST_SRC")
EOF

# Chrome manifest
cat > "\$CHROME_DIR/org.gpg_attest.client.json" <<EOF
$(sed "s|BINARY_PATH_PLACEHOLDER|$SYSTEM_BINARY_PATH|g" "$CHROME_MANIFEST_SRC")
EOF

echo "gpg-attest native manifests installed."
echo ""
echo "  gpg-attest native messaging host installed."
echo ""
echo "  Install the browser extension:"
echo "    Firefox: TODO_FIREFOX_ADDON_URL"
echo "    Chrome:  TODO_CHROME_WEBSTORE_URL"
echo ""
POSTINSTALL
chmod +x "$PKG_SCRIPTS/postinstall"
echo "Wrote postinstall script: $PKG_SCRIPTS/postinstall"

# 3. Build the .pkg
OUTPUT_PKG="$BUILD_DIR/gpg-attest.pkg"
pkgbuild \
  --root "$PKG_ROOT" \
  --scripts "$PKG_SCRIPTS" \
  --identifier org.gpg_attest.client \
  --version "$VERSION" \
  "$OUTPUT_PKG"

echo ""
echo "Package built: $OUTPUT_PKG"
echo "Install with:  sudo installer -pkg $OUTPUT_PKG -target /"

# 4. Clean up staging dirs
rm -rf "$PKG_ROOT" "$PKG_SCRIPTS"
