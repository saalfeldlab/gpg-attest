#!/bin/bash
set -euo pipefail

GNUPG_HOME="${GNUPGHOME:-$HOME/.gnupg}"

# Returns true if any key in the keyring has a UID that is NOT test@gpg-attest.org
has_foreign_keys() {
    local foreign
    foreign=$(gpg --list-keys --with-colons 2>/dev/null \
        | awk -F: '$1=="uid" && $10 !~ /test@gpg-attest\.org/ {count++} END {print count+0}')
    [ "$foreign" -gt 0 ]
}

has_test_key() {
    gpg --list-keys test@gpg-attest.org >/dev/null 2>&1
}

reset_gpg() {
    # Kill relay/existing agent
    gpgconf --kill gpg-agent 2>/dev/null || true

    # Remove all relay sockets and key material
    rm -f  "$GNUPG_HOME/S.gpg-agent" \
           "$GNUPG_HOME/S.gpg-agent.browser" \
           "$GNUPG_HOME/S.gpg-agent.extra" \
           "$GNUPG_HOME/S.gpg-agent.ssh"
    rm -f  "$GNUPG_HOME/pubring.kbx" \
           "$GNUPG_HOME/pubring.kbx~" \
           "$GNUPG_HOME/trustdb.gpg"
    rm -rf "$GNUPG_HOME/private-keys-v1.d" \
           "$GNUPG_HOME/openpgp-revocs.d"
    mkdir -p "$GNUPG_HOME/private-keys-v1.d" \
             "$GNUPG_HOME/openpgp-revocs.d"
    chmod 700 "$GNUPG_HOME" \
              "$GNUPG_HOME/private-keys-v1.d" \
              "$GNUPG_HOME/openpgp-revocs.d"

    # Start fresh container-local agent
    gpg-agent --daemon --allow-loopback-pinentry 2>/dev/null || true
}

create_test_key() {
    gpg --pinentry-mode loopback --passphrase '' --batch --gen-key <<'EOF'
Key-Type: RSA
Key-Length: 4096
Subkey-Type: RSA
Subkey-Length: 4096
Name-Real: Test Signer
Name-Email: test@gpg-attest.org
Expire-Date: 2y
%commit
EOF
    echo "Test GPG key created:"
    gpg --list-keys test@gpg-attest.org
}

if has_foreign_keys; then
    echo "gpg-init: foreign keys detected — resetting keyring and replacing relay agent"
    reset_gpg
    create_test_key
elif ! has_test_key; then
    echo "gpg-init: test key missing — initialising clean keyring"
    reset_gpg
    create_test_key
else
    echo "gpg-init: keyring clean, nothing to do"
fi
