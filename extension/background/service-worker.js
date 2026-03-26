const DEFAULT_LOG_SERVER = "https://gpg-attest.org:8443";

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  handleMessage(msg).then(sendResponse).catch(err => sendResponse({ ok: false, error: err.message }));
  return true; // keep channel open for async response
});

async function handleMessage(msg) {
  switch (msg.type) {
    case "list_keys": {
      const id = crypto.randomUUID();
      const resp = await new Promise(resolve =>
        chrome.runtime.sendNativeMessage("org.gpg_attest.client", { id, op: "list_keys" }, resolve)
      );
      return { ok: resp.ok, keys: resp.keys, error: resp.error };
    }

    case "sign": {
      return handleSign(msg.url, msg.keyID, msg.verdict);
    }

    case "get_key": {
      const result = await chrome.storage.local.get("selectedKeyID");
      return { keyID: result.selectedKeyID || null };
    }

    case "set_key": {
      await chrome.storage.local.set({ selectedKeyID: msg.keyID });
      return { ok: true };
    }

    case "get_verdicts":
      return handleGetVerdicts(msg.url);

    case "get_log_url": {
      const result = await chrome.storage.local.get("logUrl");
      return { logUrl: result.logUrl || DEFAULT_LOG_SERVER };
    }

    case "set_log_url": {
      await chrome.storage.local.set({ logUrl: msg.logUrl });
      return { ok: true };
    }

    default:
      throw new Error(`Unknown message type: ${msg.type}`);
  }
}

const VERDICT_SCORE = { false: 1, suspect: 2, plausible: 3, trusted: 4, verified: 5 };

let trustedKeysCache = null; // { keys: string[], fetchedAt: number }
const verdictsCache = new Map(); // url -> { level: number|null }

async function getTrustedFingerprints() {
  const now = Date.now();
  if (trustedKeysCache && now - trustedKeysCache.fetchedAt < 5 * 60 * 1000) {
    return trustedKeysCache.keys;
  }
  const id = crypto.randomUUID();
  const resp = await new Promise(resolve =>
    chrome.runtime.sendNativeMessage("org.gpg_attest.client", { id, op: "list_keys" }, resolve)
  );
  const keys = (resp.keys || [])
    .filter(k => k.trust === "f" || k.trust === "u")
    .map(k => k.fingerprint);
  trustedKeysCache = { keys, fetchedAt: now };
  return keys;
}

async function handleGetVerdicts(url) {
  if (verdictsCache.has(url)) return verdictsCache.get(url);

  try {
    const response = await fetch(url);
    if (!response.ok) { verdictsCache.set(url, { level: null }); return { level: null }; }
    const buffer = await response.arrayBuffer();
    const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
    const sha256Hex = Array.from(new Uint8Array(hashBuffer))
      .map(b => b.toString(16).padStart(2, "0")).join("");

    const { logUrl } = await chrome.storage.local.get("logUrl");
    const logServer = logUrl || DEFAULT_LOG_SERVER;

    const trustedFingerprints = await getTrustedFingerprints();
    if (trustedFingerprints.length === 0) { verdictsCache.set(url, { level: null }); return { level: null }; }

    const entriesResp = await fetch(`${logServer}/api/v1/entries?hash=sha256:${sha256Hex}`);
    if (!entriesResp.ok) { verdictsCache.set(url, { level: null }); return { level: null }; }
    const entries = await entriesResp.json();

    // Filter to trusted signers, keep latest entry per signer
    const bySignerMap = new Map();
    for (const entry of (entries || [])) {
      if (!trustedFingerprints.includes(entry.signer_keyid)) continue;
      const existing = bySignerMap.get(entry.signer_keyid);
      if (!existing || entry.server_timestamp > existing.server_timestamp) {
        bySignerMap.set(entry.signer_keyid, entry);
      }
    }

    if (bySignerMap.size === 0) { verdictsCache.set(url, { level: null }); return { level: null }; }

    const scores = [...bySignerMap.values()]
      .map(e => VERDICT_SCORE[e.verdict])
      .filter(s => s !== undefined);
    if (scores.length === 0) { verdictsCache.set(url, { level: null }); return { level: null }; }

    const level = Math.round(scores.reduce((a, b) => a + b, 0) / scores.length);
    const result = { level };
    verdictsCache.set(url, result);
    return result;
  } catch (_) {
    verdictsCache.set(url, { level: null });
    return { level: null };
  }
}

