#!/usr/bin/env bash
# build-deb.sh — build a Debian .deb package for gpg-attest
# Usage: build-deb.sh [/path/to/binary] [version] [architecture]
#   Defaults: binary=./build/gpg-attest, version=dev, architecture=amd64
#
# Requires: dpkg-deb
# Installs to system-level paths:
#   Binary:   /usr/bin/gpg-attest
#   Firefox:  /usr/lib/mozilla/native-messaging-hosts/
#   Chrome:   /etc/chromium/native-messaging-hosts/
#   Chrome:   /etc/opt/chrome/native-messaging-hosts/
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BINARY="${1:-$REPO_ROOT/build/gpg-attest}"
VERSION="${2:-dev}"
ARCH="${3:-amd64}"

if [[ ! -f "$BINARY" ]]; then
  echo "error: binary not found at $BINARY" >&2
  echo "Run 'make build' first." >&2
  exit 1
fi

if ! command -v dpkg-deb &>/dev/null; then
  echo "error: dpkg-deb not found — install dpkg: apt install dpkg" >&2
  exit 1
fi

BUILD_DIR="$REPO_ROOT/build"
DEB_ROOT="$BUILD_DIR/deb-root"
SYSTEM_BINARY_PATH="/usr/bin/gpg-attest"

FIREFOX_MANIFEST_SRC="$REPO_ROOT/manifests/firefox/org.gpg_attest.client.json"
CHROME_MANIFEST_SRC="$REPO_ROOT/manifests/chrome/org.gpg_attest.client.json"

# System-level native messaging host directories (Debian/Ubuntu conventions)
FIREFOX_MANIFEST_DIR="$DEB_ROOT/usr/lib/mozilla/native-messaging-hosts"
CHROMIUM_MANIFEST_DIR="$DEB_ROOT/etc/chromium/native-messaging-hosts"
CHROME_MANIFEST_DIR="$DEB_ROOT/etc/opt/chrome/native-messaging-hosts"

# 1. Clean and assemble payload
rm -rf "$DEB_ROOT"
mkdir -p "$DEB_ROOT/usr/bin"
cp "$BINARY" "$DEB_ROOT/usr/bin/gpg-attest"
chmod 0755 "$DEB_ROOT/usr/bin/gpg-attest"

# 2. Install native messaging manifests with real binary path
install_manifest() {
  local src="$1"
  local dest_dir="$2"
  mkdir -p "$dest_dir"
  sed "s|BINARY_PATH_PLACEHOLDER|$SYSTEM_BINARY_PATH|g" "$src" > "$dest_dir/org.gpg_attest.client.json"
}

install_manifest "$FIREFOX_MANIFEST_SRC" "$FIREFOX_MANIFEST_DIR"
install_manifest "$CHROME_MANIFEST_SRC"  "$CHROMIUM_MANIFEST_DIR"
install_manifest "$CHROME_MANIFEST_SRC"  "$CHROME_MANIFEST_DIR"

# 3. Write DEBIAN/control
mkdir -p "$DEB_ROOT/DEBIAN"
cat > "$DEB_ROOT/DEBIAN/control" <<EOF
Package: gpg-attest
Version: ${VERSION}
Architecture: ${ARCH}
Maintainer: gpg-attest <gpg-attest@gpg-attest.org>
Depends: gnupg
Description: gpg-attest native messaging host
 Native messaging host for the attestension browser extension.
 Bridges the browser to the local gpg binary so that private keys
 never leave the GPG keyring.
Homepage: https://gpg-attest.org
Section: utils
Priority: optional
EOF

# 4. Build the .deb
OUTPUT_DEB="$BUILD_DIR/gpg-attest_${VERSION}_${ARCH}.deb"
dpkg-deb --build "$DEB_ROOT" "$OUTPUT_DEB"

echo ""
echo "Package built: $OUTPUT_DEB"
echo "Install with:  sudo dpkg -i $OUTPUT_DEB"

# 5. Clean up staging dir
rm -rf "$DEB_ROOT"