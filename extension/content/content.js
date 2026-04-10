// --- Shared guards ---

function isOwnResource(url) {
  return url && url.startsWith(chrome.runtime.getURL(''));
}

function isOwnElement(el) {
  return el.dataset && el.dataset.attestBadge !== undefined ||
         el.dataset && el.dataset.attestDialog !== undefined ||
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

// --- Attestation dialog ---

let activeDialog = null;

const DIALOG_CSS = `
#attest-dialog-overlay {
  position: fixed;
  inset: 0;
  z-index: 2147483646;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
}
#attest-dialog {
  background: #1e1e1e;
  color: #e0e0e0;
  border: 1px solid #555;
  border-radius: 8px;
  padding: 20px 24px;
  min-width: 360px;
  max-width: 520px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6);
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: 13px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}
#attest-dialog h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  color: #f0f0f0;
}
#attest-dialog label {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 2px 0;
}
#attest-dialog input[type="checkbox"],
#attest-dialog input[type="radio"] {
  margin: 0;
  cursor: pointer;
}
#attest-dialog label img.attest-verdict-icon {
  width: 16px;
  height: 16px;
}
.attest-category {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.attest-category-label {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: #999;
  margin-top: 4px;
}
#attest-key-select {
  width: 100%;
  background: #2d2d2d;
  color: #e0e0e0;
  border: 1px solid #555;
  border-radius: 4px;
  padding: 6px 8px;
  font-size: 13px;
  cursor: pointer;
}
#attest-key-select:focus {
  outline: none;
  border-color: #888;
}
.attest-btn-row {
  display: flex;
  flex-direction: row;
  justify-content: flex-end;
  gap: 8px;
}
.attest-btn-row button {
  background: #2d2d2d;
  color: #e0e0e0;
  border: 1px solid #555;
  border-radius: 4px;
  padding: 6px 16px;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.1s;
}
.attest-btn-row button:hover {
  background: #3a3a3a;
  border-color: #888;
}
.attest-btn-row button.attest-sign-btn {
  background: #1a6b3a;
  border-color: #2a9b5a;
  color: #fff;
}
.attest-btn-row button.attest-sign-btn:hover {
  background: #1d8045;
}
.attest-btn-row button.attest-sign-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.attest-status {
  font-size: 12px;
  color: #888;
  min-height: 16px;
}
.attest-status.error { color: #ef5350; }
.attest-status.success { color: #66bb6a; }
`;

async function openAttestDialog(imageUrl) {
  if (activeDialog) closeAttestDialog();

  const host = document.createElement('div');
  host.dataset.attestDialog = '';
  host.style.cssText = 'position:fixed;top:0;left:0;width:0;height:0;z-index:2147483646;';
  const shadow = host.attachShadow({ mode: 'closed' });

  const style = document.createElement('style');
  style.textContent = DIALOG_CSS;
  shadow.appendChild(style);

  // Fetch keys and existing verdicts for this image
  let keys = [];
  let selectedKeyID = null;
  let before = { authorship: null, method: null, authenticity: null };
  try {
    const [keysResp, keyResp, myVerdictsResp] = await Promise.all([
      sendBg({ type: "list_secret_keys" }),
      sendBg({ type: "get_key" }),
      sendBg({ type: "get_my_verdicts", url: imageUrl }),
    ]);
    keys = (keysResp.keys || []).filter(k => k.can_sign);
    selectedKeyID = keyResp.keyID;
    if (myVerdictsResp && myVerdictsResp.myVerdicts) {
      before = { ...before, ...myVerdictsResp.myVerdicts };
    }
  } catch (e) {
    console.error("[attestension] failed to load keys:", e);
  }

  const overlay = document.createElement('div');
  overlay.id = 'attest-dialog-overlay';

  const dialog = document.createElement('div');
  dialog.id = 'attest-dialog';

  // Title
  const title = document.createElement('h3');
  title.textContent = 'Attest';
  dialog.appendChild(title);

  // Key selector
  const keySelect = document.createElement('select');
  keySelect.id = 'attest-key-select';
  if (keys.length === 0) {
    const opt = document.createElement('option');
    opt.textContent = 'No signing keys available';
    opt.disabled = true;
    keySelect.appendChild(opt);
  } else {
    for (const k of keys) {
      const opt = document.createElement('option');
      opt.value = k.fingerprint;
      opt.textContent = k.uid || k.fingerprint;
      if (k.fingerprint === selectedKeyID) opt.selected = true;
      keySelect.appendChild(opt);
    }
  }
  dialog.appendChild(keySelect);

  // Authorship category
  const authorshipDiv = document.createElement('div');
  authorshipDiv.className = 'attest-category';
  const authorshipLabel = document.createElement('div');
  authorshipLabel.className = 'attest-category-label';
  authorshipLabel.textContent = 'Authorship';
  authorshipDiv.appendChild(authorshipLabel);
  const authorshipCheck = document.createElement('label');
  const authorshipInput = document.createElement('input');
  authorshipInput.type = 'checkbox';
  authorshipInput.name = 'authorship';
  authorshipInput.value = 'my-work';
  authorshipCheck.appendChild(authorshipInput);
  const authorshipIcon = document.createElement('img');
  authorshipIcon.className = 'attest-verdict-icon';
  authorshipIcon.src = chrome.runtime.getURL('icons/authorship-my-work-16.png');
  authorshipCheck.appendChild(authorshipIcon);
  authorshipCheck.appendChild(document.createTextNode('I created this'));
  if (before.authorship) authorshipInput.checked = true;
  authorshipDiv.appendChild(authorshipCheck);
  dialog.appendChild(authorshipDiv);

  // Method category
  const methodDiv = document.createElement('div');
  methodDiv.className = 'attest-category';
  const methodLabel = document.createElement('div');
  methodLabel.className = 'attest-category-label';
  methodLabel.textContent = 'Method';
  methodDiv.appendChild(methodLabel);
  const methodCheck = document.createElement('label');
  const methodInput = document.createElement('input');
  methodInput.type = 'checkbox';
  methodInput.name = 'method';
  methodInput.value = 'ai-generated';
  methodCheck.appendChild(methodInput);
  const methodIcon = document.createElement('img');
  methodIcon.className = 'attest-verdict-icon';
  methodIcon.src = chrome.runtime.getURL('icons/method-ai-generated-16.png');
  methodCheck.appendChild(methodIcon);
  methodCheck.appendChild(document.createTextNode('AI-generated'));
  if (before.method) methodInput.checked = true;
  methodDiv.appendChild(methodCheck);
  dialog.appendChild(methodDiv);

  // Authenticity category (radio buttons)
  const authDiv = document.createElement('div');
  authDiv.className = 'attest-category';
  const authLabel = document.createElement('div');
  authLabel.className = 'attest-category-label';
  authLabel.textContent = 'Authenticity';
  authDiv.appendChild(authLabel);

  const authValues = [
    { value: 'authentic', label: 'Authentic' },
    { value: 'satire', label: 'Satire' },
    { value: 'misleading', label: 'Misleading' },
  ];
  const authRadios = [];
  for (const { value, label } of authValues) {
    const radioLabel = document.createElement('label');
    const radio = document.createElement('input');
    radio.type = 'radio';
    radio.name = 'authenticity';
    radio.value = value;
    authRadios.push(radio);
    // Allow deselecting radio by clicking again
    radio.addEventListener('click', () => {
      if (radio._wasChecked) {
        radio.checked = false;
        radio._wasChecked = false;
      } else {
        authRadios.forEach(r => r._wasChecked = false);
        radio._wasChecked = true;
      }
    });
    radioLabel.appendChild(radio);
    const radioIcon = document.createElement('img');
    radioIcon.className = 'attest-verdict-icon';
    radioIcon.src = chrome.runtime.getURL(`icons/authenticity-${value}-16.png`);
    radioLabel.appendChild(radioIcon);
    radioLabel.appendChild(document.createTextNode(label));
    authDiv.appendChild(radioLabel);
  }
  // Pre-populate authenticity radio from existing verdict
  if (before.authenticity) {
    const match = authRadios.find(r => r.value === before.authenticity);
    if (match) {
      match.checked = true;
      match._wasChecked = true;
    }
  }
  dialog.appendChild(authDiv);

  // Status line
  const statusEl = document.createElement('div');
  statusEl.className = 'attest-status';
  dialog.appendChild(statusEl);

  // Buttons
  const btnRow = document.createElement('div');
  btnRow.className = 'attest-btn-row';
  const cancelBtn = document.createElement('button');
  cancelBtn.textContent = 'Cancel';
  cancelBtn.addEventListener('click', closeAttestDialog);
  const signBtn = document.createElement('button');
  signBtn.textContent = 'Sign';
  signBtn.className = 'attest-sign-btn';
  signBtn.disabled = keys.length === 0;
  signBtn.addEventListener('click', async () => {
    const keyID = keySelect.value;
    if (!keyID) return;

    // Save key selection
    sendBg({ type: "set_key", keyID });

    // Compute current UI state
    const after = {
      authorship: authorshipInput.checked ? 'my-work' : null,
      method: methodInput.checked ? 'ai-generated' : null,
      authenticity: (authRadios.find(r => r.checked) || {}).value || null,
    };

    // Diff before vs after to determine actions
    const actions = [];
    for (const cat of ['authorship', 'method', 'authenticity']) {
      if (before[cat] === after[cat]) continue; // no change
      if (after[cat] !== null) {
        // New verdict or changed verdict (supersedes old)
        actions.push({ category: cat, verdict: after[cat] });
      } else {
        // Was set, now unset → revoke
        actions.push({ category: cat, verdict: 'revoke' });
      }
    }

    if (actions.length === 0) {
      statusEl.textContent = 'No changes.';
      statusEl.className = 'attest-status';
      return;
    }

    signBtn.disabled = true;
    statusEl.textContent = 'Signing...';
    statusEl.className = 'attest-status';

    let successCount = 0;
    for (const { category, verdict } of actions) {
      try {
        const result = await sendBg({
          type: "sign",
          url: imageUrl,
          keyID,
          category,
          verdict,
        });
        if (result.ok) {
          successCount++;
          console.log(`[attestension] signed ${category}:${verdict} → ${result.uuid}`);
        } else {
          statusEl.textContent = result.error || 'Sign failed';
          statusEl.className = 'attest-status error';
          signBtn.disabled = false;
          return;
        }
      } catch (err) {
        statusEl.textContent = err.message;
        statusEl.className = 'attest-status error';
        signBtn.disabled = false;
        return;
      }
    }

    statusEl.textContent = `Signed ${successCount} verdict${successCount > 1 ? 's' : ''}.`;
    statusEl.className = 'attest-status success';

    // Refresh badges for this image
    document.querySelectorAll("img").forEach(img => {
      if (img.currentSrc === imageUrl || img.src === imageUrl) {
        processedImages.delete(img);
        processSingleImage(img);
      }
    });

    setTimeout(closeAttestDialog, 800);
  });
  btnRow.appendChild(cancelBtn);
  btnRow.appendChild(signBtn);
  dialog.appendChild(btnRow);

  overlay.appendChild(dialog);
  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) closeAttestDialog();
  });
  shadow.appendChild(overlay);
  document.body.appendChild(host);

  activeDialog = host;
}

