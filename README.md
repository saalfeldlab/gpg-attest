**gpg-attest** is a browser extension + native messaging host + transparency log server that lets users attach GnuPG-signed attestations to digital content (identified by SHA-256) and record them on an append-only log. Anyone can query the log by artifact hash and evaluate attestations against their own trust model. The log timestamps each entry with its own signature so that attestations cannot be back-dated after key revocation.

- **`extension/`** — [attestension](extension/): WebExtensions Manifest V3 browser extension (Chrome, Firefox). Backend-agnostic attestation client — right-click images to attest them; query any configured log for existing attestations. Works with gpg-attest-server, and can be extended to work with other attestation services like EAS, Rekor, etc.
- **`client/`** — gpg-attest: Native messaging host (Go). Bridges the browser to the local `gpg` binary; private keys never leave the GPG keyring.
- **`server/`** — gpg-attest-server: Transparency log server (Go). Stores entries in a Trillian Merkle tree, indexes by artifact hash via Redis, signs each entry with an Ed25519 key.

The server replicates functionality provided by [Rekor](https://docs.sigstore.dev/logging/overview/) and uses the same underlying technology. We would prefer to use Rekor as is, but Rekor's current public API is focused on incurring some friction and opinions at the signing stage that is useful for software distribution, but do not apply to verdicts on arbitrary content. E.g. we want to deposit any signature on any content without verification, because verification needs to happen on the user end. This is how it's currently implemented, no data filtering, no limits, this server will not survive high frequency world wide usage or targeted DoS attacks, and will require some work to get this done.

## Build

### Prerequisites

| Component   | Requirement                       |
| ----------- | --------------------------------- |
| Extension   | No build step — plain JS/HTML/CSS |
| Native host | Go ≥ 1.19, GNU Make               |
| Server      | Go ≥ 1.23, GNU Make               |

### Extension (attestension)

No build step. Load from the `extension/` directory directly (see [Installation](#installation)).

### Native host (gpg-attest)

```bash
cd client
make build          # host platform binary → build/gpg-attest
make cross          # all platforms (linux/darwin/windows × amd64/arm64)
make test           # run test suite
```

### Server (gpg-attest-server)

```bash
cd server
make build          # → build/gpg-attest-server
```

The devcontainer starts all backing services (MariaDB, Trillian, Redis) and the server automatically on container start. To start manually:

```bash
/workspace/.devcontainer/start-services.sh
```

## Installation

### Extension (attestension)

**Firefox**

1. Go to `about:debugging` → **This Firefox** → **Load Temporary Add-on…**
2. Select `extension/manifest.json`

**Chrome / Chromium**

1. Go to `chrome://extensions` → enable **Developer mode**
2. Click **Load unpacked** → select the `extension/` directory

### Native host (gpg-attest)

The native host must be installed so the browser can launch it via native messaging.

**Linux**

```bash
cd client && make install
# Uninstall:
make uninstall-linux
```

Installs to `~/.local/bin/gpg-attest` and writes manifests for Firefox and Chromium/Chrome.

**macOS (user-level)**

```bash
cd client && make install-macos
# Uninstall:
make uninstall-macos
```

Installs to `~/Library/Application Support/gpg-attest/gpg-attest`, ad-hoc code-signs the binary (required by Chrome), and writes manifests for Firefox, Chrome, and Chromium.

**macOS (system-wide .pkg)**

```bash
cd client && make pkg            # produces build/gpg-attest.pkg
sudo installer -pkg build/gpg-attest.pkg -target /
```

Installs to `/usr/local/bin/gpg-attest` with system-level manifests.

**Windows (PowerShell)**

```powershell
# Build Windows binary first (from Linux/macOS, or natively):
cd client && make cross

# Install:
powershell -ExecutionPolicy Bypass -File scripts\install-windows.ps1 .\build\gpg-attest-windows-amd64.exe

# Uninstall:
powershell -ExecutionPolicy Bypass -File scripts\uninstall-windows.ps1
```

Installs to `%LOCALAPPDATA%\gpg-attest\` and registers manifest paths in `HKCU` for Firefox, Chrome, Edge, and Brave.

### Server (gpg-attest-server)

The server is started automatically in the devcontainer. To install and run manually:

```bash
cd server && make install        # installs to ~/.local/bin/gpg-attest-server
```

Verify it is running:

```bash
curl http://localhost:8081/api/v1/loginfo
```

Required flags: `--key <path>` (Ed25519 PEM key, created if absent), `--tree-id <id>` (Trillian tree ID written to `~/.gpg-attest/tree_id` on first devcontainer start).
