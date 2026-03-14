chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  handleMessage(msg).then(sendResponse).catch(err => sendResponse({ ok: false, error: err.message }));
  return true; // keep channel open for async response
});

async function handleMessage(msg) {
  switch (msg.type) {
    case "list_keys": {
      const id = crypto.randomUUID();
      const resp = await new Promise(resolve =>
        chrome.runtime.sendNativeMessage("dev.sig_stuff.native", { id, op: "list_keys" }, resolve)
      );
      return { ok: resp.ok, keys: resp.keys, error: resp.error };
    }

    case "sign": {
      return handleSign(msg.url, msg.keyID);
    }

    case "get_key": {
      const result = await chrome.storage.local.get("selectedKeyID");
      return { keyID: result.selectedKeyID || null };
    }

    case "set_key": {
      await chrome.storage.local.set({ selectedKeyID: msg.keyID });
      return { ok: true };
    }

    default:
      throw new Error(`Unknown message type: ${msg.type}`);
  }
}

async function handleSign(url, keyID) {
  // Fetch full binary content (CORS bypass via host_permissions)
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`Fetch failed: ${response.status} ${response.statusText}`);
  }
  const buffer = await response.arrayBuffer();

  // SHA-256 hash of the raw bytes
  const hashBuffer = await crypto.subtle.digest("SHA-256", buffer);
  const hashBytes = new Uint8Array(hashBuffer);

  // Hex string for logging
  const sha256Hex = Array.from(hashBytes).map(b => b.toString(16).padStart(2, "0")).join("");

  // Base64 encode the 32-byte hash for the native host payload
  const payload = btoa(String.fromCharCode(...hashBytes));

  const id = crypto.randomUUID();
  const nativeResp = await new Promise(resolve =>
    chrome.runtime.sendNativeMessage(
      "dev.sig_stuff.native",
      { id, op: "sign", key_id: keyID, payload },
      resolve
    )
  );

  return { ...nativeResp, sha256: sha256Hex };
}

// Register context menu items
function registerMenus() {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({ id: "sig-sign-check", title: "Sign ✓", contexts: ["image", "video", "audio"] });
    chrome.contextMenus.create({ id: "sig-sign-x",     title: "Sign ✗", contexts: ["image", "video", "audio"] });
  });
}
chrome.runtime.onInstalled.addListener(registerMenus);
chrome.runtime.onStartup.addListener(registerMenus);

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const label = info.menuItemId === "sig-sign-check" ? "checkmark" : "xmark";
  const { selectedKeyID: keyID } = await chrome.storage.local.get("selectedKeyID");
  if (!keyID) {
    chrome.tabs.sendMessage(tab.id, { type: "sig_warn", message: "No key selected. Open extension options to choose a signing key." });
    return;
  }
  try {
    const result = await handleSign(info.srcUrl, keyID);
    chrome.tabs.sendMessage(tab.id, { type: "sig_result", label, url: info.srcUrl, ...result });
  } catch (err) {
    chrome.tabs.sendMessage(tab.id, { type: "sig_error", message: err.message });
  }
});
