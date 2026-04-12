# gpg-attest — Developer Reference

This file covers architecture details, server internals, devcontainer infrastructure, and development workflows not in [@README.md](README.md)

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

## Transparency Log Server (and why we don't just use Rekor)

The custom log server replaces Rekor. It is deliberately minimal: it does **not** verify
submitted signatures. That is the client's responsibility. The server enforces input
validation (hash format, field size limits) and rate limiting (global 50 req/s + per-IP
5 req/s) but its role is:

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
  "category": "authenticity",
  "verdict": "authentic",
  "signer_keyid": "<pgp key fingerprint>",
  "signature": "<base64-encoded PGP detached signature>"
}
```

Request body is limited to 100 KB (verdict may evolve into arbitrary nested JSON).
`artifact_hash` must be `algorithm:<hex>` with correct length (currently `sha256:` + 64 hex
chars; add new algorithms to `hashLengths` in `handler.go`). `signature` is capped at 8 KB
(sufficient for all classical and ML-DSA post-quantum signatures). `signer_keyid` is capped
at 256 bytes.

The `category` field specifies which verdict dimension is being attested. Valid
category/verdict combinations:

| Category | Allowed verdicts | Type |
| --- | --- | --- |
| `authorship` | `my-work`, `revoke` | Toggle — "I created this" |
| `method` | `ai-generated`, `revoke` | Toggle — "AI was used" |
| `authenticity` | `authentic`, `satire`, `misleading`, `revoke` | Exclusive scale |

Each category is independently signable. A signer can submit multiple entries for
the same artifact (one per category). The `revoke` verdict withdraws a previous
claim in that category, returning to silence.

The server adds `uuid`, `log_index`, `server_timestamp`, and `server_signature` to the stored
entry and returns them in the response.

The signer signs the canonical JSON serialisation of the five fields above (keys sorted, no
extra whitespace). Any verifier can reconstruct the signed payload from the entry fields and
check the PGP signature against the claimed `signer_keyid` — no separate `payload` field is
needed.

### Timestamp security model

The server signs each entry with its own GPG key, providing an authoritative timestamp
that prevents back-dating. The extension verifies both the server's timestamp signature
and the signer's attestation signature before displaying a badge.

**Self-revocation** (key owner revokes their own key):

- Alice signs verdicts throughout 2025; the log timestamps each one at submission time.
- In 2026, Alice revokes her own key (e.g., she lost control of it).
- Her verdicts from 2025 remain valid — the server timestamps prove they predate the revocation.
- Nobody can fabricate a new verdict under Alice's key and claim it was signed in 2025, because
  the server would timestamp it in 2026, after the revocation.

**Trust withdrawal** (user revokes their certification on a signer's key):

- Bob certified Alice's key in 2024 (signed it with `gpg --sign-key`).
- In 2026, Bob decides Alice is no longer reliable and revokes his certification
  (`gpg --edit-key alice revsig`). The revocation has a timestamp.
- Alice's verdicts from 2025 (timestamped before Bob's revocation) remain valid for Bob.
- Alice's verdicts timestamped after 2026 are dropped for Bob.
- This is a per-user action — Carol's trust in Alice is unaffected unless Carol also revokes.

**TODO: Third-party revocation monitoring**

A separate advisory tool could watch keyservers for revocations on keys the user currently
trusts and alert them: "Carol revoked her certification on Alice — you still trust Alice
independently." The user would then decide whether to revoke their own certification. This
is independent of the verification pipeline and does not gate badge display.

## Release & Packaging

Releases are triggered by pushing a `v*` tag. CI builds installation packages for each platform
— no raw binaries are released (except Windows, which has no native installer format).

| Platform | Format | GPG dependency |
| -------- | ------ | -------------- |
| Debian/Ubuntu | `.deb` | `gnupg` (declared in `Depends:`) |
| Fedora/RHEL | `.rpm` | `gnupg2` (declared in `Requires:`) |
| macOS | `.pkg` | user must install via Homebrew (`brew install gnupg`) |
| Windows | raw `.exe` | user must install [Gpg4win](https://www.gpg4win.org/) |

### Browser extension is NOT bundled in client packages

Firefox Snap (Ubuntu default since 22.04) runs in a sandbox that cannot see system-wide
extension paths (`/usr/lib/mozilla/extensions/`). Bundling the `.xpi` into the deb would only
work for deb-installed Firefox, making it fragile and confusing. Instead, all packages print
a post-install message with links to install the extension from the browser stores.

**Placeholder URLs**: `TODO_FIREFOX_ADDON_URL` and `TODO_CHROME_WEBSTORE_URL` appear in
`build-deb.sh`, `build-rpm.sh`, `build-pkg.sh`, and `README.md`. Replace them once the
extensions are publicly listed on AMO and Chrome Web Store.

### RPM note

The RPM spec uses `Requires: gnupg2` because that is the package name on Fedora/RHEL.
Debian/Ubuntu use `gnupg` instead. Both provide the `gpg` binary.

## Infrastructure (devcontainer)

The devcontainer runs the backing services automatically on every container start.

| Service               | Purpose                               | Port     |
| --------------------- | ------------------------------------- | -------- |
| MariaDB               | Trillian backing store                | 3306     |
| Trillian log server   | Append-only Merkle tree (gRPC)        | 8090     |
| Trillian log signer   | Batches and signs tree heads          | —        |
| Redis                 | Search index (artifact hash → UUIDs)  | 6379     |
| **gpg-attest-server** | Custom log API                        | **8081** |
| Caddy                 | HTTPS reverse proxy (TLS termination) | 443      |

The Trillian tree ID is written to `~/.gpg-attest/tree_id` on first start and reused on subsequent
restarts.

### Automatic startup

`postStartCommand` runs `init-gpg.sh && start-caddy.sh && start-services.sh` (all in `.devcontainer/`). Each script is idempotent.

### Verifying the server is up

```bash
curl http://localhost:8081/api/v1/loginfo
curl -k https://gpg-attest.org/api/v1/loginfo   # -k for self-signed cert in dev
```

### Logs

```
~/.gpg-attest/logs/startup.log
~/.gpg-attest/logs/gpg-attest-server.log
~/.gpg-attest/logs/trillian-log-server.log
~/.gpg-attest/logs/trillian-log-signer.log
~/.gpg-attest/logs/redis.log
~/.gpg-attest/logs/caddy.log
```

### Manual restart

```bash
/workspace/.devcontainer/start-services.sh
```

### Pointing the extension at the local server

Set `LOG_SERVER=https://gpg-attest.org` in your `.env` file (copy from `.env.example`).

