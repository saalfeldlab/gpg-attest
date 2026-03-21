# gpg-attest

A browser extension + native messaging host + transparency log server for attaching GnuPG-signed attestations to digital content and recording them on an append-only log.

- **attestension** (`extension/`) — Backend-agnostic WebExtensions browser extension (Chrome, Firefox)
- **gpg-attest** (`client/`) — Native messaging host; bridges the browser to `gpg`
- **gpg-attest-server** (`server/`) — Transparency log server backed by Trillian

## Purpose

Digital content (images, text, video) is distributed and replicated across platforms with no reliable way to verify its veracity, origin, or meaning. gpg-attest allows actors to attach signed attestations to content pieces, identified by their SHA-256 hash. Anyone can query the transparency log for attestations about a piece of content, then evaluate those attestations against their own trust model — PGP web of trust, TOFU, a curated keyring, or any other scheme.

The log is a **discovery and timestamping mechanism**, not a trust authority. It records "key K asserted verdict V about content H at time T." Verifying whether to believe that verdict is the client's responsibility.

## Architecture

```
content → attestension (JS) → gpg-attest → gpg → signed attestation → attestension → gpg-attest-server
```

- **attestension**: Captures content, provides UI for key and server selection, submits signed attestations; backend-agnostic (works with gpg-attest-server, EAS, or others)
- **gpg-attest**: Minimal program that calls `gpg` for signing; keeps private keys in gpg's control
- **gpg-attest-server**: Custom transparency log — stores attestations, indexes by artifact hash, timestamps entries with its own key

Signing is intentionally done in the native helper (not OpenPGP.js) so that:

- Private keys never transit through the browser
- `gpg-agent` handles passphrase prompting/caching
- Hardware tokens (YubiKey, etc.) work transparently

## Components

- `extension/` - **attestension** — WebExtensions-compatible browser extension (JS/HTML/CSS, Manifest V3)
- `client/` - **gpg-attest** — Native messaging host (signs content via `gpg`, lists available keys)
- `server/` - **gpg-attest-server** — Custom transparency log server (Go, backed by Trillian)

## Native Messaging Registration

- Firefox: `~/.mozilla/native-messaging-hosts/org.gpg_attest.client.json` — uses `allowed_extensions`
- Chrome: `~/.config/google-chrome/NativeMessagingHosts/org.gpg_attest.client.json` — uses `allowed_origins`

## Transparency Log Server (and why we don't just use Rekor)

The custom log server replaces Rekor. It is deliberately minimal: it does **not** verify
submitted signatures. That is the client's responsibility. The server's role is:

1. **Append-only storage** — entries are never modified or deleted (Trillian Merkle tree)
2. **Authoritative timestamps** — the server signs each entry with its own key, preventing
   back-dating. A signer whose PGP key is later revoked cannot fabricate past verdicts.
3. **Hash-indexed lookup** — entries are queryable by artifact SHA-256
4. **Public key transparency** — the server publishes its own public key so clients can verify
   that a timestamp was genuinely issued by the log

### Why not Rekor?

Rekor enforces server-side signature verification and only accepts x509/PKIX public keys
(the Sigstore/Fulcio ecosystem). This project uses PGP web-of-trust as its trust model, which
is incompatible with Rekor's identity assumptions. Rekor also requires uploading full artifact
content for hash-indexed lookup, which is impractical for large digital content. The custom server avoids
all of these mismatches.

### API

| Method | Path                                | Description                                      |
| ------ | ----------------------------------- | ------------------------------------------------ |
| `POST` | `/api/v1/entries`                   | Submit a verdict entry                           |
| `GET`  | `/api/v1/entries?hash=sha256:<hex>` | Retrieve all entries for an artifact hash        |
| `GET`  | `/api/v1/entries/<uuid>`            | Retrieve a single entry by UUID                  |
| `GET`  | `/api/v1/publickey`                 | Server's public key (for timestamp verification) |
| `GET`  | `/api/v1/loginfo`                   | Current tree size and root hash                  |

### Entry format (POST /api/v1/entries)

```json
{
  "artifact_hash": "sha256:<hex>",
  "verdict": "trusted",
  "signer_keyid": "<pgp key fingerprint>",
  "signature": "<base64-encoded PGP detached signature>"
}
```

