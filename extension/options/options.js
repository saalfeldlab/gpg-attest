function sendBg(msg) {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendMessage(msg, resp => {
      if (chrome.runtime.lastError) return reject(chrome.runtime.lastError);
      resolve(resp);
    });
  });
}

const select = document.getElementById("sig-stuff-key-select");
const status = document.getElementById("status");

async function load() {
  let keys = [];
  try {
    const resp = await sendBg({ type: "list_keys" });
    if (resp.ok && resp.keys) {
      keys = resp.keys.filter(k => k.can_sign);
    }
  } catch (err) {
    status.textContent = "Error: could not list keys.";
    status.className = "error";
    select.disabled = true;
    return;
  }

  const { keyID: storedKeyID } = await sendBg({ type: "get_key" });

  if (keys.length === 0) {
    const opt = document.createElement("option");
    opt.textContent = "(no signing keys found)";
    opt.disabled = true;
    select.appendChild(opt);
    select.disabled = true;
    return;
  }

  for (const key of keys) {
    const opt = document.createElement("option");
    const shortFP = key.fingerprint.slice(-8);
    opt.textContent = `${key.uid} [${shortFP}]`;
    opt.value = key.fingerprint;
    if (key.fingerprint === storedKeyID) opt.selected = true;
    select.appendChild(opt);
  }

  if (!storedKeyID && keys.length > 0) {
    select.options[0].selected = true;
  }
}

select.addEventListener("change", async () => {
  const selected = select.value;
  if (!selected) return;
  try {
    await sendBg({ type: "set_key", keyID: selected });
  } catch (err) {
    status.textContent = "Error: could not save key.";
    status.className = "error";
  }
});

load();