function closeAttestDialog() {
  if (activeDialog) {
    activeDialog.remove();
    activeDialog = null;
  }
}

// --- Message listener ---

chrome.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  if (msg.type === "attest_result") {
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
  } else if (msg.type === "open_attest_dialog") {
    openAttestDialog(msg.url);
  } else if (msg.type === "get_context_image") {
    sendResponse(lastContextImageUrl);
  } else if (msg.type === "attest_warn") {
    console.warn("[attestension]", msg.message);
  } else if (msg.type === "attest_error") {
    console.error("[attestension]", msg.message);
  }
});

// --- Multi-category badge overlay ---

const CATEGORY_ORDER = ["authorship", "method", "authenticity"];
const processedImages = new WeakSet();
const processedBackgrounds = new WeakSet();
const bgUrlToElements = new Map(); // url -> Set<Element>

// --- Per-image badge overlays (horizontal 16px badges, one per category) ---

const BADGE_CSS = '.attest-badge{position:fixed;top:0;left:0;width:14px;height:14px;pointer-events:none;}';

const badgeRegistry = new Map();       // badgeId -> { targetRef, badgeEl, host, visible, category }
const targetToBadges = new WeakMap();  // Element -> Map<category, badgeId>
let nextBadgeId = 0;

const cleanupRegistry = new FinalizationRegistry(badgeIds => {
  for (const badgeId of badgeIds) removeBadge(badgeId);
});

