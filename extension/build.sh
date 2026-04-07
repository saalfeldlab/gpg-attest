#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BUILD_DIR="$SCRIPT_DIR/build"
VERSION=$(jq -r '.version' "$SCRIPT_DIR/manifest.json")

rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR/chrome" "$BUILD_DIR/firefox"

# Files to package (everything except build artifacts and the shared manifest)
copy_common() {
  local dest="$1"
  cp -r "$SCRIPT_DIR/background" "$dest/"
  cp -r "$SCRIPT_DIR/content" "$dest/"
  cp -r "$SCRIPT_DIR/icons" "$dest/"
  cp -r "$SCRIPT_DIR/options" "$dest/"
}

# --- Chrome manifest ---
# Remove Gecko-only fields: browser_specific_settings, background.scripts
jq 'del(.browser_specific_settings) | del(.background.scripts)' \
  "$SCRIPT_DIR/manifest.json" > "$BUILD_DIR/chrome/manifest.json"

copy_common "$BUILD_DIR/chrome"

(cd "$BUILD_DIR/chrome" && zip -rq "../attestension-chrome-${VERSION}.zip" .)
echo "Built $BUILD_DIR/attestension-chrome-${VERSION}.zip"

# --- Firefox manifest ---
# Remove Chrome-only field: background.service_worker
jq 'del(.background.service_worker)' \
  "$SCRIPT_DIR/manifest.json" > "$BUILD_DIR/firefox/manifest.json"

copy_common "$BUILD_DIR/firefox"

(cd "$BUILD_DIR/firefox" && zip -rq "../attestension-firefox-${VERSION}.zip" .)
echo "Built $BUILD_DIR/attestension-firefox-${VERSION}.zip"

# --- Sign Firefox extension via AMO (optional) ---
ENV_FILE="$SCRIPT_DIR/../.env"
if [[ -f "$ENV_FILE" ]]; then
  # Source .env in a subshell to avoid leaking secrets into the environment
  JWT_ISSUER=$(set -a && . "$ENV_FILE" && echo "$JWT_ISSUER")
  JWT_SECRET=$(set -a && . "$ENV_FILE" && echo "$JWT_SECRET")

  if [[ -n "${JWT_ISSUER:-}" && -n "${JWT_SECRET:-}" ]]; then
    echo "Signing Firefox extension via AMO..."
    npx web-ext sign \
      --source-dir "$BUILD_DIR/firefox" \
      --artifacts-dir "$BUILD_DIR" \
      --api-key="$JWT_ISSUER" \
      --api-secret="$JWT_SECRET" \
      --channel=unlisted
    echo "Signed .xpi written to $BUILD_DIR/"
  else
    echo "Skipping Firefox signing: JWT_ISSUER and/or JWT_SECRET not set in .env"
  fi
else
  echo "Skipping Firefox signing: .env not found"
fi

# Cleanup staging directories
rm -rf "$BUILD_DIR/chrome" "$BUILD_DIR/firefox"

echo "Done."