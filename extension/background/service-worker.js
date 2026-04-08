const DEFAULT_LOG_SERVER = "https://gpg-attest.org";

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  handleMessage(msg)
    .then(sendResponse)
    .catch((err) => sendResponse({ ok: false, error: err.message }));
  return true; // keep channel open for async response
});

async function handleMessage(msg) {
  switch (msg.type) {
    case "list_keys": {
      const resp = await nativeMessage({ op: "list_keys" });
      return { ok: resp.ok, keys: resp.keys, error: resp.error };
    }

    case "list_secret_keys": {
      const resp = await nativeMessage({ op: "list_secret_keys" });
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

const VERDICT_SCORE = {
  false: 1,
  suspect: 2,
  plausible: 3,
  trusted: 4,
  verified: 5,
};

let trustedKeysCache = null; // { keys: string[], fetchedAt: number }
let serverKeyCache = null; // { fingerprint: string, importedAt: number }
const verdictsCache = new Map();       // url -> { result, fetchedAt }
const verdictsPending = new Map();     // url -> Promise<result>
const VERDICT_TTL_OK   = 30 * 60 * 1000;  // 30 min for successful lookups
const VERDICT_TTL_NULL =  5 * 60 * 1000;  // 5 min for null/empty lookups
const VERDICT_CACHE_MAX = 500;

let trustedKeysPending = null;

async function getTrustedFingerprints() {
  const now = Date.now();
  if (trustedKeysCache && now - trustedKeysCache.fetchedAt < 5 * 60 * 1000) {
    return trustedKeysCache.keys;
  }
  if (trustedKeysPending) return trustedKeysPending;
  trustedKeysPending = (async () => {
    try {
      const resp = await nativeMessage({ op: "list_keys" });
      const keys = (resp.keys || [])
        .filter((k) => k.trust === "f" || k.trust === "u")
        .map((k) => k.fingerprint);
      trustedKeysCache = { keys, fetchedAt: Date.now() };
      return keys;
    } finally {
      trustedKeysPending = null;
    }
  })();
  return trustedKeysPending;
}

function nativeMessage(msg) {
  msg.id = msg.id || crypto.randomUUID();
  return new Promise((resolve, reject) =>
    chrome.runtime.sendNativeMessage("org.gpg_attest.client", msg, (resp) => {
      if (chrome.runtime.lastError) {
        reject(new Error(chrome.runtime.lastError.message));
      } else if (!resp) {
        reject(new Error("No response from native host"));
      } else {
        resolve(resp);
      }
    }),
  );
}

let serverKeyPending = null;

async function ensureServerKeyImported() {
  const now = Date.now();
  if (serverKeyCache && now - serverKeyCache.importedAt < 24 * 60 * 60 * 1000) {
    return serverKeyCache.fingerprint;
  }
  if (serverKeyPending) return serverKeyPending;
  serverKeyPending = (async () => {
    try {
      const { logUrl } = await chrome.storage.local.get("logUrl");
      const logServer = logUrl || DEFAULT_LOG_SERVER;
      const resp = await fetch(`${logServer}/api/v1/publickey`);
      if (!resp.ok) throw new Error("failed to fetch server public key");
      const armoredKey = await resp.text();
      const importResp = await nativeMessage({
        op: "import_key",
        payload: btoa(armoredKey),
      });
      if (!importResp.ok || !importResp.imported?.length) {
        throw new Error(importResp.error || "import_key failed");
      }
      serverKeyCache = { fingerprint: importResp.imported[0], importedAt: Date.now() };
      return serverKeyCache.fingerprint;
    } finally {
      serverKeyPending = null;
    }
  })();
  return serverKeyPending;
}

function canonicalJSON(obj) {
  return JSON.stringify(obj, Object.keys(obj).sort());
}

async function handleGetVerdicts(url) {
  const cached = verdictsCache.get(url);
  if (cached) {
    const ttl = cached.result.level != null ? VERDICT_TTL_OK : VERDICT_TTL_NULL;
    if (Date.now() - cached.fetchedAt < ttl) return cached.result;
    verdictsCache.delete(url);
  }

  if (verdictsPending.has(url)) return verdictsPending.get(url);

  const pending = (async () => {
    function cacheAndReturn(result) {
      if (verdictsCache.size >= VERDICT_CACHE_MAX) {
        verdictsCache.delete(verdictsCache.keys().next().value);
      }
      verdictsCache.set(url, { result, fetchedAt: Date.now() });
      return result;
    }

    try {
      const response = await fetch(url);
      if (!response.ok) {
        return cacheAndReturn({ level: null });
      }
      const buffer = await response.arrayBuffer();
      const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
      const sha256Hex = Array.from(new Uint8Array(hashBuffer))
        .map((b) => b.toString(16).padStart(2, "0"))
        .join("");

      const { logUrl } = await chrome.storage.local.get("logUrl");
      const logServer = logUrl || DEFAULT_LOG_SERVER;

      const trustedFingerprints = await getTrustedFingerprints();
      if (trustedFingerprints.length === 0) {
        return cacheAndReturn({ level: null });
      }

      const entriesResp = await fetch(
        `${logServer}/api/v1/entries?hash=sha256:${sha256Hex}`,
      );
      if (!entriesResp.ok) {
        return cacheAndReturn({ level: null });
      }
      const entries = await entriesResp.json();

      // Filter to trusted signers, keep latest entry per signer
      const bySignerMap = new Map();
      for (const entry of entries || []) {
        if (!trustedFingerprints.includes(entry.signer_keyid)) continue;
        const existing = bySignerMap.get(entry.signer_keyid);
        if (!existing || entry.server_timestamp > existing.server_timestamp) {
          bySignerMap.set(entry.signer_keyid, entry);
        }
      }

      if (bySignerMap.size === 0) {
        return cacheAndReturn({ level: null });
      }

      // Verify signatures: server timestamp signature + signer attestation signature
      let serverFingerprint;
      try {
        serverFingerprint = await ensureServerKeyImported();
      } catch (e) {
        console.debug("[attestension] could not import server key:", e.message);
        return cacheAndReturn({ level: null });
      }

      const entriesToVerify = [...bySignerMap.values()];
      const verifyEntries = [];
      for (const entry of entriesToVerify) {
        // Server timestamp signature
        verifyEntries.push({
          signature: entry.server_signature,
          payload: btoa(
            canonicalJSON({
              artifact_hash: entry.artifact_hash,
              log_index: entry.log_index,
              server_timestamp: entry.server_timestamp,
              signature: entry.signature,
              signer_keyid: entry.signer_keyid,
              uuid: entry.uuid,
              verdict: entry.verdict,
            }),
          ),
          signer_keyid: serverFingerprint,
          timestamp: entry.server_timestamp,
        });
        // Signer attestation signature
        verifyEntries.push({
          signature: entry.signature,
          payload: btoa(
            canonicalJSON({
              artifact_hash: entry.artifact_hash,
              signer_keyid: entry.signer_keyid,
              verdict: entry.verdict,
            }),
          ),
          signer_keyid: entry.signer_keyid,
          timestamp: entry.server_timestamp,
        });
      }

      // Get user's own key fingerprints for cert revocation checking
      const allKeysResp = await nativeMessage({ op: "list_keys" });
      const verifierKeyIDs = (allKeysResp.keys || [])
        .filter((k) => k.trust === "u")
        .map((k) => k.fingerprint);

      const verifyResp = await nativeMessage({
        op: "verify",
        entries: verifyEntries,
        verifier_keyids: verifierKeyIDs,
      });
      const verified = [];
      if (verifyResp.ok && verifyResp.verify_results) {
        for (let i = 0; i < entriesToVerify.length; i++) {
          const serverResult = verifyResp.verify_results[i * 2];
          const signerResult = verifyResp.verify_results[i * 2 + 1];
          if (serverResult?.valid && signerResult?.valid) {
            verified.push(entriesToVerify[i]);
          } else {
            const entry = entriesToVerify[i];
            const reasons = [];
            if (!serverResult?.valid)
              reasons.push("server sig: " + (serverResult?.error || "invalid"));
            if (!signerResult?.valid)
              reasons.push("signer sig: " + (signerResult?.error || "invalid"));
            console.debug(
              `[attestension] entry ${entry.uuid} dropped: ${reasons.join(", ")}`,
            );
          }
        }
      } else {
        console.debug("[attestension] verify failed:", verifyResp.error);
        return cacheAndReturn({ level: null });
      }

      if (verified.length === 0) {
        return cacheAndReturn({ level: null });
      }

      const scores = verified
        .map((e) => VERDICT_SCORE[e.verdict])
        .filter((s) => s !== undefined);
      if (scores.length === 0) {
        return cacheAndReturn({ level: null });
      }

      const level = Math.round(scores.reduce((a, b) => a + b, 0) / scores.length);
      return cacheAndReturn({ level });
    } catch (_) {
      return cacheAndReturn({ level: null });
    }
  })();
  verdictsPending.set(url, pending);
  try {
    return await pending;
  } finally {
    verdictsPending.delete(url);
  }
}

async function handleSign(url, keyID, verdict) {
  // 1. Fetch image and compute SHA-256
  const response = await fetch(url);
  if (!response.ok)
    throw new Error(`Fetch failed: ${response.status} ${response.statusText}`);
  const buffer = await response.arrayBuffer();
  const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
  const sha256Hex = Array.from(new Uint8Array(hashBuffer))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");

  // 2. Build canonical payload (keys sorted alphabetically, no whitespace)
  const canonicalPayload = canonicalJSON({
    artifact_hash: `sha256:${sha256Hex}`,
    signer_keyid: keyID,
    verdict,
  });

  // 3. Sign payload via native host
  const payloadB64 = btoa(canonicalPayload);
  const nativeResp = await nativeMessage({
    op: "sign",
    key_id: keyID,
    payload: payloadB64,
  });
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
    throw new Error(
      `Log server submission failed (${postResp.status}): ${text}`,
    );
  }
  const entry = await postResp.json();

  verdictsCache.delete(url);

  return {
    ok: true,
    sha256: sha256Hex,
    verdict,
    uuid: entry.uuid,
    logIndex: entry.log_index,
  };
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
          16: "icons/dcbs-2-suspect-16.png",
          32: "icons/dcbs-2-suspect-32.png",
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
          16: `icons/dcbs-${level}-${v}-16.png`,
          32: `icons/dcbs-${level}-${v}-32.png`,
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
    const resp = await nativeMessage({ op: "list_secret_keys" });
    const signingKeys = (resp.keys || []).filter((k) => k.can_sign);
    if (signingKeys.length > 0) {
      updates.selectedKeyID = signingKeys[0].fingerprint;
    }
  }
  if (Object.keys(updates).length > 0) {
    await chrome.storage.local.set(updates);
  }
}

chrome.runtime.onInstalled.addListener(() => {
  registerMenus();
  initDefaults().catch((err) =>
    console.warn("[attestension] initDefaults failed:", err.message),
  );
});
chrome.runtime.onStartup.addListener(() => {
  registerMenus();
  initDefaults().catch((err) =>
    console.warn("[attestension] initDefaults failed:", err.message),
  );
});

async function getContextImageUrl(info, tabId) {
  if (info.srcUrl) return info.srcUrl;
  return new Promise((resolve) =>
    chrome.tabs.sendMessage(tabId, { type: "get_context_image" }, resolve),
  );
}

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const verdict = info.menuItemId.replace("sig-verdict-", "");
  if (!VERDICTS.includes(verdict)) return;
  const { selectedKeyID: keyID } =
    await chrome.storage.local.get("selectedKeyID");
  if (!keyID) {
    chrome.tabs.sendMessage(tab.id, {
      type: "sig_warn",
      message:
        "No key selected. Open extension options to choose a signing key.",
    });
    return;
  }
  const imageUrl = await getContextImageUrl(info, tab.id);
  if (!imageUrl) {
    chrome.tabs.sendMessage(tab.id, {
      type: "sig_warn",
      message: "No image found at click target.",
    });
    return;
  }
  try {
    const result = await handleSign(imageUrl, keyID, verdict);
    chrome.tabs.sendMessage(tab.id, {
      type: "sig_result",
      url: imageUrl,
      ...result,
    });
  } catch (err) {
    chrome.tabs.sendMessage(tab.id, {
      type: "sig_error",
      message: err.message,
    });
  }
});