function createBadges(targetEl, categories) {
  const existing = targetToBadges.get(targetEl);

  // Determine which categories have verdicts
  const activeCats = CATEGORY_ORDER.filter(cat => categories[cat]);

  if (activeCats.length === 0) {
    if (existing) removeBadgesForTarget(targetEl);
    return;
  }

  if (!targetEl.parentElement) return;

  // Remove badges for categories no longer present
  if (existing) {
    const toRemove = [...existing.entries()].filter(([cat]) => !categories[cat]);
    for (const [, badgeId] of toRemove) {
      removeBadge(badgeId);
    }
  }

  const badgeMap = existing || new Map();
  if (!existing) targetToBadges.set(targetEl, badgeMap);

  let catIndex = 0;
  for (const cat of activeCats) {
    const info = categories[cat];
    const iconFile = info.icon;
    if (!iconFile) { catIndex++; continue; }

    const iconUrl = chrome.runtime.getURL(`icons/${iconFile}-14.png`);
    const title = `${cat}: ${info.verdict} (${info.signers} signer${info.signers > 1 ? 's' : ''})`;

    const existingBadgeId = badgeMap.get(cat);
    if (existingBadgeId !== undefined) {
      // Update existing badge
      const record = badgeRegistry.get(existingBadgeId);
      if (record) {
        record.badgeEl.src = iconUrl;
        record.badgeEl.alt = title;
        record.badgeEl.title = title;
        record.catIndex = catIndex;
      }
      catIndex++;
      continue;
    }

    // Create new badge
    const badgeId = nextBadgeId++;
    const badgeEl = document.createElement('img');
    badgeEl.className = 'attest-badge';
    badgeEl.src = iconUrl;
    badgeEl.alt = title;
    badgeEl.title = title;
    badgeEl.style.display = 'none';

    const host = document.createElement('div');
    host.dataset.attestBadge = '';
    host.style.cssText = 'position:fixed;top:0;left:0;width:0;height:0;overflow:visible;pointer-events:none;';
    const shadow = host.attachShadow({ mode: 'closed' });
    const styleEl = document.createElement('style');
    styleEl.textContent = BADGE_CSS;
    shadow.appendChild(styleEl);
    shadow.appendChild(badgeEl);
    targetEl.parentElement.appendChild(host);

    // If target already has other visible badges, this one should be visible too
    const alreadyObserved = badgeMap.size > 0;
    const isVisible = alreadyObserved && [...badgeMap.values()].some(
      id => badgeRegistry.get(id)?.visible
    );
    if (isVisible) {
      badgeEl.style.display = '';
    }

    const record = { targetRef: new WeakRef(targetEl), badgeEl, host, visible: isVisible, category: cat, catIndex };
    badgeRegistry.set(badgeId, record);
    badgeMap.set(cat, badgeId);

    if (!alreadyObserved) {
      // First badge for this target — set up observers
      intersectionObs.observe(targetEl);
      resizeObs.observe(targetEl);
      cleanupRegistry.register(targetEl, [...badgeMap.values()]);
    }

    catIndex++;
  }

  // Always reposition after badge changes (additions, removals, reindexing)
  scheduleUpdate();
}

