// --- Shared guards ---

function isOwnResource(url) {
  return url && url.startsWith(chrome.runtime.getURL(''));
}

function isOwnElement(el) {
  return el.classList.contains('attest-badge') ||
         el.classList.contains('attest-wrapper') ||
         (el.tagName === 'IMG' && isOwnResource(el.src));
}

// --- Background-image URL parser ---

function parseBgUrl(el) {
  const bg = getComputedStyle(el).backgroundImage;
  if (!bg || bg === 'none') return null;
  const m = bg.match(/url\(["']?([^"')]+)["']?\)/);
  if (!m) return null;
  return isOwnResource(m[1]) ? null : m[1];
}

// --- Context menu image resolver ---

function resolveContextImage(e) {
  const target = e.target;

  // 1. target is <img> and not ours — fast path
  if (target.tagName === 'IMG' && !isOwnElement(target)) {
    return target.currentSrc || target.src || null;
  }

  // 2. elementsFromPoint: finds <img> physically under the cursor even if covered by an overlay
  const atPoint = document.elementsFromPoint(e.clientX, e.clientY);
  const imgAtPoint = atPoint.find(el => el.tagName === 'IMG' && !isOwnElement(el));
  if (imgAtPoint) return imgAtPoint.currentSrc || imgAtPoint.src || null;

  // 3. Search target's own subtree for largest <img>
  const subtreeImgs = [...target.querySelectorAll('img')].filter(img => !isOwnElement(img));
  if (subtreeImgs.length > 0) {
    const largest = subtreeImgs.reduce((best, img) => {
      const r = img.getBoundingClientRect();
      const bestR = best.getBoundingClientRect();
      return r.width * r.height > bestR.width * bestR.height ? img : best;
    });
    const src = largest.currentSrc || largest.src;
    if (src) return src;
  }

  // 4. elementsFromPoint: check for background-image on any element at cursor
  for (const el of atPoint) {
    if (isOwnElement(el)) continue;
    const bg = parseBgUrl(el);
    if (bg) return bg;
  }

  // 5. parseBgUrl on the target itself
  const bgUrl = parseBgUrl(target);
  if (bgUrl) return bgUrl;

  // 6. Ancestor walk — <img> check and parseBgUrl only
  let el = target.parentElement;
  while (el && el !== document.body) {
    if (el.tagName === 'IMG' && !isOwnElement(el)) {
      return el.currentSrc || el.src || null;
    }
    const ancestorBg = parseBgUrl(el);
    if (ancestorBg) return ancestorBg;
    el = el.parentElement;
  }
  return null;
}

let lastContextImageUrl = null;

document.addEventListener('contextmenu', e => {
  lastContextImageUrl = resolveContextImage(e);
}, true);

// --- Background communication helper ---

function sendBg(msg) {
  return new Promise((resolve, reject) => {
    chrome.runtime.sendMessage(msg, (resp) => {
      if (chrome.runtime.lastError) return reject(chrome.runtime.lastError);
      resolve(resp);
    });
  });
}

// --- Message listener ---

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === "sig_result") {
    document.querySelectorAll("img").forEach(img => {
      if (img.currentSrc === msg.url || img.src === msg.url) {
        processedImages.delete(img);
        processSingleImage(img);
      }
    });
    document.querySelectorAll('*').forEach(el => {
      if (parseBgUrl(el) === msg.url) {
        processedBackgrounds.delete(el);
        processSingleBackground(el);
      }
    });
  } else if (msg.type === "get_context_image") {
    sendResponse(lastContextImageUrl);
  } else if (msg.type === "sig_warn") {
    console.warn("[attestension]", msg.message);
  } else if (msg.type === "sig_error") {
    console.error("[attestension]", msg.message);
  }
});

// --- DCBS badge overlay ---

const DCBS_LABELS = ["", "false", "suspect", "plausible", "trusted", "verified"];
const processedImages = new WeakSet();
const processedBackgrounds = new WeakSet();

async function processSingleImage(img) {
  if (processedImages.has(img)) return;
  if (isOwnElement(img)) return;
  const src = img.currentSrc || img.src;
  if (!src || src.startsWith("data:") || src.startsWith("blob:")) return;

  if (img.naturalWidth === 0 || img.naturalHeight === 0) {
    img.addEventListener("load", () => processSingleImage(img), { once: true });
    return;
  }
  if (img.naturalWidth < 48 || img.naturalHeight < 48) return;

  processedImages.add(img);

  let result;
  try {
    result = await sendBg({ type: "get_verdicts", url: src });
  } catch (_) {
    return;
  }
  if (!result || result.level === null || result.level === undefined) return;

  attachBadge(img, result.level);
}

function attachBadge(img, level) {
  const iconUrl = chrome.runtime.getURL(`icons/dcbs-${level}-${DCBS_LABELS[level]}-24.png`);
  const title = `DCBS level ${level}: ${DCBS_LABELS[level]}`;

  if (img.parentElement && img.parentElement.classList.contains("attest-wrapper")) {
    const badge = img.parentElement.querySelector(".attest-badge");
    if (badge) { badge.src = iconUrl; badge.alt = title; badge.title = title; }
    return;
  }

  const wrapper = document.createElement("span");
  wrapper.className = "attest-wrapper";
  img.parentElement.insertBefore(wrapper, img);
  wrapper.appendChild(img);

  const badge = document.createElement("img");
  badge.className = "attest-badge";
  badge.src = iconUrl;
  badge.alt = title;
  badge.title = title;
  wrapper.appendChild(badge);
}

function attachBadgeToBg(el, level) {
  const iconUrl = chrome.runtime.getURL(`icons/dcbs-${level}-${DCBS_LABELS[level]}-24.png`);
  const title = `DCBS level ${level}: ${DCBS_LABELS[level]}`;

  if (getComputedStyle(el).position === 'static') {
    el.style.position = 'relative';
  }

  const existing = el.querySelector(':scope > .attest-badge');
  if (existing) {
    existing.src = iconUrl;
    existing.alt = title;
    existing.title = title;
    return;
  }

  const badge = document.createElement("img");
  badge.className = "attest-badge";
  badge.src = iconUrl;
  badge.alt = title;
  badge.title = title;
  el.appendChild(badge);
}

async function processSingleBackground(el) {
  if (processedBackgrounds.has(el)) return;
  if (isOwnElement(el)) return;
  if (el.tagName === 'IMG') return;
  const url = parseBgUrl(el);
  if (!url || url.startsWith('data:') || url.startsWith('blob:')) return;
  const rect = el.getBoundingClientRect();
  if (rect.width < 48 || rect.height < 48) return;

  processedBackgrounds.add(el);

  let result;
  try {
    result = await sendBg({ type: "get_verdicts", url });
  } catch (_) { return; }
  if (!result || result.level == null) return;

  attachBadgeToBg(el, result.level);
}

function processImages() {
  document.querySelectorAll("img").forEach(img => processSingleImage(img));
  document.querySelectorAll('*').forEach(el => processSingleBackground(el));
}

const observer = new MutationObserver(mutations => {
  for (const mutation of mutations) {
    for (const node of mutation.addedNodes) {
      if (node.nodeType !== Node.ELEMENT_NODE) continue;
      if (node.tagName === "IMG") {
        processSingleImage(node);
      } else {
        node.querySelectorAll("img").forEach(img => processSingleImage(img));
        processSingleBackground(node);
        node.querySelectorAll('*').forEach(el => processSingleBackground(el));
      }
    }
  }
});
observer.observe(document.documentElement, { childList: true, subtree: true });

processImages();
