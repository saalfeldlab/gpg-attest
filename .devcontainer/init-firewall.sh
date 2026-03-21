#!/bin/bash
set -euo pipefail
IFS=$'\n\t'

# 1. Extract Docker DNS info BEFORE any flushing
DOCKER_DNS_RULES=$(iptables-save -t nat | grep "127\.0\.0\.11" || true)

# Flush existing rules and delete existing ipsets
iptables -F
iptables -X
iptables -t nat -F
iptables -t nat -X
iptables -t mangle -F
iptables -t mangle -X
ipset destroy allowed-domains 2>/dev/null || true

# 2. Selectively restore ONLY internal Docker DNS resolution
if [ -n "$DOCKER_DNS_RULES" ]; then
    echo "Restoring Docker DNS rules..."
    iptables -t nat -N DOCKER_OUTPUT 2>/dev/null || true
    iptables -t nat -N DOCKER_POSTROUTING 2>/dev/null || true
    echo "$DOCKER_DNS_RULES" | xargs -L 1 iptables -t nat
else
    echo "No Docker DNS rules to restore"
fi

# Create ipset with CIDR support
ipset create allowed-domains hash:net

# Collect all domains to resolve
DOMAINS_TO_RESOLVE=()


# === Module: core.conf ===
# Core firewall rules - always included
# DNS, SSH, localhost, OAuth callback

# Allow outbound DNS
iptables -A OUTPUT -p udp --dport 53 -j ACCEPT
# Allow inbound DNS responses
iptables -A INPUT -p udp --sport 53 -j ACCEPT
# Allow outbound SSH
iptables -A OUTPUT -p tcp --dport 22 -j ACCEPT
# Allow inbound SSH responses
iptables -A INPUT -p tcp --sport 22 -m state --state ESTABLISHED -j ACCEPT
# Allow localhost
iptables -A INPUT -i lo -j ACCEPT
iptables -A OUTPUT -o lo -j ACCEPT
# Allow incoming connections on port 3000 for OAuth callback
iptables -A INPUT -p tcp --dport 3000 -j ACCEPT

# === Module: github.conf ===
# GitHub firewall rules - always included
# Allows access to GitHub services

# Fetch GitHub meta information and add their IP ranges
echo "Fetching GitHub IP ranges..."
gh_ranges=$(curl -s https://api.github.com/meta)
if [ -z "$gh_ranges" ]; then
    echo "ERROR: Failed to fetch GitHub IP ranges"
    exit 1
fi

if ! echo "$gh_ranges" | jq -e '.web and .api and .git' >/dev/null; then
    echo "ERROR: GitHub API response missing required fields"
    exit 1
fi

echo "Processing GitHub IPs..."
while read -r cidr; do
    if [[ ! "$cidr" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}/[0-9]{1,2}$ ]]; then
        echo "ERROR: Invalid CIDR range from GitHub meta: $cidr"
        exit 1
    fi
    echo "Adding GitHub range $cidr"
    ipset add allowed-domains "$cidr"
done < <(echo "$gh_ranges" | jq -r '(.web + .api + .git)[]' | aggregate -q)

# Add github.com domain for additional resolution
DOMAINS_TO_RESOLVE+=("github.com")

# === Module: anthropic.conf ===
# Anthropic firewall rules - always included
# Allows access to Anthropic/Claude API services

DOMAINS_TO_RESOLVE+=(
    "api.anthropic.com"
    "statsig.anthropic.com"
    "sentry.io"
    "statsig.com"
)

# === Module: golang.conf ===
# Go module proxy and checksum database
# Allows go build/install to fetch modules at container runtime

DOMAINS_TO_RESOLVE+=(
    "proxy.golang.org"
    "sum.golang.org"
    "storage.googleapis.com"
)

# === Module: package-managers.conf ===
# Package manager firewall rules - always included
# Allows access to npm, PyPI, Conda, and Pixi

DOMAINS_TO_RESOLVE+=(
    "registry.npmjs.org"
    "pypi.org"
    "files.pythonhosted.org"
    "conda.anaconda.org"
    "repo.anaconda.com"
    "conda-forge.org"
    "prefix.dev"
    "conda-mapping.prefix.dev"
    "github.io"
    "www.washingtonpost.com"
    "www.sueddeutsche.de"
)

# === Resolve and add domains ===
for domain in "${DOMAINS_TO_RESOLVE[@]}"; do
    echo "Resolving $domain..."
    ips=$(dig +short A "$domain" | grep -E '^([0-9]{1,3}\.){3}[0-9]{1,3}$')
    if [ -z "$ips" ]; then
        echo "WARNING: Failed to resolve $domain - skipping"
        continue
    fi

    while read -r ip; do
        if [[ ! "$ip" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
            echo "WARNING: Invalid IP from DNS for $domain: $ip"
            continue
        fi
        echo "Adding $ip for $domain"
        ipset add allowed-domains "$ip" 2>/dev/null || true
    done < <(echo "$ips")
done

# Get host IP from default route
HOST_IP=$(ip route | grep default | cut -d" " -f3)
if [ -z "$HOST_IP" ]; then
    echo "ERROR: Failed to detect host IP"
    exit 1
fi

HOST_NETWORK=$(echo "$HOST_IP" | sed "s/\.[0-9]*$/.0\/24/")
echo "Host network detected as: $HOST_NETWORK"

# Set up remaining iptables rules
iptables -A INPUT -s "$HOST_NETWORK" -j ACCEPT
iptables -A OUTPUT -d "$HOST_NETWORK" -j ACCEPT

# Set default policies to DROP first
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT DROP

# First allow established connections for already approved traffic
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# Then allow only specific outbound traffic to allowed domains
iptables -A OUTPUT -m set --match-set allowed-domains dst -j ACCEPT

echo "Firewall configuration complete"
echo "Verifying firewall rules..."
if curl --connect-timeout 5 https://example.com >/dev/null 2>&1; then
    echo "ERROR: Firewall verification failed - was able to reach https://example.com"
    exit 1
else
    echo "Firewall verification passed - unable to reach https://example.com as expected"
fi

# Verify GitHub API access
if ! curl --connect-timeout 5 https://api.github.com/zen >/dev/null 2>&1; then
    echo "ERROR: Firewall verification failed - unable to reach https://api.github.com"
    exit 1
else
    echo "Firewall verification passed - able to reach https://api.github.com as expected"
fi
