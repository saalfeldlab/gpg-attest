#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ── 1. Build + install client ──────────────────────────────────────────
echo "==> Building and installing client..."
(cd "$SCRIPT_DIR/client" && make install)

# ── 2. Start gpg-attest stack (idempotent) ───────────────────────────────────
echo "==> Starting gpg-attest stack..."
"$SCRIPT_DIR/.devcontainer/start-services.sh"

# ── 3. Start test HTTP server (if not already running) ──────────────────────
if nc -z 127.0.0.1 8080 2>/dev/null; then
  echo "==> Test HTTP server already running on :8080, skipping."
else
  echo "==> Starting test HTTP server on :8080..."
  mkdir -p "$HOME/.gpg-attest/logs"
  (cd "$SCRIPT_DIR/testpage" && python3 -m http.server 8080 \
    >>"$HOME/.gpg-attest/logs/testpage.log" 2>&1) &
  disown $!
  echo -n "    Waiting for test server"
  for i in $(seq 1 20); do
    if nc -z 127.0.0.1 8080 2>/dev/null; then
      echo " ready."
      break
    fi
    sleep 0.5
    echo -n "."
  done
  if ! nc -z 127.0.0.1 8080 2>/dev/null; then
    echo
    echo "WARNING: test server did not come up after 10 s." >&2
  fi
fi

# ── 4. Open Firefox with three tabs ─────────────────────────────────────────
echo "==> Opening Firefox (about:debugging | about:addons | http://localhost:8080)..."
firefox about:debugging about:addons http://localhost:8080 https://gpg-attest.org/swagger/index.html&
disown $!

echo "==> Done."
