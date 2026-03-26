#!/usr/bin/env bash
# Starts the full local gpg-attest stack: MySQL → Redis → Trillian → gpg-attest-server.
# Idempotent: skips any service that is already listening on its port.
# Logs go to ~/.gpg-attest/logs/.
set -euo pipefail

# --- Devcontainer-only internals (not user-tunable) ---
TRILLIAN_HTTP_PORT=8092
TRILLIAN_SIGNER_RPC_PORT=8093
TRILLIAN_SIGNER_HTTP_PORT=8094
LOG_DIR=$HOME/.gpg-attest/logs
TREE_ID_FILE=$HOME/.gpg-attest/tree_id
KEY_FILE=$HOME/.gpg-attest/server.key
WORKSPACE=${WORKSPACE:-/workspace}

# --- Source .env if present (user overrides) ---
if [ -f "$WORKSPACE/.env" ]; then
    # shellcheck disable=SC1091
    set -a
    # shellcheck source=/dev/null
    source "$WORKSPACE/.env"
    set +a
fi

# --- Defaults for .env variables ---
SERVER_PORT=${SERVER_PORT:-8081}
SERVER_HOST=${SERVER_HOST:-localhost}
HTTPS_PORT=${HTTPS_PORT:-8443}
TRILLIAN_HOST=${TRILLIAN_HOST:-localhost}
TRILLIAN_RPC_PORT=${TRILLIAN_RPC_PORT:-8090}
REDIS_HOST=${REDIS_HOST:-localhost}
REDIS_PORT=${REDIS_PORT:-6379}
MYSQL_HOST=${MYSQL_HOST:-localhost}
MYSQL_PORT=${MYSQL_PORT:-3306}
MYSQL_USER=${MYSQL_USER:-trillian}
MYSQL_PASSWORD=${MYSQL_PASSWORD:-trillian}
MYSQL_DB=${MYSQL_DB:-trillian}

# --- Derived values ---
MYSQL_DSN="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DB}"
SERVER_BIN=$WORKSPACE/server/build/gpg-attest-server

mkdir -p "$LOG_DIR"

log() { echo "[start-services] $*" | tee -a "$LOG_DIR/startup.log"; }

wait_tcp() {
    local name=$1 host=$2 port=$3 secs=${4:-30}
    log "Waiting for $name on $host:$port..."
    for i in $(seq 1 "$secs"); do
        nc -z "$host" "$port" 2>/dev/null && { log "$name ready"; return 0; }
        sleep 1
    done
    log "ERROR: $name did not become ready on $host:$port within ${secs}s"
    log "       Check $LOG_DIR/$name.log for details"
    return 1
}

# --- MySQL ---
if ! nc -z "$MYSQL_HOST" "$MYSQL_PORT" 2>/dev/null; then
    log "Starting MySQL..."
    sudo /usr/sbin/service mariadb start
fi
wait_tcp mysql "$MYSQL_HOST" "$MYSQL_PORT"

# Initialise Trillian database and schema (idempotent)
sudo /usr/local/bin/init-trillian-db.sh

# --- Redis ---
if ! nc -z "$REDIS_HOST" "$REDIS_PORT" 2>/dev/null; then
    log "Starting Redis..."
    redis-server --daemonize yes \
        --logfile "$LOG_DIR/redis.log" \
        --port "$REDIS_PORT"
fi
wait_tcp redis "$REDIS_HOST" "$REDIS_PORT"

# --- Trillian log server ---
if ! nc -z "$TRILLIAN_HOST" "$TRILLIAN_RPC_PORT" 2>/dev/null; then
    log "Starting Trillian log server..."
    trillian_log_server \
        --mysql_uri="$MYSQL_DSN" \
        --rpc_endpoint="${TRILLIAN_HOST}:$TRILLIAN_RPC_PORT" \
        --http_endpoint="${TRILLIAN_HOST}:$TRILLIAN_HTTP_PORT" \
        >"$LOG_DIR/trillian-log-server.log" 2>&1 &
fi
wait_tcp trillian-log-server "$TRILLIAN_HOST" "$TRILLIAN_RPC_PORT"

# --- Trillian log signer ---
if ! pgrep -f trillian_log_signer >/dev/null 2>&1; then
    log "Starting Trillian log signer..."
    trillian_log_signer \
        --mysql_uri="$MYSQL_DSN" \
        --rpc_endpoint="${TRILLIAN_HOST}:$TRILLIAN_SIGNER_RPC_PORT" \
        --http_endpoint="${TRILLIAN_HOST}:$TRILLIAN_SIGNER_HTTP_PORT" \
        --force_master \
        >"$LOG_DIR/trillian-log-signer.log" 2>&1 &
    # Allow signer time to elect itself master before we create a tree
    sleep 3
fi

# --- Trillian tree (created once, ID persisted across restarts) ---
if [ ! -f "$TREE_ID_FILE" ]; then
    log "Creating Trillian tree..."
    TREE_ID=$(createtree --admin_server="${TRILLIAN_HOST}:${TRILLIAN_RPC_PORT}" 2>/dev/null)
    echo "$TREE_ID" > "$TREE_ID_FILE"
    log "Tree ID: $TREE_ID"
fi
TREE_ID=$(cat "$TREE_ID_FILE")

# --- gpg-attest-server ---
if ! nc -z "$SERVER_HOST" "$SERVER_PORT" 2>/dev/null; then
    log "Building gpg-attest-server..."
    (cd "$WORKSPACE/server" && make build) >>"$LOG_DIR/startup.log" 2>&1

    log "Starting gpg-attest-server on :$SERVER_PORT (tree $TREE_ID)..."
    "$SERVER_BIN" \
        --trillian="${TRILLIAN_HOST}:${TRILLIAN_RPC_PORT}" \
        --redis="${REDIS_HOST}:${REDIS_PORT}" \
        --tree-id="$TREE_ID" \
        --key="$KEY_FILE" \
        --addr=":$SERVER_PORT" \
        >"$LOG_DIR/gpg-attest-server.log" 2>&1 &
fi
wait_tcp gpg-attest-server "$SERVER_HOST" "$SERVER_PORT"

# --- Caddy TLS cert (self-signed, generated once) ---
CADDY_CERT_DIR=$HOME/.gpg-attest/caddy
if [ ! -f "$CADDY_CERT_DIR/cert.pem" ]; then
    log "Generating self-signed TLS certificate..."
    mkdir -p "$CADDY_CERT_DIR"
    openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 \
        -keyout "$CADDY_CERT_DIR/key.pem" -out "$CADDY_CERT_DIR/cert.pem" \
        -days 365 -nodes -subj "/CN=gpg-attest.org" -addext "subjectAltName=DNS:gpg-attest.org,DNS:localhost,IP:127.0.0.1" \
        2>>"$LOG_DIR/startup.log"
fi

# --- Caddy reverse proxy (HTTPS) ---
if ! nc -z "$SERVER_HOST" "$HTTPS_PORT" 2>/dev/null; then
    log "Starting Caddy reverse proxy on :$HTTPS_PORT..."
    caddy start \
        --config /workspace/.devcontainer/Caddyfile \
        --adapter caddyfile \
        >"$LOG_DIR/caddy.log" 2>&1
fi
wait_tcp caddy "$SERVER_HOST" "$HTTPS_PORT"

log "gpg-attest stack is up — http://${SERVER_HOST}:${SERVER_PORT} | https://${SERVER_HOST}:${HTTPS_PORT}"
