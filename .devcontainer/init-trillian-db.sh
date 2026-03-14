#!/usr/bin/env bash
# Run as root (via sudo) to initialise the Trillian MySQL database.
# Idempotent: exits 0 immediately if the schema is already in place.
set -euo pipefail

if mysql -u trillian -ptrillian trillian -e "SELECT 1 FROM Trees LIMIT 1" >/dev/null 2>&1; then
    exit 0
fi

mysql <<'SQL'
CREATE DATABASE IF NOT EXISTS trillian;
CREATE USER IF NOT EXISTS 'trillian'@'localhost' IDENTIFIED BY 'trillian';
GRANT ALL ON trillian.* TO 'trillian'@'localhost';
FLUSH PRIVILEGES;
SQL

mysql trillian < /usr/local/share/trillian/storage.sql
echo "init-trillian-db: schema applied"