async function handleSign(url, keyID, verdict) {
  // 1. Fetch image and compute SHA-256
  const response = await fetch(url);
  if (!response.ok) throw new Error(`Fetch failed: ${response.status} ${response.statusText}`);
  const buffer = await response.arrayBuffer();
  const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
  const sha256Hex = Array.from(new Uint8Array(hashBuffer))
    .map(b => b.toString(16).padStart(2, "0")).join("");

  // 2. Build canonical payload (keys sorted alphabetically, no whitespace)
  const canonicalPayload = JSON.stringify({
    artifact_hash: `sha256:${sha256Hex}`,
    signer_keyid: keyID,
    verdict,
  });

  // 3. Sign payload via native host
  const signId = crypto.randomUUID();
  const payloadB64 = btoa(unescape(encodeURIComponent(canonicalPayload)));
  const nativeResp = await new Promise(resolve =>
    chrome.runtime.sendNativeMessage(
      "org.gpg_attest.client",
      { id: signId, op: "sign", key_id: keyID, payload: payloadB64 },
      resolve
    )
  );
  if (!nativeResp.ok) throw new Error(nativeResp.error || "sign failed");

  // 4. POST to log server
  const { logUrl } = await chrome.storage.local.get("logUrl");
  const logServer = logUrl || DEFAULT_LOG_SERVER;
  const postResp = await fetch(`${logServer}/api/v1/entries`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      artifact_hash: `sha256:${sha256Hex}`,
      verdict,
      signer_keyid: keyID,
      signature: nativeResp.signature,
    }),
  });
  if (!postResp.ok) {
    const text = await postResp.text();
    throw new Error(`Log server submission failed (${postResp.status}): ${text}`);
  }
  const entry = await postResp.json();

  verdictsCache.delete(url);

  return { ok: true, sha256: sha256Hex, verdict, uuid: entry.uuid, logIndex: entry.log_index };
}

const VERDICTS = ["false", "suspect", "plausible", "trusted", "verified"];

// Register context menu items
function registerMenus() {
  chrome.contextMenus.removeAll(() => {
    const isFirefox = !!chrome.runtime.getBrowserInfo;
    if (isFirefox) {
      chrome.contextMenus.create({
        id: "sig-attest",
        title: "Attest",
        icons: {
          "16": "icons/dcbs-2-suspect-16.png",
          "32": "icons/dcbs-2-suspect-32.png",
        },
        contexts: ["all"],
      });
    }
    for (const v of VERDICTS) {
      const level = VERDICT_SCORE[v];
      const item = {
        id: `sig-verdict-${v}`,
        title: v,
        icons: {
          "16": `icons/dcbs-${level}-${v}-16.png`,
          "32": `icons/dcbs-${level}-${v}-32.png`,
        },
        contexts: ["all"],
      };
      if (isFirefox) item.parentId = "sig-attest";
      chrome.contextMenus.create(item);
    }
  });
}
async function initDefaults() {
  const stored = await chrome.storage.local.get(["selectedKeyID", "logUrl"]);
  const updates = {};
  if (!stored.logUrl) {
    updates.logUrl = DEFAULT_LOG_SERVER;
  }
  if (!stored.selectedKeyID) {
    const id = crypto.randomUUID();
    const resp = await new Promise(resolve =>
      chrome.runtime.sendNativeMessage("org.gpg_attest.client", { id, op: "list_keys" }, resolve)
    );
    const signingKeys = (resp.keys || []).filter(k => k.can_sign);
    if (signingKeys.length > 0) {
      updates.selectedKeyID = signingKeys[0].fingerprint;
    }
  }
  if (Object.keys(updates).length > 0) {
    await chrome.storage.local.set(updates);
  }
}

chrome.runtime.onInstalled.addListener(() => { registerMenus(); initDefaults(); });
chrome.runtime.onStartup.addListener(() => { registerMenus(); initDefaults(); });

async function getContextImageUrl(info, tabId) {
  if (info.srcUrl) return info.srcUrl;
  return new Promise(resolve =>
    chrome.tabs.sendMessage(tabId, { type: "get_context_image" }, resolve)
  );
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const verdict = info.menuItemId.replace("sig-verdict-", "");
  if (!VERDICTS.includes(verdict)) return;
  const { selectedKeyID: keyID } = await chrome.storage.local.get("selectedKeyID");
  if (!keyID) {
    chrome.tabs.sendMessage(tab.id, { type: "sig_warn", message: "No key selected. Open extension options to choose a signing key." });
    return;
  }
  const imageUrl = await getContextImageUrl(info, tab.id);
  if (!imageUrl) {
    chrome.tabs.sendMessage(tab.id, { type: "sig_warn", message: "No image found at click target." });
    return;
  }
  try {
    const result = await handleSign(imageUrl, keyID, verdict);
    chrome.tabs.sendMessage(tab.id, { type: "sig_result", url: imageUrl, ...result });
  } catch (err) {
    chrome.tabs.sendMessage(tab.id, { type: "sig_error", message: err.message });
  }
});
