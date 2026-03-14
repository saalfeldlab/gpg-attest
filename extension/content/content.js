// --- Background communication helper ---

function sendBg(msg) {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendMessage(msg, resp => {
      if (chrome.runtime.lastError) return reject(chrome.runtime.lastError);
      resolve(resp);
    });
  });
}

// --- Message listener ---

chrome.runtime.onMessage.addListener((msg) => {
  if (msg.type === "sig_result")
    console.log(`[sig-stuff] ${msg.label}\nurl: ${msg.url}\nsha256: ${msg.sha256}\n${msg.signature}`);
  else if (msg.type === "sig_warn")  console.warn("[sig-stuff]", msg.message);
  else if (msg.type === "sig_error") console.error("[sig-stuff]", msg.message);
});