function removeBadge(badgeId) {
  const record = badgeRegistry.get(badgeId);
  if (!record) return;
  record.host.remove();
  badgeRegistry.delete(badgeId);
  const target = record.targetRef.deref();
  if (target) {
    const badgeMap = targetToBadges.get(target);
    if (badgeMap) {
      badgeMap.delete(record.category);
      if (badgeMap.size === 0) {
        targetToBadges.delete(target);
        intersectionObs.unobserve(target);
        resizeObs.unobserve(target);
      }
    }
  }
}

function removeBadgesForTarget(el) {
  const badgeMap = targetToBadges.get(el);
  if (!badgeMap) return;
  for (const badgeId of [...badgeMap.values()]) {
    const record = badgeRegistry.get(badgeId);
    if (record) record.host.remove();
    badgeRegistry.delete(badgeId);
  }
  targetToBadges.delete(el);
  intersectionObs.unobserve(el);
  resizeObs.unobserve(el);
}

// --- Visible content rect (accounts for object-fit) ---

function parsePosValue(val, availableSpace) {
  if (val.endsWith('%')) return (parseFloat(val) / 100) * availableSpace;
  if (val.endsWith('px')) return parseFloat(val);
  if (val === 'left' || val === 'top') return 0;
  if (val === 'right' || val === 'bottom') return availableSpace;
  return availableSpace / 2;
}

