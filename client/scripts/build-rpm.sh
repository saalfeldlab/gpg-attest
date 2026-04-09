#!/usr/bin/env bash
# build-rpm.sh — build an RPM package for gpg-attest
# Usage: build-rpm.sh [/path/to/binary] [version] [architecture]
#   Defaults: binary=./build/gpg-attest, version=dev, architecture=x86_64
#
# Requires: rpmbuild (apt install rpm on Debian/Ubuntu)
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
ARCH="${3:-x86_64}"

if [[ ! -f "$BINARY" ]]; then
  echo "error: binary not found at $BINARY" >&2
  echo "Run 'make build' first." >&2
  exit 1
fi

if ! command -v rpmbuild &>/dev/null; then
  echo "error: rpmbuild not found — install rpm: apt install rpm (Debian/Ubuntu)" >&2
  exit 1
fi

BUILD_DIR="$REPO_ROOT/build"
RPM_TOPDIR="$BUILD_DIR/rpm-build"
SYSTEM_BINARY_PATH="/usr/bin/gpg-attest"

FIREFOX_MANIFEST_SRC="$REPO_ROOT/manifests/firefox/org.gpg_attest.client.json"
CHROME_MANIFEST_SRC="$REPO_ROOT/manifests/chrome/org.gpg_attest.client.json"

# 1. Clean and set up rpmbuild directory structure
rm -rf "$RPM_TOPDIR"
mkdir -p "$RPM_TOPDIR"/{BUILD,RPMS,SOURCES,SPECS,SRPMS,BUILDROOT}

# 2. Assemble payload in BUILDROOT
BUILDROOT="$RPM_TOPDIR/BUILDROOT/gpg-attest-${VERSION}-1.${ARCH}"
mkdir -p "$BUILDROOT/usr/bin"
cp "$BINARY" "$BUILDROOT/usr/bin/gpg-attest"
chmod 0755 "$BUILDROOT/usr/bin/gpg-attest"

# 3. Install native messaging manifests with real binary path
install_manifest() {
  local src="$1"
  local dest_dir="$2"
  mkdir -p "$dest_dir"
  sed "s|BINARY_PATH_PLACEHOLDER|$SYSTEM_BINARY_PATH|g" "$src" > "$dest_dir/org.gpg_attest.client.json"
}

FIREFOX_MANIFEST_DIR="$BUILDROOT/usr/lib/mozilla/native-messaging-hosts"
CHROMIUM_MANIFEST_DIR="$BUILDROOT/etc/chromium/native-messaging-hosts"
CHROME_MANIFEST_DIR="$BUILDROOT/etc/opt/chrome/native-messaging-hosts"

install_manifest "$FIREFOX_MANIFEST_SRC" "$FIREFOX_MANIFEST_DIR"
install_manifest "$CHROME_MANIFEST_SRC"  "$CHROMIUM_MANIFEST_DIR"
install_manifest "$CHROME_MANIFEST_SRC"  "$CHROME_MANIFEST_DIR"

# 4. Write spec file
cat > "$RPM_TOPDIR/SPECS/gpg-attest.spec" <<EOF
Name:    gpg-attest
Version: ${VERSION}
Release: 1
Summary: gpg-attest native messaging host
License: MIT
URL:     https://gpg-attest.org
Requires: gnupg2

%description
Native messaging host for the attestension browser extension.
Bridges the browser to the local gpg binary so that private keys
never leave the GPG keyring.

%install
cp -a %{buildroot}/* %{buildroot}/ 2>/dev/null || true

%post
echo ""
echo "  gpg-attest native messaging host installed."
echo ""
echo "  Install the browser extension:"
echo "    Firefox: TODO_FIREFOX_ADDON_URL"
echo "    Chrome:  TODO_CHROME_WEBSTORE_URL"
echo ""

%files
%attr(0755, root, root) /usr/bin/gpg-attest
/usr/lib/mozilla/native-messaging-hosts/org.gpg_attest.client.json
%config /etc/chromium/native-messaging-hosts/org.gpg_attest.client.json
%config /etc/opt/chrome/native-messaging-hosts/org.gpg_attest.client.json
EOF

# 5. Build the RPM
rpmbuild \
  --define "_topdir $RPM_TOPDIR" \
  --define "_binary_payload w9.gzdio" \
  --target "$ARCH" \
  -bb "$RPM_TOPDIR/SPECS/gpg-attest.spec"

# 6. Move RPM to build directory
OUTPUT_RPM=$(find "$RPM_TOPDIR/RPMS" -name "*.rpm" | head -1)
if [[ -z "$OUTPUT_RPM" ]]; then
  echo "error: rpmbuild produced no output" >&2
  exit 1
fi
mv "$OUTPUT_RPM" "$BUILD_DIR/"
FINAL_RPM="$BUILD_DIR/$(basename "$OUTPUT_RPM")"

echo ""
echo "Package built: $FINAL_RPM"
echo "Install with:  sudo rpm -i $FINAL_RPM"

# 7. Clean up staging dirs
rm -rf "$RPM_TOPDIR"
