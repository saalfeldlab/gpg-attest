# gpg-attest — Decentralized attestations for digital content

**gpg-attest** is a browser extension + native messaging host + transparency log server that lets users attach GnuPG-signed attestations to digital content (identified by SHA-256) and record them on an append-only log. Anyone can query the log by artifact hash and evaluate attestations against their own trust model. The log timestamps each entry with its own signature so that attestations cannot be back-dated after key revocation.

For any media element (currently only images), the extension queries log servers for trusted attestations and displays badges directly over the content — so users can see at a glance what their trust network thinks.

### Why?

AI tools can now generate digital content that is indistinguishable from reality. When content cannot be attributed to a known source, individuals have no way to verify whether what they see is real.

[C2PA](https://c2pa.org/) addresses this by embedding cryptographic signatures in media files. However, it only works for file formats that support sideloaded metadata, and the signatures must be certified by centralized identity providers.

[Sigstore's Rekor](https://www.sigstore.dev/) takes a different approach: it stores signatures of content hashes in an external append-only log, so the signature does not need to travel with the file. This means it cannot be falsified and does not even require that the server is trusted. Rekor's default workflow, however, relies on centralized identity providers like [GitHub](https://github.com/) and [Google](https://google.com/) for signer identity.

gpg-attest combines hash-based lookup and append-only transparency from Rekor with decentralized trust from the PGP web-of-trust — no centralized authorities required.

- **`extension/`** — [attestension](extension/): WebExtensions Manifest V3 browser extension (Chrome, Firefox). Attestation client and display — right-click images to attest them; query any configured log for existing attestations.
- **`client/`** — gpg-attest: Native messaging host (Go). Bridges the browser to the local `gpg` binary; private keys never leave the GPG keyring.
- **`server/`** — gpg-attest-server: Transparency log server (Go). Stores entries in a Trillian Merkle tree, indexes by artifact hash via Redis, signs each entry with its GPG key.

## Verdict Categories

Verdicts use three independent categories, each independently signable and revocable:

| Category         | Type            | Verdicts                              | Meaning                       |
| ---------------- | --------------- | ------------------------------------- | ----------------------------- |
| **Authorship**   | Toggle          | `my-work`                             | "I created this"              |
| **Method**       | Toggle          | `ai-generated`                        | "AI was used to produce this" |
| **Authenticity** | Exclusive scale | `authentic` / `satire` / `misleading` | Deception/intent spectrum     |

A signer can select any combination. No selection in a category means no claim about that dimension. The special verdict `revoke` withdraws a previous claim in a category.

## Transparency log and Rekor

The server covers core functionality targeted by [Rekor](https://docs.sigstore.dev/logging/overview/) and uses the same underlying technology (Trillian). The intended production path would be a Rekor instance extended with a custom `pgp-verdict` entry type that accepts `{artifact_hash, verdict, signer_keyid, pgp_signature}` without server-side verification. Contributing that entry type upstream (or maintaining a minimal fork) would give this project Rekor's battle-tested sharding, indexing, and ops for free. This server exists as a prototype while that work is pending: no data filtering, no signature verification, no limits. As currently implemented, it will not survive high-frequency worldwide usage or targeted DoS attacks.

The browser extension is designed to support additional attestation backends (e.g., EAS, Rekor), contributions welcome.

## This is a prototype

This is an experiment to test how we could create trust in decentralized data without content based means to check for correctness and monopolist "trust guarantors". To that end, it tries to achieve:

1. **Use decades-old established technology:** GPG, PGP web-of-trust, Merkle trees.
2. **Small data footprint:** Sign and query hashes, not full data, don't check or test what's none of your business.
3. **Minimum friction for users:** No complicated dialogs, badges on content, must just work and be super easy.
4. **No centralized authorities:** Signatures hosted on untrusted mirrors are as good as anything, because testing depends on your local trust chain.
5. **Good enough:** Don't solve all problems at once. E.g., trust chains of users believing in nonsense are as valid as serious actors, but you can know and can decide whom to trust. Don't over-engineer, this is not a zero trust system but a useful hint that is hard to falsify, temporary mistakes are expected and can be revoked/updated.

### Known weaknesses

There are likely many, we would love to hear your input!

#### 1. Display of badges over content can be falsified

Here are some ways to do that:

- Providers can fake the badges in the browser by showing images including such badges. Once discovered, these images could be signed as untrusted which would paint over the fake badges.
- This, in return, can be falsified by hosting images with ever changing binary signatures (recompress with random meta data tags), such that no record sticks. This requires self-hosting on a dedicated server, once copied into other sites (e.g. social media), the content becomes static and the signatures will stick.
- A content provider could inject Javascript that tampers with the badges and context menus. This depends on somebody hosting such Javascript and does not transfer to re-posting the content.

#### 2. GPG key handling on device may be considered an inconvenience by those inclined to complain about stuff

We can integrate sensible actions into the browser extension with opinionated defaults that make this extremely easy. E.g. default choices about algorithms, expiration, always publish to standard keyservers. Here are things that users should be able to do without any friction:

- show keys
- make a key
- revoke a key
- trust somebody's key
- revoke trust of somebody's key

#### 3. The server is not robust

We would prefer to build on the hardened Rekor Sigstore stack directly instead of maintaining an adjacent API. What we need is an API adjusted to permit submission of {hash, verdict, signature} without checking anything. The current server is not robust or scalable. A future version could add rate limiting and basic sanity checks (e.g., enforced wait times between submissions) to resist DoS without breaking the no-verification model. Bulk submission APIs for app-level use may need more thought.

## Build

### Prerequisites

| Component   | Requirement                          |
| ----------- | ------------------------------------ |
| Extension   | No build required, plain JS/HTML/CSS |
| Native host | Go ≥ 1.19, GNU Make                  |
| Server      | Go ≥ 1.23, GNU Make                  |

### Extension (attestension)

Package and sign browser extensions with `extension/build.sh`.

For testing, no build required, this is Javascript. Load from the `extension/` directory directly (see [Installation](#installation)).

### Native host (gpg-attest)

```bash
cd client
make build          # host platform binary → build/gpg-attest
make cross          # all platforms (linux/darwin/windows × amd64/arm64)
make deb            # Debian package → build/gpg-attest_<version>_amd64.deb
make test           # run test suite
```

### Server (gpg-attest-server)

```bash
cd server
make build          # → build/gpg-attest-server
```

The devcontainer starts all backing services (MariaDB, Trillian, Redis) and the server automatically on container start and creates GPG test keys. To start manually:

```bash
/workspace/.devcontainer/start-services.sh
```

## Installation

### 1. Browser extension (attestension)

<!-- TODO: replace placeholder URLs once publicly listed -->

**Firefox:** Install from [TODO_FIREFOX_ADDON_URL](about:blank)

**Chrome / Chromium:** Install from [TODO_CHROME_WEBSTORE_URL](about:blank)

For development or testing, see [Loading the extension from source](#loading-the-extension-from-source) below.

### 2. Native host (gpg-attest)

The native host bridges the browser extension to your local GnuPG keyring. Install it after the extension.

**Linux (.deb — Debian, Ubuntu)**

Download the `.deb` from the [latest release](../../releases/latest) and install:

```bash
sudo dpkg -i gpg-attest_<version>_amd64.deb
```

**Linux (.rpm — Fedora, RHEL, openSUSE)**

Download the `.rpm` from the [latest release](../../releases/latest) and install:

```bash
sudo rpm -i gpg-attest-<version>-1.x86_64.rpm
```

**macOS (.pkg)**

Download `gpg-attest-<version>.pkg` from the [latest release](../../releases/latest) and double-click to install, or:

```bash
sudo installer -pkg gpg-attest-<version>.pkg -target /
```

Prerequisite: GnuPG must be installed. The easiest way is via [Homebrew](https://brew.sh/):

```bash
brew install gnupg
```

**Windows**

Download `gpg-attest-windows-amd64.exe` from the [latest release](../../releases/latest).

Prerequisite: [Gpg4win](https://www.gpg4win.org/) must be installed (provides `gpg.exe`).

Place the binary and register it for native messaging using the included PowerShell script:

```powershell
powershell -ExecutionPolicy Bypass -File scripts\install-windows.ps1 .\gpg-attest-windows-amd64.exe

# Uninstall:
powershell -ExecutionPolicy Bypass -File scripts\uninstall-windows.ps1
```

Installs to `%LOCALAPPDATA%\gpg-attest\` and registers manifest paths in `HKCU` for Firefox, Chrome, Edge, and Brave.

### Building from source

#### Native host

```bash
cd client
make build          # host platform binary → build/gpg-attest
make install        # auto-detects OS, installs binary + browser manifests
make cross          # all platforms (linux/darwin/windows × amd64/arm64)
make deb            # Debian package → build/gpg-attest_<version>_amd64.deb
make rpm            # RPM package → build/gpg-attest-<version>-1.x86_64.rpm
make pkg            # macOS package → build/gpg-attest.pkg (requires Xcode CLI tools)
make test           # run test suite
```

User-level install (no package manager):

```bash
# Linux:
cd client && make install
make uninstall-linux

# macOS:
cd client && make install-macos
make uninstall-macos
```

#### Loading the extension from source

**Firefox**

1. Go to `about:debugging` → **This Firefox** → **Load Temporary Add-on…**
2. Select `extension/manifest.json`

**Chrome / Chromium**

1. Go to `chrome://extensions` → enable **Developer mode**
2. Click **Load unpacked** → select the `extension/` directory

### Server (gpg-attest-server)

The server is started automatically in the devcontainer. To install and run manually:

```bash
cd server && make install        # installs to ~/.local/bin/gpg-attest-server
```

Verify it is running:

```bash
curl http://localhost:8081/api/v1/loginfo
```

Required flags: `--gpg-keyid <id>` (GPG key fingerprint, key ID, or email for server timestamp signing), `--tree-id <id>` (Trillian tree ID written to `~/.gpg-attest/tree_id` on first devcontainer start).

### HTTPS reverse proxy (Caddy)

The server listens on plain HTTP (`localhost:8081`). Caddy sits in front of it to provide HTTPS with TLS termination.

**Devcontainer (development)**

Caddy is pre-installed in the devcontainer and starts automatically on container start. No setup is needed:

- Listens on `gpg-attest.org:443` and reverse-proxies to `localhost:8081`
- A self-signed TLS certificate is generated on first start (stored in `~/.gpg-attest/caddy/`)
- The container maps `gpg-attest.org` to `127.0.0.1` via `--add-host`, so the domain resolves inside the container
- The extension's default `LOG_SERVER` points to `https://gpg-attest.org`, so it works out of the box
- Browsers will show a certificate warning for the self-signed cert; accept it to proceed
- Override the listen port by setting `HTTPS_PORT` in `.env` (default: `443`)

**Production deployment**

A production Caddyfile template is provided at `server/Caddyfile.production`. Caddy auto-provisions Let's Encrypt TLS certificates for real domains.

Prerequisites:

- DNS A/AAAA record pointing to your server
- Ports 80 (ACME HTTP challenge) and 443 (HTTPS) open in the firewall

```bash
# 1. Edit the Caddyfile — replace yourdomain.example.com with your domain
vi server/Caddyfile.production

# 2. Start Caddy
caddy run --config server/Caddyfile.production --adapter caddyfile
```

Then set `LOG_SERVER=https://yourdomain.example.com` in `.env` so the browser extension points to your server.