The server adds `uuid`, `log_index`, `server_timestamp`, and `server_signature` to the stored
entry and returns them in the response.

The signer signs the canonical JSON serialisation of the four fields above (keys sorted, no
extra whitespace). Any verifier can reconstruct the signed payload from the entry fields and
check the PGP signature against the claimed `signer_keyid` — no separate `payload` field is
needed.

### Timestamp security model

The server timestamp prevents back-dating after key revocation:

- Alice signs verdicts throughout 2025; the log timestamps each one at submission time.
- In 2026, Alice's key is revoked because she became unreliable.
- Her verdicts from 2025 remain valid — the server's timestamps prove they predate the revocation.
- Alice cannot fabricate a new verdict and claim it was signed in 2025, because the server would
  timestamp it in 2026, after the revocation.

### Infrastructure (devcontainer)

The devcontainer runs the backing services automatically on every container start.

| Service               | Purpose                              | Port     |
| --------------------- | ------------------------------------ | -------- |
| MariaDB               | Trillian backing store               | 3306     |
| Trillian log server   | Append-only Merkle tree (gRPC)       | 8090     |
| Trillian log signer   | Batches and signs tree heads         | —        |
| Redis                 | Search index (artifact hash → UUIDs) | 6379     |
| **gpg-attest-server** | Custom log API                       | **8081** |

The Trillian tree ID is written to `~/.gpg-attest/tree_id` on first start and reused on subsequent
restarts.

### Automatic startup

`.devcontainer/start-services.sh` is called by `postStartCommand` and is idempotent.

### Verifying the server is up

```bash
curl http://localhost:8081/api/v1/loginfo
```

### Logs

```
~/.gpg-attest/logs/startup.log
~/.gpg-attest/logs/gpg-attest-server.log
~/.gpg-attest/logs/trillian-log-server.log
~/.gpg-attest/logs/trillian-log-signer.log
~/.gpg-attest/logs/redis.log
```

### Manual restart

```bash
/workspace/.devcontainer/start-services.sh
```

### Pointing the extension at the local server

Set `LOG_SERVER=http://localhost:8081` in your `.env` file (copy from `.env.example`).

## Getting Started

```bash
cp .env.example .env
```

## Platform Support

The native messaging host (`client/`) is written in Go and cross-compiles for all targets from a single codebase.

### Native host registration paths

**Linux** (implemented):

- Firefox: `~/.mozilla/native-messaging-hosts/org.gpg_attest.client.json`
- Chromium: `~/.config/chromium/NativeMessagingHosts/org.gpg_attest.client.json`
- Google Chrome: `~/.config/google-chrome/NativeMessagingHosts/org.gpg_attest.client.json`

**macOS** (stub — not yet implemented):

- Firefox: `~/Library/Application Support/Mozilla/NativeMessagingHosts/`
- Chrome: `~/Library/Application Support/Google/Chrome/NativeMessagingHosts/`

**Windows** (stub — not yet implemented):

- Chrome: `HKCU\Software\Google\Chrome\NativeMessagingHosts\org.gpg_attest.client`
- Firefox: `HKCU\Software\Mozilla\NativeMessagingHosts\org.gpg_attest.client`

### Building

All build commands are run from the `client/` directory:

| Platform      | Command                   |
| ------------- | ------------------------- |
| Linux (dev)   | `cd client && make build` |
| Cross-compile | `cd client && make cross` |

Run `cd client && make install` on Linux to build and register for both Firefox and Chromium.

## Testing

Run the full native test suite:

```bash
cd client && go test ./...
```

| Package             | Tests                                                         |
| ------------------- | ------------------------------------------------------------- |
| `internal/gpg`      | Real `ListKeys()` calls against the GPG keystore              |
| `internal/protocol` | Framing encode/decode round-trips                             |
| `cmd/gpg-attest`    | Handler validation (no GPG) + real `list_keys` and `sign` ops |

### Devcontainer test key

The GPG tests require the devcontainer test key to be present:

- **UID**: `Test Signer <test@gpg-attest.org>`
- **Fingerprint**: regenerated each time the devcontainer is created — check with `gpg --list-keys test@gpg-attest.org`
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
2. Click **Select signing key…** → key dialog appears → select `Test Signer <test@gpg-attest.org>` → **Save**
3. Right-click the image again → click **Sign ✓**
4. The Console prints the sha256 hash and PGP signature

