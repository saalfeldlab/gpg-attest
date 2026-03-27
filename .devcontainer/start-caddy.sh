#!/usr/bin/env bash
# Starts Caddy reverse proxy with a self-signed TLS cert.
# Idempotent: skips if Caddy is already listening.
set -euo pipefail

HTTPS_PORT=${HTTPS_PORT:-443}
LOG_DIR=$HOME/.gpg-attest/logs
CADDY_CERT_DIR=$HOME/.gpg-attest/caddy

mkdir -p "$LOG_DIR"

log() { echo "[start-caddy] $*" | tee -a "$LOG_DIR/startup.log"; }

# --- TLS cert (self-signed, generated once) ---
if [ ! -f "$CADDY_CERT_DIR/cert.pem" ]; then
    log "Generating self-signed TLS certificate..."
    mkdir -p "$CADDY_CERT_DIR"
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -keyout "$CADDY_CERT_DIR/key.pem" -out "$CADDY_CERT_DIR/cert.pem" \
        -days 365 -nodes -subj "/CN=gpg-attest.org" -addext "subjectAltName=DNS:gpg-attest.org,DNS:localhost,IP:127.0.0.1" \
        2>>"$LOG_DIR/startup.log"
fi

# --- Caddy reverse proxy ---
if ! nc -z localhost "$HTTPS_PORT" 2>/dev/null; then
    log "Starting Caddy reverse proxy on :$HTTPS_PORT..."
    caddy start \
        --config /workspace/.devcontainer/Caddyfile \
        --adapter caddyfile \
        >"$LOG_DIR/caddy.log" 2>&1
fi

for i in $(seq 1 10); do
    nc -z localhost "$HTTPS_PORT" 2>/dev/null && { log "Caddy ready on :$HTTPS_PORT"; exit 0; }
    sleep 1
done
log "ERROR: Caddy did not become ready on :$HTTPS_PORT within 10s"
exit 1