## GPG keyring isolation

VS Code's Dev Containers extension injects host GPG keys and relay sockets into the container
at attach time (`pubring.kbx`, `private-keys-v1.d/*.key`, and four `S.gpg-agent*` sockets).
For a signing project this is a risk: `gpg --sign` could operate on host private keys, and
`S.gpg-agent.ssh` exposes host SSH keys.

`.devcontainer/init-gpg.sh` runs at all three lifecycle hooks (`postCreateCommand`,
`postStartCommand`, `postAttachCommand`) to defend against this. On each attach it:

1. Checks whether any non-test key is present in the keyring
2. If yes (or if the test key is missing): kills the relay agent, wipes all injected key
   material and relay sockets, starts a fresh container-local `gpg-agent`, creates the test key
3. Otherwise: no-op

The `postAttachCommand` hook is the critical one — it fires after VS Code has fully attached
and injected keys, so it always runs last.

## Manual Testing

A complete end-to-end test using Firefox and the local test page:

### 1. Build and install the client

```bash
cd /workspace/client && make install
```

This compiles the Go binary and writes the native messaging manifests for Firefox
(`~/.mozilla/native-messaging-hosts/`) and Chromium (`~/.config/chromium/NativeMessagingHosts/`).

### 2. Start the test page HTTP server

The service worker uses `fetch()`, which is blocked on `file://` origins.

```bash
cd /workspace/testpage && python3 -m http.server 8080
```

Leave this running in a separate terminal.

### 3. Open Firefox and load the extension

Load the extension as a temporary add-on (see [Installation](README.md#installation) in README).

### 4. Open the test page

Navigate to `http://localhost:8080` in Firefox.

Open DevTools (F12) and switch to the **Console** tab.

### 5. Attest an image

1. Right-click any image → click **Attest...** in the context menu
2. The attestation dialog opens with key selector, checkboxes for **I created this** (authorship) and **AI-generated** (method), and radio buttons for **Authentic** / **Satire** / **Misleading** (authenticity)
3. Select one or more verdicts → click **Sign** → the Console prints the sha256 hash and PGP signature for each category

### Reloading after code changes

After editing extension JS/CSS, click **Reload** on the `about:debugging` page.
After editing the native host, re-run `cd /workspace/client && make install` and reload
the extension.

## Verdict Categories

Verdicts use three independent categories. Each is independently signable and revocable.

| Category | Type | Verdicts | Meaning |
| --- | --- | --- | --- |
| **Authorship** | Toggle (checkbox) | `my-work` | "I created this" |
| **Method** | Toggle (checkbox) | `ai-generated` | "AI was used to produce this" |
| **Authenticity** | Exclusive scale (radio) | `authentic`, `satire`, `misleading` | Deception/intent spectrum |

A signer can select any combination (e.g., `my-work` + `ai-generated` + `authentic`).
No selection in a category means no claim about that dimension. The `revoke` verdict
withdraws a previous claim in a category, returning to silence.

### Aggregation

The extension queries the log for all entries matching an artifact hash, filters to
trusted signers (per the user's GPG web-of-trust), keeps the latest entry per
(signer, category) pair, verifies both server-timestamp and signer signatures, then
aggregates per category using plurality vote. Up to three badges are displayed per
image (one per category), stacked horizontally using 16px icons.

### Badge Icons

SVG source and pre-rendered PNGs live in `extension/icons/`. Each icon is a 32×32 symbol
with white stroke outline, drawn as SVG paths/polygons (no fonts). Area shapes (star,
triangle) use solid fill + white stroke. Line shapes (checkmark, tilde, times) use a
two-layer stroke: thicker white underneath, thinner colored on top.

| File prefix              | Shape     | Color  | Hex       |
| ------------------------ | --------- | ------ | --------- |
| `authorship-my-work`     | ★ star    | Green  | `#2E7D32` |
| `method-ai-generated`    | △ triangle| Blue   | `#1565C0` |
| `authenticity-authentic` | ✓ check   | Green  | `#2E7D32` |
| `authenticity-satire`    | ~ tilde   | Amber  | `#F57F17` |
| `authenticity-misleading`| ✕ times   | Red    | `#C62828` |

PNGs are generated from the SVGs at sizes 16, 24, 32, 64, and 128 px using `rsvg-convert`:

```bash
for svg in extension/icons/authorship-*.svg extension/icons/method-*.svg extension/icons/authenticity-*.svg; do
  base="${svg%.svg}"
  for size in 16 24 32 64 128; do
    rsvg-convert -w $size -h $size "$svg" -o "${base}-${size}.png"
  done
done
```