#### Reloading after code changes

After editing extension JS/CSS, click **Reload** on the `about:debugging` page.
After editing the native host, re-run `cd /workspace/client && make install` and reload
the extension.

## # Digital Content Belief Scale (DCBS)

A five-level classification for assessing the authenticity, accuracy, and correctness of digital content. Modeled on PGP's trust levels (1–5).

---

## Digital Content Belief Scale (DCBS)

### The Scale

| Level | Label         | Summary                                                                                                |
| ----- | ------------- | ------------------------------------------------------------------------------------------------------ |
| 1     | **False**     | Contradicted by evidence; fabricated, manipulated, or demonstrably wrong.                              |
| 2     | **Suspect**   | Uncorroborated; significant red flags; origin untraceable or anonymous.                                |
| 3     | **Plausible** | Consistent with known facts; partially corroborated; source recognizable but unverified.               |
| 4     | **Trusted**   | Multiple independent credible sources; valid signatures; identifiable origin.                          |
| 5     | **Verified**  | Cryptographic proof of origin and integrity; independently confirmed by authoritative primary sources. |

---

### Assessment Dimensions

Two independent dimensions determine the overall level:

- **Provenance Integrity** — Is the content authentically from its claimed source? (signatures, metadata, chain of custody)
- **Factual Accuracy** — Is what it states true? (cross-referencing, independent confirmation)

The overall level reflects the **lower** of the two scores.

---

### Decision Criteria by Level

#### Level 1 — False

- Contradicted by multiple authoritative sources
- Provenance analysis reveals manipulation (deepfakes, doctored metadata, forgery)
- Fails logical or factual consistency
- **Action:** Reject. Do not rely on for any purpose.

#### Level 2 — Suspect

- Anonymous or untraceable origin
- No cryptographic signature
- Single uncorroborated source; extraordinary claims
- Not provably false, but insufficient evidence for reliability
- **Action:** Investigate further. Do not use for decisions.

#### Level 3 — Plausible

- Source recognizable but not fully verified
- Partial corroboration (one independent source, or metadata checks pass)
- Minor inconsistencies or gaps may exist
- **Action:** Use provisionally with caveats. Suitable for working hypotheses.

#### Level 4 — Trusted

- Multiple independent, credible sources confirm
- Origin identifiable and reputable
- Technical integrity checks pass (valid digital signatures, intact metadata, consistent timestamps)
- Remaining uncertainty narrow and bounded
- **Action:** Suitable for informed decision-making.

#### Level 5 — Verified

- End-to-end cryptographic proof of origin and integrity
- Fully validated chain of trust
- Factual claims independently confirmed by authoritative primary sources
- Provenance transparent and auditable
- **Action:** Treat as ground truth within the trust model.

---

### Quick-Reference Summary

```
1  False      ██░░░░░░░░  Reject
2  Suspect    ████░░░░░░  Investigate
3  Plausible  ██████░░░░  Use with caveats
4  Trusted    ████████░░  Decide on
5  Verified   ██████████  Ground truth
```

---

### Badge Icons

SVG source and pre-rendered PNGs live in `extension/icons/dcbs/`. Each icon is a 32×32 filled
circle with a 1 px white stroke and a white glyph:

| File prefix        | Color  | Hex       | Glyph |
| ------------------ | ------ | --------- | ----- |
| `dcbs-1-false`     | Red    | `#C62828` | ✘     |
| `dcbs-2-suspect`   | Orange | `#E65100` | ?     |
| `dcbs-3-plausible` | Grey   | `#757575` | ~     |
| `dcbs-4-trusted`   | Teal   | `#00796B` | ✓     |
| `dcbs-5-verified`  | Indigo | `#283593` | ★     |

PNGs are generated from the SVGs at sizes 16, 24, 32, 64, and 128 px using `rsvg-convert`:

```bash
for svg in extension/icons/dcbs/dcbs-*.svg; do
  base="${svg%.svg}"
  for size in 16 24 32 64 128; do
    rsvg-convert -w $size -h $size "$svg" -o "${base}-${size}.png"
  done
done
```
