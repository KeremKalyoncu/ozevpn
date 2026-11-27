const $ = (id) => document.getElementById(id);
let appConfig = null;
let engineStatus = null;
let installStatus = null;
let lastNetInfo = null;
let lastProbeReport = null;
let busy = false;
let consecutiveStatusFails = 0;
let statusSeq = 0;
let statusInFlight = false;
let pollTimer = null;
let maskSensitiveIP = false;
let relaunchingAdmin = false;
let appShuttingDown = false;
let wizardInstallChoice = 'resident';
let wizardModeChoice = 'full';
let activeModal = null;
let lastFocusedBeforeModal = null;
let confirmResolve = null;
try {
  maskSensitiveIP = localStorage.getItem('ozevpn_mask_ip') === '1';
} catch {}
const els = {
  appShell: $('appShell'),
  offlineBanner: $('offlineBanner'),
  offlineBannerText: $('offlineBannerText'),
  retryConnBtn: $('retryConnBtn'),
  themeToggleBtn: $('themeToggleBtn'),
  themeIcon: $('themeIcon'),
  maskIpBtn: $('maskIpBtn'),
  maskIpIcon: $('maskIpIcon'),
  maskIpLabel: $('maskIpLabel'),
  advancedSettings: $('advancedSettings'),
  toolsMenuWrap: $('toolsMenuWrap'),
  toolsMenuBtn: $('toolsMenuBtn'),
  toolsMenu: $('toolsMenu'),
  toastStack: $('toastStack'),
  wizardOverlay: $('wizardOverlay'),
  wizardInstallChoices: $('wizardInstallChoices'),
  wizardModeChoices: $('wizardModeChoices'),
  wizardError: $('wizardError'),
  wizardSkipBtn: $('wizardSkipBtn'),
  wizardFinishBtn: $('wizardFinishBtn'),
  confirmOverlay: $('confirmOverlay'),
  confirmTitle: $('confirmTitle'),
  confirmDesc: $('confirmDesc'),
  confirmCancelBtn: $('confirmCancelBtn'),
  confirmOkBtn: $('confirmOkBtn'),
  openWizardBtn: $('openWizardBtn'),
  quitAppBtn: $('quitAppBtn'),
  newIpBtn: $('newIpBtn'),
  relaunchAdminBtn: $('relaunchAdminBtn'),
  emergencyResetBtn: $('emergencyResetBtn'),
  adBlockToggle: $('adBlockToggle'),
  killSwitchToggle: $('killSwitchToggle'),
  blockLanToggle: $('blockLanToggle'),
  disableNetProbeToggle: $('disableNetProbeToggle'),
  torBridgeToggle: $('torBridgeToggle'),
  clearServerBtn: $('clearServerBtn'),
  speedBtn: $('speedBtn'),
  speedStrip: $('speedStrip'),
  speedMbps: $('speedMbps'),
  speedPing: $('speedPing'),
  speedJitter: $('speedJitter'),
  speedPath: $('speedPath'),
  rescueInput: $('rescueInput'),
  rescueTestBtn: $('rescueTestBtn'),
  rescueAddBtn: $('rescueAddBtn'),
  rescueResult: $('rescueResult'),
  brandDot: $('brandDot'),
  installBadge: $('installBadge'),
  protoBadge: $('protoBadge'),
  coloBadge: $('coloBadge'),
  routeSchematic: $('routeSchematic'),
  nodeDeviceVal: $('nodeDeviceVal'),
  nodeTunnelVal: $('nodeTunnelVal'),
  uptimeBadge: $('uptimeBadge'),
  powerBtn: $('powerBtn'),
  statusText: $('statusText'),
  statusSub: $('statusSub'),
  modeGrid: $('modeGrid'),
  engineProfileSelect: $('engineProfileSelect'),
  protocolSelect: $('protocolSelect'),
  customServerBox: $('customServerBox'),
  customServerInput: $('customServerInput'),
  customServerErr: $('customServerErr'),
  saveServerBtn: $('saveServerBtn'),
  residentTitle: $('residentTitle'),
  residentSub: $('residentSub'),
  toggleResidentBtn: $('toggleResidentBtn'),
  logBox: $('logBox'),
  modemHint: $('modemHint'),
  dnsInfo: $('dnsInfo'),
  ipv6Info: $('ipv6Info'),
  probeBtn: $('probeBtn'),
  probeGrid: $('probeGrid'),
  traceSummary: $('traceSummary'),
  bypassPresets: $('bypassPresets'),
  tunnelPresets: $('tunnelPresets'),
  customBypassInput: $('customBypassInput'),
  bypassErr: $('bypassErr'),
  addBypassBtn: $('addBypassBtn'),
  customBypassChips: $('customBypassChips'),
  customTunnelInput: $('customTunnelInput'),
  tunnelErr: $('tunnelErr'),
  addTunnelBtn: $('addTunnelBtn'),
  customTunnelChips: $('customTunnelChips'),
};
function setUseIcon(svgEl, iconId) {
  if (!svgEl) return;
  const useEl = svgEl.querySelector('use');
  if (useEl) useEl.setAttribute('href', '#' + iconId);
}
function createSVGIcon(iconId, cls = 'ico') {
  const ns = 'http://www.w3.org/2000/svg';
  const svg = document.createElementNS(ns, 'svg');
  svg.setAttribute('class', cls);
  svg.setAttribute('aria-hidden', 'true');
  const use = document.createElementNS(ns, 'use');
  use.setAttribute('href', '#' + iconId);
  svg.appendChild(use);
  return svg;
}
function nowStamp() {
  try {
    return new Date().toLocaleTimeString('tr-TR', { hour12: false });
  } catch {
    return '';
  }
}
function formatDuration(sec) {
  const s = Math.max(0, Math.floor(Number(sec) || 0));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const rem = s % 60;
  const pad = (n) => String(n).padStart(2, '0');
  return h > 0 ? `${pad(h)}:${pad(m)}:${pad(rem)}` : `${pad(m)}:${pad(rem)}`;
}
function addLog(msg) {
  if (!els.logBox) return;
  const div = document.createElement('div');
  div.className = 'log-line';
  const text = String(msg || '');
  div.textContent = /^\[\d{2}:\d{2}:\d{2}\]/.test(text) ? text : `[${nowStamp()}] ${text}`;
  els.logBox.appendChild(div);
  while (els.logBox.children.length > 200) {
    els.logBox.removeChild(els.logBox.firstChild);
  }
  els.logBox.scrollTop = els.logBox.scrollHeight;
}
const activeToasts = new Map();
function showToast(msg, kind = 'info', dedupeKey = '') {
  if (!els.toastStack || !msg) return;
  const key = dedupeKey || `${kind}:${msg}`;
  if (activeToasts.has(key)) {
    const existing = activeToasts.get(key);
    existing.msgEl.textContent = msg;
    existing.resetTimer();
    return;
  }
  while (els.toastStack.children.length >= 3) {
    const oldest = els.toastStack.firstElementChild;
    if (oldest && oldest.dataset.toastKey) {
      const entry = activeToasts.get(oldest.dataset.toastKey);
      if (entry) entry.dismiss();
      else oldest.remove();
    } else break;
  }
  const toast = document.createElement('div');
  toast.className = `toast ${kind}`;
  toast.dataset.toastKey = key;
  toast.setAttribute('role', kind === 'error' ? 'alert' : 'status');
  const iconId = kind === 'ok' ? 'ico-check' : kind === 'error' ? 'ico-alert' : 'ico-shield';
  const ico = createSVGIcon(iconId, 'ico toast-ico');
  const msgEl = document.createElement('span');
  msgEl.className = 'toast-msg';
  msgEl.textContent = msg;
  const closeBtn = document.createElement('button');
  closeBtn.type = 'button';
  closeBtn.className = 'toast-close';
  closeBtn.setAttribute('aria-label', 'Bildirimi kapat');
  closeBtn.appendChild(createSVGIcon('ico-x', 'ico ico-xs'));
  let timer = null;
  const dismiss = () => {
    if (timer) clearTimeout(timer);
    activeToasts.delete(key);
    toast.remove();
  };
  const resetTimer = () => {
    if (timer) clearTimeout(timer);
    // Errors remain until explicitly dismissed so critical feedback is never lost
    if (kind !== 'error') {
      timer = setTimeout(dismiss, 3800);
    }
  };
  closeBtn.addEventListener('click', dismiss);
  toast.addEventListener('mouseenter', () => { if (timer) clearTimeout(timer); });
  toast.addEventListener('mouseleave', resetTimer);
  toast.addEventListener('focusin', () => { if (timer) clearTimeout(timer); });
  toast.addEventListener('focusout', resetTimer);
  toast.append(ico, msgEl, closeBtn);
  els.toastStack.appendChild(toast);
  activeToasts.set(key, { el: toast, msgEl, dismiss, resetTimer });
  resetTimer();
}
window.addEventListener('error', (e) => {
  if (e && e.message) {
    addLog('Beklenmeyen arayüz hatası: ' + e.message);
    showToast('Beklenmeyen arayüz hatası: ' + e.message, 'error');
  }
});
window.addEventListener('unhandledrejection', (e) => {
  const reason = e && e.reason ? (e.reason.message || String(e.reason)) : 'Bilinmeyen hata';
  addLog('Beklenmeyen işlem hatası: ' + reason);
  showToast('İşlem hatası: ' + reason, 'error');
});
function showInlineError(el, inputEl, msg) {
  if (el) {
    el.textContent = msg || '';
    el.classList.toggle('hidden', !msg);
  }
  if (inputEl) {
    if (msg) inputEl.setAttribute('aria-invalid', 'true');
    else inputEl.removeAttribute('aria-invalid');
  }
}
function setToolsMenuOpen(open, restoreFocus = false) {
  if (!els.toolsMenu || !els.toolsMenuBtn) return;
  els.toolsMenu.classList.toggle('hidden', !open);
  els.toolsMenuBtn.setAttribute('aria-expanded', String(open));
  if (open) {
    const first = els.toolsMenu.querySelector('.menu-item:not([disabled])');
    if (first) first.focus();
  } else if (restoreFocus) {
    els.toolsMenuBtn.focus();
  }
}
if (els.toolsMenuBtn && els.toolsMenu) {
  els.toolsMenuBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    const isOpen = !els.toolsMenu.classList.contains('hidden');
    setToolsMenuOpen(!isOpen, false);
  });
  els.toolsMenu.addEventListener('keydown', (e) => {
    const items = Array.from(els.toolsMenu.querySelectorAll('.menu-item:not([disabled])'));
    const idx = items.indexOf(document.activeElement);
    if (e.key === 'Escape') {
      e.preventDefault();
      setToolsMenuOpen(false, true);
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      items[(idx + 1) % items.length]?.focus();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      items[(idx - 1 + items.length) % items.length]?.focus();
    } else if (e.key === 'Home') {
      e.preventDefault();
      items[0]?.focus();
    } else if (e.key === 'End') {
      e.preventDefault();
      items[items.length - 1]?.focus();
    }
  });
  document.addEventListener('click', (e) => {
    if (els.toolsMenuWrap && !els.toolsMenuWrap.contains(e.target)) {
      if (!els.toolsMenu.classList.contains('hidden')) setToolsMenuOpen(false, false);
    }
  });
}
function applyTheme(theme, persist) {
  const t = theme === 'light' ? 'light' : 'dark';
  document.documentElement.dataset.theme = t;
  if (persist) {
    try { localStorage.setItem('ozevpn_theme', t); } catch {}
  }
  if (els.themeToggleBtn) {
    const isLight = t === 'light';
    setUseIcon(els.themeIcon, isLight ? 'ico-moon' : 'ico-sun');
    els.themeToggleBtn.title = isLight ? 'Koyu temaya geç' : 'Açık temaya geç';
    els.themeToggleBtn.setAttribute('aria-label', isLight ? 'Koyu temaya geç' : 'Açık temaya geç');
    els.themeToggleBtn.setAttribute('aria-pressed', String(isLight));
  }
}
applyTheme(document.documentElement.dataset.theme || 'dark', false);
if (els.themeToggleBtn) {
  els.themeToggleBtn.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
    applyTheme(next, true);
    showToast(next === 'light' ? 'Açık tema etkinleştirildi.' : 'Koyu tema etkinleştirildi.', 'info', 'theme');
  });
}
if (window.matchMedia) {
  window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', (e) => {
    let saved = null;
    try { saved = localStorage.getItem('ozevpn_theme'); } catch {}
    if (!saved) applyTheme(e.matches ? 'light' : 'dark', false);
  });
}
function updateMaskButtonUI() {
  if (!els.maskIpBtn) return;
  if (els.maskIpLabel) els.maskIpLabel.textContent = maskSensitiveIP ? 'IP Gizli' : 'IP Gizle';
  setUseIcon(els.maskIpIcon, maskSensitiveIP ? 'ico-eye-off' : 'ico-eye');
  els.maskIpBtn.setAttribute('aria-pressed', String(maskSensitiveIP));
}
function renderNetInfo() {
  if (maskSensitiveIP) {
    const ctry = lastNetInfo && lastNetInfo.country && lastNetInfo.country !== 'AUTO' ? ` · ${lastNetInfo.country}` : '';
    const hint = `Yerel Ağ Gizli${ctry}`;
    els.modemHint.textContent = hint;
    els.modemHint.title = hint;
    els.dnsInfo.textContent = 'Maskeli';
    els.dnsInfo.title = 'Maskeli';
    els.ipv6Info.textContent = lastNetInfo ? (lastNetInfo.ipv6_active ? 'Aktif' : 'Kapalı') : 'Gizli';
    return;
  }
  if (!lastNetInfo) {
    els.modemHint.textContent = 'Taranıyor...';
    els.dnsInfo.textContent = '—';
    els.ipv6Info.textContent = '—';
    return;
  }
  const ni = lastNetInfo;
  const ctry = ni.country && ni.country !== 'AUTO' ? ` · ${ni.country}` : '';
  const hint = (ni.modem_hint || 'Tespit edilemedi') + ctry;
  const dns = (ni.dns_servers && ni.dns_servers.length) ? ni.dns_servers.slice(0, 2).join(', ') : 'Otomatik';
  els.modemHint.textContent = hint;
  els.modemHint.title = hint;
  els.dnsInfo.textContent = dns;
  els.dnsInfo.title = dns;
  els.ipv6Info.textContent = ni.ipv6_active ? 'Aktif' : 'Kapalı';
}
function renderProbeSummary() {
  if (!lastProbeReport) return;
  const rep = lastProbeReport;
  const t = rep.trace || {};
  if (t.ip) {
    const viaTxt = rep.via_proxy ? 'OzeVPN Tüneli' : 'Direkt Hat';
    const shownIP = maskSensitiveIP ? '•••.•••.•••.•••' : t.ip;
    els.coloBadge.textContent = `${t.colo || '—'} · ${t.loc || '—'}`;
    els.traceSummary.textContent = `IP: ${shownIP} · Bölge: ${t.loc || '?'} · Sunucu: ${t.colo || '?'} · ${viaTxt} (${rep.timestamp})`;
  } else {
    els.traceSummary.textContent = 'Trace bilgisi alınamadı.';
  }
}
updateMaskButtonUI();
renderNetInfo();
if (els.maskIpBtn) {
  els.maskIpBtn.addEventListener('click', () => {
    maskSensitiveIP = !maskSensitiveIP;
    try { localStorage.setItem('ozevpn_mask_ip', maskSensitiveIP ? '1' : '0'); } catch {}
    updateMaskButtonUI();
    renderNetInfo();
    renderProbeSummary();
    showToast(maskSensitiveIP ? 'IP ve modem bilgileri maskelendi.' : 'IP maskeleme kapatıldı.', 'info', 'ip-mask');
  });
}
function setAppInert(isInert) {
  if (els.appShell && 'inert' in els.appShell) {
    els.appShell.inert = isInert;
  }
}
function getFocusableElements(container) {
  return Array.from(container.querySelectorAll(
    'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
  )).filter(el => el.offsetParent !== null && !el.closest('.hidden'));
}
function openModal(overlayEl, triggerEl) {
  setToolsMenuOpen(false, false);
  const candidate = triggerEl || document.activeElement;
  // Never store skip-link, body, or hidden elements as focus return target (fixes stuck skip-link bug)
  if (candidate && candidate !== document.body && !candidate.classList?.contains('skip-link') && els.appShell?.contains(candidate)) {
    lastFocusedBeforeModal = candidate;
  } else {
    lastFocusedBeforeModal = els.powerBtn;
  }
  activeModal = overlayEl;
  overlayEl.classList.remove('hidden');
  setAppInert(true);
  const focusables = getFocusableElements(overlayEl);
  if (focusables.length > 0) {
    focusables[0].focus();
  } else {
    const card = overlayEl.querySelector('.wizard-card');
    if (card) card.focus();
  }
}
function closeModal(overlayEl) {
  overlayEl.classList.add('hidden');
  if (activeModal === overlayEl) activeModal = null;
  const anyOpen = !els.wizardOverlay.classList.contains('hidden') || !els.confirmOverlay.classList.contains('hidden');
  if (!anyOpen) setAppInert(false);
  const target = (lastFocusedBeforeModal && !lastFocusedBeforeModal.classList?.contains('skip-link') && lastFocusedBeforeModal.offsetParent !== null)
    ? lastFocusedBeforeModal
    : els.powerBtn;
  if (target && typeof target.focus === 'function') {
    target.focus({ preventScroll: true });
  }
  lastFocusedBeforeModal = null;
}
document.addEventListener('keydown', (e) => {
  if (activeModal) {
    if (e.key === 'Escape') {
      e.preventDefault();
      if (activeModal === els.confirmOverlay) finishConfirm(false);
      else if (activeModal === els.wizardOverlay) closeModal(els.wizardOverlay);
      return;
    }
    if (e.key === 'Tab') {
      const focusables = getFocusableElements(activeModal);
      if (focusables.length === 0) return;
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      if (e.shiftKey) {
        if (document.activeElement === first || !activeModal.contains(document.activeElement)) {
          e.preventDefault();
          last.focus();
        }
      } else {
        if (document.activeElement === last || !activeModal.contains(document.activeElement)) {
          e.preventDefault();
          first.focus();
        }
      }
    }
    return;
  }
  // Global keyboard shortcuts (FE-15-03) — ignored while typing in an input/select
  const tag = (document.activeElement && document.activeElement.tagName || '').toLowerCase();
  const isTyping = tag === 'input' || tag === 'textarea' || tag === 'select';
  if (e.altKey && !e.ctrlKey && !e.metaKey && e.key.toLowerCase() === 'c') {
    e.preventDefault();
    togglePower();
  } else if (e.altKey && !e.ctrlKey && !e.metaKey && e.key.toLowerCase() === 't') {
    e.preventDefault();
    runProbe();
  } else if (!isTyping && !e.ctrlKey && !e.altKey && !e.metaKey && e.key === '/' && els.rescueInput) {
    e.preventDefault();
    els.rescueInput.focus();
  }
});
els.wizardOverlay.addEventListener('click', (e) => {
  if (e.target === els.wizardOverlay) closeModal(els.wizardOverlay);
});
function askConfirm(title, desc, okLabel, triggerEl) {
  return new Promise((resolve) => {
    confirmResolve = resolve;
    els.confirmTitle.textContent = title;
    els.confirmDesc.textContent = desc;
    els.confirmOkBtn.textContent = okLabel || 'Onayla';
    openModal(els.confirmOverlay, triggerEl);
  });
}
function finishConfirm(answer) {
  const r = confirmResolve;
  confirmResolve = null;
  closeModal(els.confirmOverlay);
  if (r) r(answer);
}
els.confirmCancelBtn.addEventListener('click', () => finishConfirm(false));
els.confirmOkBtn.addEventListener('click', () => finishConfirm(true));
els.confirmOverlay.addEventListener('click', (e) => {
  if (e.target === els.confirmOverlay) finishConfirm(false);
});
function getSessionToken() {
  const m = document.querySelector('meta[name="ozevpn-token"]');
  return m ? (m.getAttribute('content') || '').trim() : '';
}
async function api(path, opts = {}, timeoutMs = 15000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const hdrs = { 'Content-Type': 'application/json', ...(opts.headers || {}) };
    const tok = getSessionToken();
    if (tok) hdrs['X-OzeVPN-Token'] = tok;
    const res = await fetch(path, { ...opts, headers: hdrs, signal: controller.signal });
    const raw = await res.text();
    let data = {};
    if (raw) {
      try {
        data = JSON.parse(raw);
      } catch {
        data = { error: res.ok ? '' : 'Sunucu beklenmeyen bir yanıt döndürdü.' };
      }
    }
    if (!res.ok) {
      if (res.status === 429) {
        throw new Error(data.error || 'Çok fazla eşzamanlı istek; birkaç saniye bekleyip tekrar dene.');
      }
      throw new Error(data.error || `İstek başarısız (HTTP ${res.status})`);
    }
    return data;
  } catch (err) {
    if (err && err.name === 'AbortError') {
      throw new Error('İstek zaman aşımına uğradı; tekrar dene.');
    }
    throw err;
  } finally {
    clearTimeout(timer);
  }
}
function setOfflineState(isOffline) {
  if (!els.offlineBanner) return;
  els.offlineBanner.classList.toggle('hidden', !isOffline);
  if (isOffline) {
    els.statusText.textContent = 'Servis Yanıt Vermiyor';
    els.statusSub.textContent = 'OzeVPN arka plan sunucusuyla bağlantı kesildi.';
  }
}
async function saveConfig() {
  appConfig = await api('/api/config', {
    method: 'POST',
    body: JSON.stringify(appConfig),
  });
}
async function commitConfig(mutator, logMsg, hotReload = false) {
  if (!appConfig) return false;
  const snapshot = JSON.parse(JSON.stringify(appConfig));
  try {
    mutator(appConfig);
    renderConfig();
    await saveConfig();
    renderConfig();
    if (logMsg) addLog(logMsg);
    if (hotReload && engineStatus && engineStatus.running) {
      try {
        const r = await api('/api/reload', { method: 'POST' }, 15000);
        const msg = r.message || 'Kurallar anında yenilendi.';
        addLog(msg);
        showToast(msg, 'ok', 'hot-reload');
      } catch (rErr) {
        addLog('Canlı kural yenileme uyarısı: ' + rErr.message);
        showToast('Kural yenileme uyarısı: ' + rErr.message, 'error');
      }
    } else if (logMsg) {
      showToast(logMsg, 'ok', 'config-save');
    }
    return true;
  } catch (err) {
    appConfig = snapshot;
    renderConfig();
    addLog('Ayar kaydedilemedi: ' + err.message);
    showToast('Ayar kaydedilemedi: ' + err.message, 'error');
    return false;
  }
}
function renderInstall() {
  if (!installStatus) return;
  if (installStatus.resident_enabled) {
    els.installBadge.textContent = 'Yerleşik';
    els.residentTitle.textContent = 'Yerleşik Başlangıç: Aktif';
    els.residentSub.textContent = 'Windows her açıldığında sessizce başlar ve bağlanır.';
    els.toggleResidentBtn.textContent = 'Aç/Kapa Yap';
  } else {
    els.installBadge.textContent = 'Aç/Kapa';
    els.residentTitle.textContent = 'Yerleşik Başlangıç: Kapalı';
    els.residentSub.textContent = 'Aç/Kapa modunda — sadece sen açınca çalışır.';
    els.toggleResidentBtn.textContent = 'Yerleşik Yap';
  }
}
function renderStatus() {
  if (!engineStatus) return;
  const st = (engineStatus.state || 'Disconnected').toLowerCase();
  const isConn = st === 'connected';
  const isConnIng = st === 'connecting';
  els.powerBtn.className = 'power-btn ' + (isConn ? 'connected' : isConnIng ? 'connecting' : 'disconnected');
  els.powerBtn.setAttribute('aria-pressed', String(isConn));
  els.powerBtn.setAttribute('aria-label', isConn ? 'Bağlantıyı Kes' : isConnIng ? 'Bağlanıyor' : 'Bağlan');
  els.brandDot.className = 'brand-mark ' + (isConn ? 'connected' : isConnIng ? 'connecting' : '');
  if (els.routeSchematic) {
    els.routeSchematic.dataset.state = isConn ? 'connected' : isConnIng ? 'connecting' : 'disconnected';
  }
  // Update real-data schematic nodes
  if (els.nodeDeviceVal) {
    els.nodeDeviceVal.textContent = engineStatus.kill_switch_active ? 'Kill Switch' : 'Windows';
  }
  const layerShort = engineStatus.is_admin ? 'ozevpn0 TUN' : 'Proxy :10808';
  if (els.protoBadge) {
    els.protoBadge.textContent = layerShort;
    els.protoBadge.title = engineStatus.capture_layer || layerShort;
  }
  if (els.nodeTunnelVal) {
    if (isConn) {
      if (engineStatus.warp_native_wg) els.nodeTunnelVal.textContent = 'WARP WG UDP';
      else if (engineStatus.active_profile === 'custom') els.nodeTunnelVal.textContent = 'Özel Sunucu';
      else if (engineStatus.tor_alive) els.nodeTunnelVal.textContent = 'Tor EU';
      else els.nodeTunnelVal.textContent = 'WARP Proxy';
    } else if (isConnIng) {
      els.nodeTunnelVal.textContent = 'El Sıkışma...';
    } else {
      els.nodeTunnelVal.textContent = 'Beklemede';
    }
  }
  if (els.uptimeBadge) {
    if (isConn && engineStatus.uptime_sec > 0) {
      els.uptimeBadge.textContent = formatDuration(engineStatus.uptime_sec);
      els.uptimeBadge.classList.remove('hidden');
    } else {
      els.uptimeBadge.classList.add('hidden');
    }
  }
  if (isConn) {
    const scopeTxt = engineStatus.is_admin ? 'Tam Sistem TUN (UDP)' : 'Tarayıcı/Proxy Aktif';
    const ksTxt = engineStatus.kill_switch_active ? ' · Kill Switch Aktif' : '';
    const wgTxt = engineStatus.warp_native_wg ? 'WARP WireGuard TCP+UDP' : (engineStatus.active_profile || 'hybrid_eu');
    els.statusText.textContent = `Bağlı — ${scopeTxt}`;
    els.statusSub.textContent = `${wgTxt}${ksTxt}`;
  } else if (isConnIng) {
    els.statusText.textContent = 'Bağlanıyor...';
    els.statusSub.textContent = 'sing-box ve WARP WireGuard (TCP+UDP) el sıkışması sürüyor...';
  } else {
    els.statusText.textContent = 'Bağlı Değil';
    els.statusSub.textContent = engineStatus.last_error
      ? `Hata: ${engineStatus.last_error}`
      : 'Hazır — Tek tıkla engelsiz ve düşük pingli bağlantıya geç';
  }
  if (els.relaunchAdminBtn) {
    const labelSpan = els.relaunchAdminBtn.querySelector('.btn-label');
    if (engineStatus.is_admin) {
      if (labelSpan) labelSpan.textContent = 'TUN Aktif';
      els.relaunchAdminBtn.classList.add('tun-active');
      els.relaunchAdminBtn.disabled = true;
      els.relaunchAdminBtn.title = 'Yönetici (Oyun TUN) modu zaten aktif';
    } else {
      if (labelSpan) labelSpan.textContent = 'Oyun TUN';
      els.relaunchAdminBtn.classList.remove('tun-active');
      els.relaunchAdminBtn.disabled = false;
      els.relaunchAdminBtn.title = 'Roblox masaüstü ve oyun UDP trafiği için Yönetici (TUN) modunda aç';
    }
  }
}
function renderConfig() {
  if (!appConfig) return;
  document.querySelectorAll('.mode-row').forEach(btn => {
    const isAct = btn.dataset.mode === appConfig.mode;
    btn.classList.toggle('active', isAct);
    btn.setAttribute('aria-pressed', String(isAct));
  });
  els.engineProfileSelect.value = appConfig.engine_profile || 'hybrid_eu';
  els.protocolSelect.value = appConfig.protocol || 'Auto';
  els.customServerInput.placeholder = appConfig.custom_server_set
    ? 'Kayıtlı özel sunucu aktif (değiştirmek için yenisini yapıştır)'
    : 'vless:// veya WireGuard .conf yapıştır';
  els.customServerBox.classList.toggle('hidden', els.engineProfileSelect.value !== 'custom');
  if (els.adBlockToggle) els.adBlockToggle.checked = !!appConfig.ad_block_speed;
  if (els.killSwitchToggle) els.killSwitchToggle.checked = !!appConfig.kill_switch;
  if (els.blockLanToggle) els.blockLanToggle.checked = !!appConfig.block_lan;
  if (els.disableNetProbeToggle) els.disableNetProbeToggle.checked = !!appConfig.disable_startup_net_probe;
  if (els.clearServerBtn) els.clearServerBtn.classList.toggle('hidden', !appConfig.custom_server_set);
  if (els.torBridgeToggle) els.torBridgeToggle.checked = !!appConfig.tor_bridge;
  if (els.advancedSettings && (appConfig.engine_profile === 'custom' || appConfig.tor_bridge)) {
    els.advancedSettings.open = true;
  }
  renderPresets(els.bypassPresets, appConfig.bypass_presets);
  renderPresets(els.tunnelPresets, appConfig.tunnel_presets);
  renderChips(els.customBypassChips, appConfig.custom_bypass || [], 'bypass');
  renderChips(els.customTunnelChips, appConfig.custom_tunnel || [], 'tunnel');
}
function renderPresets(container, list) {
  container.replaceChildren();
  (list || []).forEach(p => {
    const row = document.createElement('label');
    row.className = 'switch-row';
    const info = document.createElement('span');
    info.className = 'switch-copy';
    const title = document.createElement('span');
    title.className = 'switch-title';
    title.textContent = `${p.name} (${(p.domains || []).length})`;
    const desc = document.createElement('span');
    desc.className = 'switch-desc';
    desc.textContent = p.description || '';
    info.append(title, desc);
    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.className = 'switch-input';
    cb.setAttribute('role', 'switch');
    cb.checked = !!p.enabled;
    cb.addEventListener('change', async () => {
      const nextVal = cb.checked;
      cb.disabled = true;
      await commitConfig((cfg) => {
        const target = (cfg.bypass_presets || []).concat(cfg.tunnel_presets || []).find(item => item.id === p.id);
        if (target) target.enabled = nextVal;
      }, `Liste güncellendi: ${p.name}`, true);
      cb.disabled = false;
    });
    row.append(info, cb);
    container.appendChild(row);
  });
}
function renderChips(container, domains, kind) {
  container.replaceChildren();
  domains.forEach(d => {
    const chip = document.createElement('span');
    chip.className = 'chip mono';
    const textSpan = document.createElement('span');
    textSpan.className = 'chip-text';
    textSpan.textContent = d;
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.title = `${d} sil`;
    btn.setAttribute('aria-label', `${d} sitesini listeden sil`);
    btn.appendChild(createSVGIcon('ico-x', 'ico ico-xs'));
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      showInlineError(kind === 'bypass' ? els.bypassErr : els.tunnelErr, kind === 'bypass' ? els.customBypassInput : els.customTunnelInput, '');
      await commitConfig((cfg) => {
        if (kind === 'bypass') cfg.custom_bypass = (cfg.custom_bypass || []).filter(x => x !== d);
        else cfg.custom_tunnel = (cfg.custom_tunnel || []).filter(x => x !== d);
      }, `Site silindi: ${d}`, true);
    });
    chip.append(textSpan, btn);
    container.appendChild(chip);
  });
}
function syncWizardChoicesUI() {
  els.wizardInstallChoices.querySelectorAll('.wizard-choice').forEach(b => {
    const active = b.dataset.install === wizardInstallChoice;
    b.classList.toggle('active', active);
    b.setAttribute('aria-pressed', String(active));
  });
  els.wizardModeChoices.querySelectorAll('.wizard-choice').forEach(b => {
    const active = b.dataset.mode === wizardModeChoice;
    b.classList.toggle('active', active);
    b.setAttribute('aria-pressed', String(active));
  });
}
function openWizard(triggerEl) {
  wizardInstallChoice = (installStatus && installStatus.resident_enabled)
    ? 'resident'
    : (appConfig && appConfig.setup_completed ? (appConfig.install_style || 'on_demand') : 'resident');
  if (appConfig && appConfig.setup_completed && (appConfig.mode === 'full' || appConfig.mode === 'smart')) {
    wizardModeChoice = appConfig.mode;
  } else {
    wizardModeChoice = 'full';
  }
  showInlineError(els.wizardError, null, '');
  syncWizardChoicesUI();
  openModal(els.wizardOverlay, triggerEl);
}
async function refreshStatus(force) {
  if (appShuttingDown || relaunchingAdmin || (busy && !force)) return;
  if (statusInFlight && !force) return;
  statusInFlight = true;
  const seq = ++statusSeq;
  try {
    const fresh = await api('/api/status', {}, 6000);
    if (seq !== statusSeq || appShuttingDown || relaunchingAdmin) return;
    engineStatus = fresh;
    if (consecutiveStatusFails > 0) {
      consecutiveStatusFails = 0;
      setOfflineState(false);
      addLog('Arka plan servisiyle bağlantı yeniden kuruldu.');
      showToast('Arka plan servisiyle bağlantı yeniden kuruldu.', 'ok', 'online');
    }
    renderStatus();
  } catch (err) {
    if (seq !== statusSeq || appShuttingDown || relaunchingAdmin) return;
    consecutiveStatusFails++;
    if (consecutiveStatusFails === 1) addLog('Durum alınamadı: ' + err.message);
    if (consecutiveStatusFails >= 2) setOfflineState(true);
  } finally {
    statusInFlight = false;
  }
}
function startPolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(() => {
    if (!document.hidden) refreshStatus(false);
  }, 5000);
}
document.addEventListener('visibilitychange', () => {
  if (!document.hidden) refreshStatus(true);
});
async function refreshNetInfo() {
  try {
    lastNetInfo = await api('/api/netinfo', {}, 10000);
    renderNetInfo();
  } catch {
    if (!maskSensitiveIP) {
      els.modemHint.textContent = 'Alınamadı';
      els.modemHint.title = 'Ağ bilgisi alınamadı';
    }
  }
}
async function refreshExitTrace() {
  if (appConfig && appConfig.disable_startup_net_probe) return;
  try {
    const t = await api('/api/trace', {}, 9000);
    if (t && t.ip) {
      lastProbeReport = { timestamp: nowStamp(), via_proxy: !!(engineStatus && engineStatus.running), trace: t };
      renderProbeSummary();
    }
  } catch {}
}
async function togglePower() {
  if (busy) return;
  busy = true;
  els.powerBtn.disabled = true;
  let connectedNow = false;
  try {
    const isRunning = engineStatus && engineStatus.running;
    if (isRunning) {
      addLog('OzeVPN motoru durduruluyor...');
      await api('/api/disconnect', { method: 'POST' }, 20000);
      if (els.coloBadge) els.coloBadge.textContent = '—';
      addLog('Bağlantı kesildi, sistem normal hatta döndü.');
      showToast('Bağlantı kesildi — normal internete dönüldü.', 'info', 'power');
    } else {
      els.powerBtn.className = 'power-btn connecting';
      if (els.routeSchematic) els.routeSchematic.dataset.state = 'connecting';
      els.statusText.textContent = 'Bağlanıyor...';
      els.statusSub.textContent = 'sing-box + WARP WireGuard (TCP+UDP) başlatılıyor (~5 sn)...';
      addLog('OzeVPN v6 (sing-box Hibrit Motor) başlatılıyor...');
      const res = await api('/api/connect', { method: 'POST' }, 60000);
      (res.logs || []).forEach(addLog);
      connectedNow = true;
      showToast('OzeVPN bağlandı — şifreli tünel aktif.', 'ok', 'power');
    }
  } catch (err) {
    addLog('Hata: ' + err.message);
    showToast('Bağlantı hatası: ' + err.message, 'error', 'power-err');
  } finally {
    busy = false;
    els.powerBtn.disabled = false;
    await refreshStatus(true);
    if (connectedNow) refreshExitTrace();
  }
}
async function runProbe() {
  els.probeBtn.disabled = true;
  const origLabel = els.probeBtn.querySelector('span');
  if (origLabel) origLabel.textContent = 'Ölçülüyor...';
  els.probeGrid.replaceChildren();
  const loading = document.createElement('div');
  loading.className = 'probe-empty';
  loading.textContent = 'Discord, Roblox, Pastebin, Google ve e-Devlet ölçülüyor...';
  els.probeGrid.appendChild(loading);
  try {
    const rep = await api('/api/probe', {}, 25000);
    lastProbeReport = rep;
    renderProbeSummary();
    els.probeGrid.replaceChildren();
    let okCount = 0;
    (rep.targets || []).forEach(item => {
      if (item.reachable) okCount++;
      const card = document.createElement('div');
      card.className = 'probe-item ' + (item.reachable ? 'ok' : 'fail');
      const top = document.createElement('div');
      top.className = 'probe-top';
      const nameSpan = document.createElement('span');
      nameSpan.textContent = item.name || '';
      const msSpan = document.createElement('span');
      msSpan.className = 'probe-ms mono';
      msSpan.textContent = item.reachable ? `${item.latency_ms} ms` : 'Engelli';
      top.append(nameSpan, msSpan);
      const bar = document.createElement('div');
      bar.className = 'probe-bar';
      const fill = document.createElement('div');
      const pct = item.reachable ? Math.max(8, Math.min(100, Math.round((item.latency_ms / 800) * 100))) : 100;
      fill.className = 'probe-bar-fill' + (!item.reachable ? ' fail' : item.latency_ms > 350 ? ' warn' : '');
      fill.style.width = `${pct}%`;
      bar.appendChild(fill);
      const errDiv = document.createElement('div');
      errDiv.className = 'probe-err';
      errDiv.textContent = item.reachable ? `HTTP ${item.status_code}` : (item.error || 'Ulaşılamadı');
      card.append(top, bar, errDiv);
      els.probeGrid.appendChild(card);
    });
    showToast(`Erişim testi tamamlandı: ${okCount}/${(rep.targets || []).length} hedef erişilebilir.`, okCount > 0 ? 'ok' : 'error', 'probe');
  } catch (err) {
    els.probeGrid.replaceChildren();
    const errBox = document.createElement('div');
    errBox.className = 'probe-empty';
    errBox.textContent = 'Test hatası: ' + err.message;
    els.probeGrid.appendChild(errBox);
    showToast('Erişim testi başarısız: ' + err.message, 'error', 'probe');
  } finally {
    els.probeBtn.disabled = false;
    if (origLabel) origLabel.textContent = 'Test Et';
  }
}
els.wizardInstallChoices.addEventListener('click', (e) => {
  const btn = e.target.closest('.wizard-choice');
  if (!btn) return;
  wizardInstallChoice = btn.dataset.install;
  syncWizardChoicesUI();
});
els.wizardModeChoices.addEventListener('click', (e) => {
  const btn = e.target.closest('.wizard-choice');
  if (!btn) return;
  wizardModeChoice = btn.dataset.mode;
  syncWizardChoicesUI();
});
els.wizardSkipBtn.addEventListener('click', async () => {
  closeModal(els.wizardOverlay);
  if (appConfig && !appConfig.setup_completed) {
    await commitConfig((cfg) => { cfg.setup_completed = true; });
  }
});
els.wizardFinishBtn.addEventListener('click', async () => {
  els.wizardFinishBtn.disabled = true;
  els.wizardSkipBtn.disabled = true;
  els.wizardFinishBtn.textContent = 'Kuruluyor ve Bağlanılıyor...';
  showInlineError(els.wizardError, null, '');
  try {
    const res = await api('/api/install/setup', {
      method: 'POST',
      body: JSON.stringify({
        install_style: wizardInstallChoice,
        mode: wizardModeChoice,
        engine_profile: 'hybrid_eu',
        connect_now: true,
      }),
    }, 60000);
    appConfig = res.config;
    installStatus = res.install;
    renderConfig();
    renderInstall();
    (res.logs || []).forEach(addLog);
    closeModal(els.wizardOverlay);
    await refreshStatus(true);
    showToast('Kurulum tamamlandı ve OzeVPN başlatıldı.', 'ok', 'wizard');
  } catch (err) {
    showInlineError(els.wizardError, null, 'Kurulum hatası: ' + err.message);
    addLog('Kurulum hatası: ' + err.message);
  } finally {
    els.wizardFinishBtn.disabled = false;
    els.wizardSkipBtn.disabled = false;
    els.wizardFinishBtn.textContent = 'Kurulumu Tamamla ve Başlat';
  }
});
els.openWizardBtn.addEventListener('click', () => openWizard(els.toolsMenuBtn));
if (els.retryConnBtn) {
  els.retryConnBtn.addEventListener('click', async () => {
    els.retryConnBtn.disabled = true;
    els.retryConnBtn.textContent = 'Deneniyor...';
    try {
      await initApp();
    } finally {
      els.retryConnBtn.disabled = false;
      els.retryConnBtn.textContent = 'Yeniden Dene';
    }
  });
}
if (els.quitAppBtn) {
  els.quitAppBtn.addEventListener('click', async () => {
    setToolsMenuOpen(false, false);
    const running = !!(engineStatus && engineStatus.running);
    const desc = running
      ? 'Aktif OzeVPN tüneli kesilecek, Windows proxy normale döndürülecek ve arka plan uygulaması tamamen kapanacak.'
      : 'OzeVPN arka plan uygulaması kapatılacak. Tekrar açmak için masaüstü kısayolunu kullanabilirsin.';
    const ok = await askConfirm('OzeVPN Kapatılsın mı?', desc, 'Evet, Kapat', els.toolsMenuBtn);
    if (!ok) return;
    els.quitAppBtn.disabled = true;
    appShuttingDown = true;
    if (pollTimer) clearInterval(pollTimer);
    try {
      addLog('OzeVPN kapatılıyor...');
      await api('/api/shutdown', { method: 'POST' }, 8000);
      els.statusText.textContent = 'OzeVPN Kapatıldı';
      els.statusSub.textContent = 'Arka plan servisi güvenli şekilde durduruldu. Bu pencereyi kapatabilirsin.';
      els.powerBtn.disabled = true;
      window.close();
    } catch (err) {
      appShuttingDown = false;
      startPolling();
      addLog('Kapatma hatası: ' + err.message);
      showToast('Kapatma hatası: ' + err.message, 'error');
      els.quitAppBtn.disabled = false;
    }
  });
}
els.toggleResidentBtn.addEventListener('click', async () => {
  const targetStyle = (installStatus && installStatus.resident_enabled) ? 'on_demand' : 'resident';
  els.toggleResidentBtn.disabled = true;
  const prevTxt = els.toggleResidentBtn.textContent;
  els.toggleResidentBtn.textContent = 'Kaydediliyor...';
  try {
    const res = await api('/api/install/setup', {
      method: 'POST',
      body: JSON.stringify({
        install_style: targetStyle,
        mode: appConfig ? appConfig.mode : 'full',
        connect_now: false,
      }),
    }, 20000);
    appConfig = res.config;
    installStatus = res.install;
    renderInstall();
    const msg = targetStyle === 'resident' ? 'Yerleşik Mod etkinleştirildi.' : 'Aç/Kapa Moduna geçildi.';
    addLog(msg);
    showToast(msg, 'ok', 'resident');
  } catch (err) {
    els.toggleResidentBtn.textContent = prevTxt;
    addLog('Kurulum değişikliği hatası: ' + err.message);
    showToast('Kurulum değişikliği hatası: ' + err.message, 'error');
  } finally {
    els.toggleResidentBtn.disabled = false;
  }
});
els.powerBtn.addEventListener('click', togglePower);
els.probeBtn.addEventListener('click', runProbe);
els.modeGrid.addEventListener('click', async (e) => {
  const row = e.target.closest('.mode-row');
  if (!row || !appConfig) return;
  const nextMode = row.dataset.mode;
  const names = { full: 'Tüm Dünya Engelsiz', smart: 'Hibrit + TR Hız Koruması', pac: 'Sadece Seçili Siteler (PAC)' };
  await commitConfig((cfg) => { cfg.mode = nextMode; }, `Mod seçildi: ${names[nextMode] || nextMode}`, true);
});
els.engineProfileSelect.addEventListener('change', async () => {
  const nextProfile = els.engineProfileSelect.value;
  await commitConfig((cfg) => { cfg.engine_profile = nextProfile; }, `Motor profili seçildi: ${nextProfile}`);
});
els.protocolSelect.addEventListener('change', async () => {
  const nextProto = els.protocolSelect.value;
  await commitConfig((cfg) => { cfg.protocol = nextProto; }, `WARP protokolü seçildi: ${nextProto}`);
});
async function saveCustomServer() {
  const val = els.customServerInput.value.trim();
  if (!val) {
    showInlineError(els.customServerErr, els.customServerInput, 'Kaydetmek için geçerli bir sunucu bağlantısı veya WireGuard yapılandırması yapıştır.');
    return;
  }
  showInlineError(els.customServerErr, els.customServerInput, '');
  els.saveServerBtn.disabled = true;
  els.saveServerBtn.textContent = 'Kaydediliyor...';
  try {
    const ok = await commitConfig((cfg) => { cfg.custom_server = val; }, 'Özel sunucu bağlantısı şifreli kaydedildi.');
    if (ok) els.customServerInput.value = '';
    else showInlineError(els.customServerErr, els.customServerInput, 'Özel sunucu kaydedilemedi; bağlantıyı kontrol et.');
  } finally {
    els.saveServerBtn.disabled = false;
    els.saveServerBtn.textContent = 'Kaydet';
  }
}
els.saveServerBtn.addEventListener('click', saveCustomServer);
els.customServerInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') saveCustomServer(); });
if (els.adBlockToggle) {
  els.adBlockToggle.addEventListener('change', async () => {
    const nextVal = els.adBlockToggle.checked;
    els.adBlockToggle.disabled = true;
    await commitConfig((cfg) => { cfg.ad_block_speed = nextVal; }, `Reklam Kalkanı: ${nextVal ? 'Açık' : 'Kapalı'}`, true);
    els.adBlockToggle.disabled = false;
  });
}
if (els.torBridgeToggle) {
  els.torBridgeToggle.addEventListener('change', async () => {
    const nextVal = els.torBridgeToggle.checked;
    els.torBridgeToggle.disabled = true;
    await commitConfig((cfg) => { cfg.tor_bridge = nextVal; }, `Tor Snowflake Köprü: ${nextVal ? 'Açık (yeniden bağlanınca aktif)' : 'Kapalı'}`);
    els.torBridgeToggle.disabled = false;
  });
}
if (els.killSwitchToggle) {
  els.killSwitchToggle.addEventListener('change', async () => {
    const nextVal = els.killSwitchToggle.checked;
    els.killSwitchToggle.disabled = true;
    await commitConfig((cfg) => { cfg.kill_switch = nextVal; }, `Kill Switch: ${nextVal ? 'Açık (Oyun TUN bağlantısında aktif)' : 'Kapalı'}`);
    els.killSwitchToggle.disabled = false;
  });
}
if (els.blockLanToggle) {
  els.blockLanToggle.addEventListener('change', async () => {
    const nextVal = els.blockLanToggle.checked;
    els.blockLanToggle.disabled = true;
    await commitConfig((cfg) => { cfg.block_lan = nextVal; }, `Yerel Ağ İzolasyonu: ${nextVal ? 'Açık' : 'Kapalı'}`, true);
    els.blockLanToggle.disabled = false;
  });
}
if (els.disableNetProbeToggle) {
  els.disableNetProbeToggle.addEventListener('change', async () => {
    const nextVal = els.disableNetProbeToggle.checked;
    els.disableNetProbeToggle.disabled = true;
    await commitConfig((cfg) => { cfg.disable_startup_net_probe = nextVal; }, `Harici başlangıç sorgusu: ${nextVal ? 'Kapalı' : 'Açık'}`);
    els.disableNetProbeToggle.disabled = false;
  });
}
if (els.clearServerBtn) {
  els.clearServerBtn.addEventListener('click', async () => {
    els.clearServerBtn.disabled = true;
    try {
      appConfig = await api('/api/config', {
        method: 'POST',
        body: JSON.stringify({ ...appConfig, custom_server: '', clear_custom_server: true }),
      });
      renderConfig();
      addLog('Kayıtlı özel sunucu anahtarı güvenli biçimde silindi.');
      showToast('Kayıtlı özel sunucu anahtarı silindi.', 'info');
    } catch (err) {
      addLog('Özel sunucu silinemedi: ' + err.message);
      showToast('Özel sunucu silinemedi: ' + err.message, 'error');
    } finally {
      els.clearServerBtn.disabled = false;
    }
  });
}
if (els.newIpBtn) {
  els.newIpBtn.addEventListener('click', async () => {
    setToolsMenuOpen(false, false);
    if (!engineStatus || !engineStatus.running) {
      const msg = 'Yeni Avrupa IP almak için önce güç düğmesine basıp bağlan.';
      addLog(msg);
      showToast(msg, 'info', 'new-ip');
      return;
    }
    els.newIpBtn.disabled = true;
    showToast('Yeni Avrupa Tor IP alınıyor...', 'info', 'new-ip');
    try {
      const r = await api('/api/new-identity', { method: 'POST' }, 35000);
      const msg = r.message || 'Yeni Avrupa IP alındı.';
      addLog(msg);
      showToast(msg, 'ok', 'new-ip');
    } catch (err) {
      addLog('IP yenileme hatası: ' + err.message);
      showToast('IP yenileme hatası: ' + err.message, 'error', 'new-ip');
    } finally {
      els.newIpBtn.disabled = false;
    }
  });
}
if (els.speedBtn) {
  els.speedBtn.addEventListener('click', async () => {
    els.speedBtn.disabled = true;
    const sp = els.speedBtn.querySelector('span');
    if (sp) sp.textContent = 'Ölçülüyor...';
    try {
      const s = await api('/api/speedtest', {}, 30000);
      if (s.error) {
        els.traceSummary.textContent = 'Hız testi hatası: ' + s.error;
        showToast('Hız testi hatası: ' + s.error, 'error', 'speed');
      } else {
        const mode = s.via_proxy ? 'OzeVPN Tüneli' : 'Direkt Hat';
        const ts = nowStamp();
        if (els.speedStrip) {
          els.speedStrip.classList.remove('hidden');
          els.speedMbps.textContent = `${s.download_mbps} Mbps`;
          els.speedPing.textContent = `${s.ping_ms} ms`;
          els.speedJitter.textContent = `${s.jitter_ms} ms`;
          els.speedPath.textContent = mode;
        }
        els.traceSummary.textContent = `Hız (${mode}): ${s.download_mbps} Mbps · Ping: ${s.ping_ms} ms · Jitter: ${s.jitter_ms} ms (${ts})`;
        addLog(`Hız testi (${mode}): ${s.download_mbps} Mbps | Ping ${s.ping_ms} ms | Jitter ${s.jitter_ms} ms`);
        showToast(`Hız: ${s.download_mbps} Mbps · Ping: ${s.ping_ms} ms`, 'ok', 'speed');
      }
    } catch (err) {
      els.traceSummary.textContent = 'Hız testi hatası: ' + err.message;
      addLog('Hız testi hatası: ' + err.message);
      showToast('Hız testi hatası: ' + err.message, 'error', 'speed');
    } finally {
      els.speedBtn.disabled = false;
      if (sp) sp.textContent = 'Hız Ölç';
    }
  });
}
async function runRescueTest() {
  const dom = els.rescueInput.value.trim();
  if (!dom) {
    els.rescueInput.setAttribute('aria-invalid', 'true');
    els.rescueResult.textContent = 'Kontrol etmek istediğin bir site adresi yaz (örn: wattpad.com).';
    els.rescueInput.focus();
    return;
  }
  els.rescueInput.removeAttribute('aria-invalid');
  els.rescueTestBtn.disabled = true;
  els.rescueAddBtn.disabled = true;
  els.rescueTestBtn.textContent = 'Test ediliyor...';
  els.rescueResult.textContent = 'Direkt, Tünel ve Avrupa Tor yolları test ediliyor...';
  try {
    const r = await api('/api/rescue', { method: 'POST', body: JSON.stringify({ domain: dom, add_to_eu: false }) }, 25000);
    const parts = (r.paths || []).map(p => `${p.path}: ${p.reachable ? 'OK ' + p.latency_ms + ' ms' : 'Engelli (' + (p.error || 'Hata') + ')'}`);
    els.rescueResult.textContent = `${r.domain} → ${parts.join(' | ')} (Öneri: ${r.recommended})`;
  } catch (err) {
    els.rescueResult.textContent = 'Test hatası: ' + err.message;
  } finally {
    els.rescueTestBtn.disabled = false;
    els.rescueAddBtn.disabled = false;
    els.rescueTestBtn.textContent = 'Kontrol Et';
  }
}
async function runRescueAdd() {
  const dom = els.rescueInput.value.trim();
  if (!dom) {
    els.rescueInput.setAttribute('aria-invalid', 'true');
    els.rescueResult.textContent = 'Avrupa çıkışına almak istediğin site adresini yaz (örn: wattpad.com).';
    els.rescueInput.focus();
    return;
  }
  els.rescueInput.removeAttribute('aria-invalid');
  els.rescueTestBtn.disabled = true;
  els.rescueAddBtn.disabled = true;
  els.rescueAddBtn.textContent = 'Ekleniyor...';
  try {
    const r = await api('/api/rescue', { method: 'POST', body: JSON.stringify({ domain: dom, add_to_eu: true }) }, 25000);
    appConfig = r.config;
    renderConfig();
    els.rescueInput.value = '';
    els.rescueResult.textContent = `${r.domain} Avrupa çıkışına eklendi — ${r.message}`;
    addLog(`${r.domain} Avrupa çıkışına alındı (${r.message})`);
    showToast(`${r.domain} Avrupa çıkışına eklendi.`, 'ok', 'rescue');
  } catch (err) {
    els.rescueResult.textContent = 'Hata: ' + err.message;
    showToast('Site eklenemedi: ' + err.message, 'error');
  } finally {
    els.rescueTestBtn.disabled = false;
    els.rescueAddBtn.disabled = false;
    els.rescueAddBtn.textContent = "Avrupa'ya Al";
  }
}
if (els.rescueTestBtn) els.rescueTestBtn.addEventListener('click', runRescueTest);
if (els.rescueAddBtn) els.rescueAddBtn.addEventListener('click', runRescueAdd);
if (els.rescueInput) {
  els.rescueInput.addEventListener('keydown', (e) => { if (e.key === 'Enter') runRescueTest(); });
}
if (els.emergencyResetBtn) {
  els.emergencyResetBtn.addEventListener('click', async () => {
    setToolsMenuOpen(false, false);
    const ok = await askConfirm(
      'Windows Proxy Sıfırlansın mı?',
      'Aktif OzeVPN tünelleri durdurulacak ve Windows sistem proxy ayarları tamamen temizlenerek normal internete dönülecek.',
      'Evet, Sıfırla',
      els.toolsMenuBtn
    );
    if (!ok) return;
    els.emergencyResetBtn.disabled = true;
    try {
      const r = await api('/api/emergency-reset', { method: 'POST' }, 15000);
      const msg = r.message || 'Proxy sıfırlandı.';
      addLog(msg);
      showToast(msg, 'ok', 'reset');
      await refreshStatus(true);
    } catch (err) {
      addLog('Sıfırlama hatası: ' + err.message);
      showToast('Sıfırlama hatası: ' + err.message, 'error');
    } finally {
      els.emergencyResetBtn.disabled = false;
    }
  });
}
if (els.relaunchAdminBtn) {
  els.relaunchAdminBtn.addEventListener('click', async () => {
    if (els.relaunchAdminBtn.disabled || relaunchingAdmin) return;
    relaunchingAdmin = true;
    els.relaunchAdminBtn.disabled = true;
    const lbl = els.relaunchAdminBtn.querySelector('.btn-label');
    if (lbl) lbl.textContent = 'UAC Bekleniyor...';
    setOfflineState(false);
    addLog('Yönetici (Oyun TUN) izni isteniyor — Windows UAC penceresinden "Evet" onayla...');
    try {
      await api('/api/relaunch-admin', { method: 'POST' }, 60000);
      if (lbl) lbl.textContent = 'TUN Geçiliyor...';
      els.statusSub.textContent = 'Yönetici (Oyun TUN) servisi devralıyor, lütfen bekle...';
      addLog('Yönetici onayı alındı — yeni TUN servisinin portu devralması bekleniyor...');
      let switched = false;
      for (let i = 0; i < 30; i++) {
        await new Promise(r => setTimeout(r, 500));
        try {
          const st = await api('/api/status', {}, 2500);
          if (st && st.is_admin) {
            engineStatus = st;
            switched = true;
            break;
          }
        } catch {}
      }
      relaunchingAdmin = false;
      consecutiveStatusFails = 0;
      setOfflineState(false);
      if (switched) {
        renderStatus();
        const msg = 'Yönetici (Oyun TUN) modu aktif — artık Bağlan ile gerçek TUN ağ kartını açabilirsin.';
        addLog(msg);
        showToast('Oyun TUN (Yönetici) modu hazır!', 'ok', 'admin-tun');
      } else {
        await refreshStatus(true);
        addLog('Yönetici geçişi tamamlandı ancak durum doğrulanamadı — sayfayı yenileyebilirsin.');
      }
    } catch (err) {
      relaunchingAdmin = false;
      addLog('Yönetici başlatma iptal edildi: ' + err.message);
      showToast('Yönetici geçişi iptal edildi.', 'info');
      renderStatus();
    }
  });
}
const tabButtons = Array.from(document.querySelectorAll('.tab[role="tab"]'));
function activateTab(btn, moveFocus, persist = true) {
  tabButtons.forEach(b => {
    const isSel = b === btn;
    b.classList.toggle('active', isSel);
    b.setAttribute('aria-selected', String(isSel));
    b.setAttribute('tabindex', isSel ? '0' : '-1');
    const panel = $('tab-' + b.dataset.tab);
    if (panel) {
      panel.classList.toggle('active', isSel);
      panel.hidden = !isSel;
    }
  });
  if (persist && btn && btn.dataset.tab) {
    try { sessionStorage.setItem('ozevpn_active_tab', btn.dataset.tab); } catch {}
  }
  if (moveFocus) btn.focus();
}
tabButtons.forEach((btn, idx) => {
  btn.addEventListener('click', () => activateTab(btn, false));
  btn.addEventListener('keydown', (e) => {
    let nextIdx = null;
    if (e.key === 'ArrowRight') nextIdx = (idx + 1) % tabButtons.length;
    else if (e.key === 'ArrowLeft') nextIdx = (idx - 1 + tabButtons.length) % tabButtons.length;
    else if (e.key === 'Home') nextIdx = 0;
    else if (e.key === 'End') nextIdx = tabButtons.length - 1;
    if (nextIdx !== null) {
      e.preventDefault();
      activateTab(tabButtons[nextIdx], true);
    }
  });
});
try {
  const savedTab = sessionStorage.getItem('ozevpn_active_tab');
  const found = savedTab && tabButtons.find(b => b.dataset.tab === savedTab);
  if (found) activateTab(found, false, false);
} catch {}
function cleanClientDomain(raw) {
  let d = String(raw || '').trim().toLowerCase();
  d = d.replace(/^https?:\/\//, '').split('/')[0].split(':')[0].replace(/^\*\./, '').replace(/^www\./, '').trim();
  if (!d || d.length > 253) return '';
  if (!/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$/.test(d)) return '';
  if (/^\d+\.\d+\.\d+\.\d+$/.test(d)) return '';
  return d;
}
async function addCustomDomain(kind) {
  const isBypass = kind === 'bypass';
  const input = isBypass ? els.customBypassInput : els.customTunnelInput;
  const errEl = isBypass ? els.bypassErr : els.tunnelErr;
  const btn = isBypass ? els.addBypassBtn : els.addTunnelBtn;
  const rawVal = input.value.trim();
  if (!rawVal) {
    showInlineError(errEl, input, 'Eklemek için bir site adresi gir (örn: ornek.com).');
    input.focus();
    return;
  }
  const val = cleanClientDomain(rawVal);
  if (!val) {
    showInlineError(errEl, input, 'Geçerli bir alan adı gir (örn: ornek.com — boşluk veya IP adresi kabul edilmez).');
    input.focus();
    return;
  }
  const existing = isBypass ? (appConfig && appConfig.custom_bypass || []) : (appConfig && appConfig.custom_tunnel || []);
  if (existing.includes(val)) {
    showInlineError(errEl, input, `${val} zaten bu listede kayıtlı.`);
    input.focus();
    return;
  }
  showInlineError(errEl, input, '');
  btn.disabled = true;
  btn.textContent = 'Ekleniyor...';
  try {
    const ok = await commitConfig((cfg) => {
      if (isBypass) cfg.custom_bypass = [...(cfg.custom_bypass || []), val];
      else cfg.custom_tunnel = [...(cfg.custom_tunnel || []), val];
    }, `Site eklendi: ${val}`, true);
    if (ok) input.value = '';
    else showInlineError(errEl, input, 'Site kaydedilemedi; geçerli bir alan adı girdiğinden emin ol.');
  } finally {
    btn.disabled = false;
    btn.textContent = 'Ekle';
  }
}
els.addBypassBtn.addEventListener('click', () => addCustomDomain('bypass'));
els.addTunnelBtn.addEventListener('click', () => addCustomDomain('tunnel'));
els.customBypassInput.addEventListener('keydown', e => { if (e.key === 'Enter') addCustomDomain('bypass'); });
els.customTunnelInput.addEventListener('keydown', e => { if (e.key === 'Enter') addCustomDomain('tunnel'); });
els.customBypassInput.addEventListener('input', () => showInlineError(els.bypassErr, els.customBypassInput, ''));
els.customTunnelInput.addEventListener('input', () => showInlineError(els.tunnelErr, els.customTunnelInput, ''));
els.customServerInput.addEventListener('input', () => showInlineError(els.customServerErr, els.customServerInput, ''));
if (els.rescueInput) {
  els.rescueInput.addEventListener('input', () => els.rescueInput.removeAttribute('aria-invalid'));
}
async function initApp() {
  try {
    [appConfig, installStatus] = await Promise.all([
      api('/api/config', {}, 10000),
      api('/api/install/status', {}, 10000),
    ]);
    setOfflineState(false);
    consecutiveStatusFails = 0;
    renderConfig();
    renderInstall();
    if (!appConfig.setup_completed && !activeModal) {
      openWizard(null);
    }
    refreshNetInfo();
    await refreshStatus(true);
  } catch (err) {
    setOfflineState(true);
    addLog('Başlatma hatası: ' + err.message);
  }
}
initApp();
startPolling();
