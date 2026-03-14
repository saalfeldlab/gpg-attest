# sig-stuff

A browser extension (Chrome + Firefox) that signs arbitrary web content using the user's existing GnuPG keystore and posts the signed result to a Rekor transparency log server.

## Architecture

```
Extension (JS) → Native Messaging → Native Helper → gpg → signature → Extension → Rekor API
```

- **Browser extension**: Captures content, provides UI for key selection, submits signed entries to Rekor
- **Native helper**: Minimal program that calls `gpg` for signing; keeps private keys in gpg's control

Signing is intentionally done in the native helper (not OpenPGP.js) so that:
- Private keys never transit through the browser
- `gpg-agent` handles passphrase prompting/caching
- Hardware tokens (YubiKey, etc.) work transparently

## Components

- `extension/` - WebExtensions-compatible browser extension (JS/HTML/CSS, Manifest V3)
- `native/` - Native messaging host (signs content via `gpg`, lists available keys)

## Native Messaging Registration

- Firefox: `~/.mozilla/native-messaging-hosts/` — uses `allowed_extensions`
- Chrome: `~/.config/google-chrome/NativeMessagingHosts/` — uses `allowed_origins`

## Rekor Integration

Uses the `rekord` entry type with PGP. The extension handles all Rekor API calls via `fetch()`.

## Local Rekor Server

The devcontainer runs a full local Rekor transparency log stack automatically on every container
start. This enables offline development and testing without hitting the public Rekor instance.

### Stack

| Service | Purpose | Port |
|---|---|---|
| MariaDB | Trillian backing store | 3306 |
| Trillian log server | Append-only Merkle tree (gRPC) | 8090 |
| Trillian log signer | Batches and signs tree heads | — |
| Redis | rekor search index | 6379 |
| rekor-server | Rekor API | **8081** |

The Trillian tree ID is written to `~/.rekor/tree_id` on first start and reused on subsequent
restarts. Rebuilding the devcontainer image resets the log.

### Automatic startup

`.devcontainer/start-rekor.sh` is called by `postStartCommand` and is idempotent — it skips
any service that is already running. Startup takes ~15 seconds on first run (waiting for each
service to become ready).

### Verifying the server is up

```bash
rekor-cli --rekor_server http://localhost:8081 loginfo
```

### Logs

All service logs are written to `~/.rekor/logs/`:

```
~/.rekor/logs/startup.log            # start-rekor.sh output
~/.rekor/logs/rekor-server.log
~/.rekor/logs/trillian-log-server.log
~/.rekor/logs/trillian-log-signer.log
~/.rekor/logs/redis.log
```

### Manual restart

If the stack is not running (e.g. after a crash), restart it with:

```bash
/workspace/.devcontainer/start-rekor.sh
```

### Pointing the extension at the local server

Set `REKOR_SERVER=http://localhost:8081` in your `.env` file (copy from `.env.example`).

## Getting Started

```bash
cp .env.example .env
```

## Platform Support

The native messaging host (`native/`) is written in Go and cross-compiles for all targets from a single codebase.

### Native host registration paths

**Linux** (implemented):
- Firefox: `~/.mozilla/native-messaging-hosts/dev.sig_stuff.native.json`
- Chromium: `~/.config/chromium/NativeMessagingHosts/dev.sig_stuff.native.json`
- Google Chrome: `~/.config/google-chrome/NativeMessagingHosts/dev.sig_stuff.native.json`

**macOS** (stub — not yet implemented):
- Firefox: `~/Library/Application Support/Mozilla/NativeMessagingHosts/`
- Chrome: `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`

**Windows** (stub — not yet implemented):
- Chrome: `HKCU\Software\Google\Chrome\NativeMessagingHosts\dev.sig_stuff.native`
- Firefox: `HKCU\Software\Mozilla\NativeMessagingHosts\dev.sig_stuff.native`

### Building

All build commands are run from the `native/` directory:

| Platform       | Command                                              |
|----------------|------------------------------------------------------|
| Linux (dev)    | `cd native && make build`                            |
| Cross-compile  | `cd native && make cross`                            |

Run `cd native && make install` on Linux to build and register for both Firefox and Chromium.

## Testing

Run the full native test suite:

```bash
cd native && go test ./...
```

| Package | Tests |
|---|---|
| `internal/gpg` | Real `ListKeys()` calls against the GPG keystore |
| `internal/protocol` | Framing encode/decode round-trips |
| `cmd/sig-stuff-native` | Handler validation (no GPG) + real `list_keys` and `sign` ops |

### Devcontainer test key

The GPG tests require the devcontainer test key to be present:

- **UID**: `Test Signer <test@sig-stuff.dev>`
- **Fingerprint**: regenerated each time the devcontainer is created — check with `gpg --list-keys test@sig-stuff.dev`
- **Passphrase**: none

The key is provisioned automatically by the devcontainer. Tests that exercise GPG will fail
if this key is absent.

### GPG keyring isolation

VS Code's Dev Containers extension injects host GPG keys and relay sockets into the container
at attach time (`pubring.kbx`, `private-keys-v1.d/*.key`, and four `S.gpg-agent*` sockets).
For a signing project this is a risk: `gpg --sign` could operate on host private keys, and
`S.gpg-agent.ssh` exposes host SSH keys.

`.devcontainer/gpg-init.sh` runs at all three lifecycle hooks (`postCreateCommand`,
`postStartCommand`, `postAttachCommand`) to defend against this. On each attach it:

1. Checks whether any non-test key is present in the keyring
2. If yes (or if the test key is missing): kills the relay agent, wipes all injected key
   material and relay sockets, starts a fresh container-local `gpg-agent`, creates the test key
3. Otherwise: no-op

The `postAttachCommand` hook is the critical one — it fires after VS Code has fully attached
and injected keys, so it always runs last.

### Manual Testing

A complete end-to-end test using Firefox and the local test page:

#### 1. Build and install the native host

```bash
cd /workspace/native && make install
```

This compiles the Go binary and writes the native messaging manifests for Firefox
(`~/.mozilla/native-messaging-hosts/`) and Chromium (`~/.config/chromium/NativeMessagingHosts/`).

#### 2. Start the test page HTTP server

The service worker uses `fetch()`, which is blocked on `file://` origins.

```bash
cd /workspace/testpage && python3 -m http.server 8080
```

Leave this running in a separate terminal.

#### 3. Open Firefox and load the extension

```bash
firefox &
```

In Firefox:
1. Go to `about:debugging`
2. Click **This Firefox** in the left sidebar
3. Click **Load Temporary Add-on…**
4. Navigate to `/workspace/extension/` and select `manifest.json`

The extension is now active for this browser session. It must be reloaded after each
Firefox restart (temporary add-on limitation).

#### 4. Open the test page

Navigate to `http://localhost:8080` in Firefox.

Open DevTools (F12) and switch to the **Console** tab.

#### 5. Sign an image

1. Right-click any image → browser context menu shows **Sign ✓**, **Sign ✗**, **Select signing key…**
2. Click **Select signing key…** → key dialog appears → select `Test Signer <test@sig-stuff.dev>` → **Save**
3. Right-click the image again → click **Sign ✓**
4. The Console prints the sha256 hash and PGP signature

#### Reloading after code changes

After editing extension JS/CSS, click **Reload** on the `about:debugging` page.
After editing the native host, re-run `cd /workspace/native && make install` and reload
the extension.