function getVisibleContentRect(target) {
  const elemRect = target.getBoundingClientRect();
  if (target.tagName !== 'IMG' || !target.naturalWidth || !target.naturalHeight) {
    return elemRect;
  }
  const style = getComputedStyle(target);
  const fit = style.objectFit;
  if (fit !== 'contain' && fit !== 'scale-down') return elemRect;
  const natW = target.naturalWidth;
  const natH = target.naturalHeight;
  const elemW = elemRect.width;
  const elemH = elemRect.height;
  if (fit === 'scale-down' && natW <= elemW && natH <= elemH) return elemRect;
  const scale = Math.min(elemW / natW, elemH / natH);
  const contentW = natW * scale;
  const contentH = natH * scale;
  const pos = style.objectPosition;
  let offsetX = (elemW - contentW) / 2;
  let offsetY = (elemH - contentH) / 2;
  if (pos) {
    const parts = pos.split(/\s+/);
    if (parts.length >= 2) {
      offsetX = parsePosValue(parts[0], elemW - contentW);
      offsetY = parsePosValue(parts[1], elemH - contentH);
    }
  }
  return {
    top:    elemRect.top + offsetY,
    left:   elemRect.left + offsetX,
    right:  elemRect.left + offsetX + contentW,
    bottom: elemRect.top + offsetY + contentH,
    width:  contentW,
    height: contentH,
  };
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
    const targetRect = getVisibleContentRect(target);
    const hostRect = record.host.getBoundingClientRect();
    updates.push({
      badgeEl: record.badgeEl,
      top: targetRect.top - hostRect.top + 2,
      left: targetRect.right - hostRect.left - 14 - 2 - (record.catIndex * 16),
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
    const badgeMap = targetToBadges.get(entry.target);
    if (!badgeMap) continue;
    for (const badgeId of badgeMap.values()) {
      const record = badgeRegistry.get(badgeId);
      if (!record) continue;
      record.visible = entry.isIntersecting;
      record.badgeEl.style.display = entry.isIntersecting ? '' : 'none';
    }
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
  if (!result || !result.categories || Object.keys(result.categories).length === 0) return;

  createBadges(img, result.categories);
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
  if (!result || !result.categories || Object.keys(result.categories).length === 0) return;

  createBadges(el, result.categories);
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
      removeBadgesForTarget(node);
      for (const [url, elSet] of bgUrlToElements) {
        elSet.delete(node);
        if (elSet.size === 0) bgUrlToElements.delete(url);
      }
      if (node.querySelectorAll) {
        node.querySelectorAll('img').forEach(img => removeBadgesForTarget(img));
        node.querySelectorAll('*').forEach(el => {
          removeBadgesForTarget(el);
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
