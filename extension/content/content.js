// --- Shared guards ---

function isOwnResource(url) {
  return url && url.startsWith(chrome.runtime.getURL(''));
}

function isOwnElement(el) {
  return el.dataset && el.dataset.attestBadge !== undefined ||
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
    const bgEls = bgUrlToElements.get(msg.url);
    if (bgEls) {
      for (const el of bgEls) {
        if (el.isConnected) {
          processedBackgrounds.delete(el);
          processSingleBackground(el);
        }
      }
    }
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
const bgUrlToElements = new Map(); // url -> Set<Element>

// --- Per-image badge overlays (inserted into target's parent for correct stacking) ---

const BADGE_CSS = '.attest-badge{position:fixed;top:0;left:0;width:24px;height:24px;pointer-events:none;}';

const badgeRegistry = new Map();       // badgeId → { targetRef, badgeEl, host, visible }
const targetToBadgeId = new WeakMap(); // Element → badgeId
let nextBadgeId = 0;

const cleanupRegistry = new FinalizationRegistry(badgeId => removeBadge(badgeId));

function createBadge(targetEl, level) {
  const existing = targetToBadgeId.get(targetEl);
  if (existing !== undefined) {
    updateBadge(existing, level);
    return;
  }
  if (!targetEl.parentElement) return;

  const badgeId = nextBadgeId++;
  const iconUrl = chrome.runtime.getURL(`icons/dcbs-${level}-${DCBS_LABELS[level]}-24.png`);
  const title = `DCBS level ${level}: ${DCBS_LABELS[level]}`;

  const badgeEl = document.createElement('img');
  badgeEl.className = 'attest-badge';
  badgeEl.src = iconUrl;
  badgeEl.alt = title;
  badgeEl.title = title;
  badgeEl.style.display = 'none'; // hidden until IntersectionObserver confirms visibility

  const host = document.createElement('div');
  host.dataset.attestBadge = '';
  host.style.cssText = 'position:fixed;top:0;left:0;width:0;height:0;overflow:visible;pointer-events:none;';
  const shadow = host.attachShadow({ mode: 'closed' });
  const style = document.createElement('style');
  style.textContent = BADGE_CSS;
  shadow.appendChild(style);
  shadow.appendChild(badgeEl);
  targetEl.parentElement.appendChild(host);

  const record = { targetRef: new WeakRef(targetEl), badgeEl, host, visible: false };
  badgeRegistry.set(badgeId, record);
  targetToBadgeId.set(targetEl, badgeId);

  intersectionObs.observe(targetEl);
  resizeObs.observe(targetEl);
  cleanupRegistry.register(targetEl, badgeId);
}

function updateBadge(badgeId, level) {
  const record = badgeRegistry.get(badgeId);
  if (!record) return;
  const iconUrl = chrome.runtime.getURL(`icons/dcbs-${level}-${DCBS_LABELS[level]}-24.png`);
  const title = `DCBS level ${level}: ${DCBS_LABELS[level]}`;
  record.badgeEl.src = iconUrl;
  record.badgeEl.alt = title;
  record.badgeEl.title = title;
}

function removeBadge(badgeId) {
  const record = badgeRegistry.get(badgeId);
  if (!record) return;
  record.host.remove();
  badgeRegistry.delete(badgeId);
  const target = record.targetRef.deref();
  if (target) {
    targetToBadgeId.delete(target);
    intersectionObs.unobserve(target);
    resizeObs.unobserve(target);
  }
}

function removeBadgeForTarget(el) {
  const badgeId = targetToBadgeId.get(el);
  if (badgeId !== undefined) removeBadge(badgeId);
}

// --- Batched position updates ---

let updateScheduled = false;

function scheduleUpdate() {
  if (updateScheduled) return;
  updateScheduled = true;
  requestAnimationFrame(updateAllBadgePositions);
}

function updateAllBadgePositions() {
  updateScheduled = false;
  // Read phase — batch all getBoundingClientRect calls
  const updates = [];
  for (const [badgeId, record] of badgeRegistry) {
    if (!record.visible) continue;
    const target = record.targetRef.deref();
    if (!target) { removeBadge(badgeId); continue; }
    const targetRect = target.getBoundingClientRect();
    const hostRect = record.host.getBoundingClientRect();
    updates.push({
      badgeEl: record.badgeEl,
      top: targetRect.top - hostRect.top + 2,
      left: targetRect.right - hostRect.left - 24 - 2,
    });
  }
  // Write phase — batch all transform writes
  for (const { badgeEl, top, left } of updates) {
    badgeEl.style.transform = `translate(${left}px, ${top}px)`;
  }
}

// --- Observers ---

const intersectionObs = new IntersectionObserver(entries => {
  for (const entry of entries) {
    const badgeId = targetToBadgeId.get(entry.target);
    if (badgeId === undefined) continue;
    const record = badgeRegistry.get(badgeId);
    if (!record) continue;
    record.visible = entry.isIntersecting;
    record.badgeEl.style.display = entry.isIntersecting ? '' : 'none';
    if (entry.isIntersecting) scheduleUpdate();
  }
}, { threshold: 0 });

const resizeObs = new ResizeObserver(() => scheduleUpdate());

window.addEventListener('scroll', scheduleUpdate, { capture: true, passive: true });
window.addEventListener('resize', scheduleUpdate, { passive: true });

// --- Image and background processing ---

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

  createBadge(img, result.level);
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
  let elSet = bgUrlToElements.get(url);
  if (!elSet) { elSet = new Set(); bgUrlToElements.set(url, elSet); }
  elSet.add(el);

  let result;
  try {
    result = await sendBg({ type: "get_verdicts", url });
  } catch (_) { return; }
  if (!result || result.level == null) return;

  createBadge(el, result.level);
}

function processImages() {
  document.querySelectorAll("img").forEach(img => processSingleImage(img));
  document.querySelectorAll('*').forEach(el => processSingleBackground(el));
}

// --- DOM observer (structure + style/class attribute changes) ---

const domObserver = new MutationObserver(mutations => {
  let hasAttrChange = false;
  for (const mutation of mutations) {
    if (mutation.type === 'attributes') {
      hasAttrChange = true;
      continue;
    }
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
    for (const node of mutation.removedNodes) {
      if (node.nodeType !== Node.ELEMENT_NODE) continue;
      removeBadgeForTarget(node);
      for (const [url, elSet] of bgUrlToElements) {
        elSet.delete(node);
        if (elSet.size === 0) bgUrlToElements.delete(url);
      }
      if (node.querySelectorAll) {
        node.querySelectorAll('img').forEach(img => removeBadgeForTarget(img));
        node.querySelectorAll('*').forEach(el => {
          removeBadgeForTarget(el);
          for (const [url, elSet] of bgUrlToElements) {
            elSet.delete(el);
            if (elSet.size === 0) bgUrlToElements.delete(url);
          }
        });
      }
    }
  }
  if (hasAttrChange) scheduleUpdate();
});
domObserver.observe(document.documentElement, {
  childList: true,
  subtree: true,
  attributes: true,
  attributeFilter: ['style', 'class'],
});

processImages();
