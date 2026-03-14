#!/usr/bin/env bash
# Starts the full local Rekor stack: MySQL → Redis → Trillian → rekor-server.
# Idempotent: skips any service that is already listening on its port.
# Logs go to ~/.rekor/logs/.
set -euo pipefail

REKOR_PORT=8081
TRILLIAN_RPC_PORT=8090
TRILLIAN_HTTP_PORT=8092
TRILLIAN_SIGNER_HTTP_PORT=8094
MYSQL_DSN="trillian:trillian@tcp(127.0.0.1:3306)/trillian"
REDIS_PORT=6379
LOG_DIR=$HOME/.rekor/logs
TREE_ID_FILE=$HOME/.rekor/tree_id

mkdir -p "$LOG_DIR"

log() { echo "[start-rekor] $*" | tee -a "$LOG_DIR/startup.log"; }

wait_tcp() {
    local name=$1 port=$2 secs=${3:-30}
    log "Waiting for $name on :$port..."
    for i in $(seq 1 "$secs"); do
        nc -z 127.0.0.1 "$port" 2>/dev/null && { log "$name ready"; return 0; }
        sleep 1
    done
    log "ERROR: $name did not become ready on :$port within ${secs}s"
    log "       Check $LOG_DIR/$name.log for details"
    return 1
}

# --- MySQL ---
if ! nc -z 127.0.0.1 3306 2>/dev/null; then
    log "Starting MySQL..."
    sudo /usr/sbin/service mariadb start
fi
wait_tcp mysql 3306

# Initialise Trillian database and schema (idempotent)
sudo /usr/local/bin/init-trillian-db.sh

# --- Redis ---
if ! nc -z 127.0.0.1 "$REDIS_PORT" 2>/dev/null; then
    log "Starting Redis..."
    redis-server --daemonize yes \
        --logfile "$LOG_DIR/redis.log" \
        --port "$REDIS_PORT"
fi
wait_tcp redis "$REDIS_PORT"

# --- Trillian log server ---
if ! nc -z 127.0.0.1 "$TRILLIAN_RPC_PORT" 2>/dev/null; then
    log "Starting Trillian log server..."
    trillian_log_server \
        --mysql_uri="$MYSQL_DSN" \
        --rpc_endpoint="127.0.0.1:$TRILLIAN_RPC_PORT" \
        --http_endpoint="127.0.0.1:$TRILLIAN_HTTP_PORT" \
        >"$LOG_DIR/trillian-log-server.log" 2>&1 &
fi
wait_tcp trillian-log-server "$TRILLIAN_RPC_PORT"

# --- Trillian log signer ---
if ! pgrep -f trillian_log_signer >/dev/null 2>&1; then
    log "Starting Trillian log signer..."
    trillian_log_signer \
        --mysql_uri="$MYSQL_DSN" \
        --rpc_endpoint="127.0.0.1:8093" \
        --http_endpoint="127.0.0.1:$TRILLIAN_SIGNER_HTTP_PORT" \
        --force_master \
        >"$LOG_DIR/trillian-log-signer.log" 2>&1 &
    # Allow signer time to elect itself master before we create a tree
    sleep 3
fi

# --- Trillian tree (created once, ID persisted across restarts) ---
if [ ! -f "$TREE_ID_FILE" ]; then
    log "Creating Trillian tree..."
    TREE_ID=$(createtree --admin_server="127.0.0.1:$TRILLIAN_RPC_PORT" 2>/dev/null)
    echo "$TREE_ID" > "$TREE_ID_FILE"
    log "Tree ID: $TREE_ID"
fi
TREE_ID=$(cat "$TREE_ID_FILE")

# --- rekor-server ---
if ! nc -z 127.0.0.1 "$REKOR_PORT" 2>/dev/null; then
    log "Starting rekor-server on :$REKOR_PORT (tree $TREE_ID)..."
    rekor-server serve \
        --host=127.0.0.1 \
        --port="$REKOR_PORT" \
        --rekor_server.signer=memory \
        --trillian_log_server.address=127.0.0.1 \
        --trillian_log_server.port="$TRILLIAN_RPC_PORT" \
        --trillian_log_server.tlog_id="$TREE_ID" \
        --redis_server.address=127.0.0.1 \
        --redis_server.port="$REDIS_PORT" \
        --log_type=prod \
        >"$LOG_DIR/rekor-server.log" 2>&1 &
fi
wait_tcp rekor-server "$REKOR_PORT"

log "Rekor stack is up — http://127.0.0.1:$REKOR_PORT"