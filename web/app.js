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
  appShell: document.getElementById('appShell'),
  offlineBanner: document.getElementById('offlineBanner'),
  offlineBannerText: document.getElementById('offlineBannerText'),
  retryConnBtn: document.getElementById('retryConnBtn'),
  themeToggleBtn: document.getElementById('themeToggleBtn'),
  maskIpBtn: document.getElementById('maskIpBtn'),
  advancedSettings: document.getElementById('advancedSettings'),

  wizardOverlay: document.getElementById('wizardOverlay'),
  wizardInstallChoices: document.getElementById('wizardInstallChoices'),
  wizardModeChoices: document.getElementById('wizardModeChoices'),
  wizardError: document.getElementById('wizardError'),
  wizardSkipBtn: document.getElementById('wizardSkipBtn'),
  wizardFinishBtn: document.getElementById('wizardFinishBtn'),

  confirmOverlay: document.getElementById('confirmOverlay'),
  confirmTitle: document.getElementById('confirmTitle'),
  confirmDesc: document.getElementById('confirmDesc'),
  confirmCancelBtn: document.getElementById('confirmCancelBtn'),
  confirmOkBtn: document.getElementById('confirmOkBtn'),

  openWizardBtn: document.getElementById('openWizardBtn'),
  quitAppBtn: document.getElementById('quitAppBtn'),
  newIpBtn: document.getElementById('newIpBtn'),
  relaunchAdminBtn: document.getElementById('relaunchAdminBtn'),
  emergencyResetBtn: document.getElementById('emergencyResetBtn'),
  adBlockToggle: document.getElementById('adBlockToggle'),
  killSwitchToggle: document.getElementById('killSwitchToggle'),
  blockLanToggle: document.getElementById('blockLanToggle'),
  disableNetProbeToggle: document.getElementById('disableNetProbeToggle'),
  torBridgeToggle: document.getElementById('torBridgeToggle'),
  clearServerBtn: document.getElementById('clearServerBtn'),
  speedBtn: document.getElementById('speedBtn'),
  rescueInput: document.getElementById('rescueInput'),
  rescueTestBtn: document.getElementById('rescueTestBtn'),
  rescueAddBtn: document.getElementById('rescueAddBtn'),
  rescueResult: document.getElementById('rescueResult'),

  brandDot: document.getElementById('brandDot'),
  installBadge: document.getElementById('installBadge'),
  protoBadge: document.getElementById('protoBadge'),
  coloBadge: document.getElementById('coloBadge'),
  powerBtn: document.getElementById('powerBtn'),
  statusText: document.getElementById('statusText'),
  statusSub: document.getElementById('statusSub'),
  modeGrid: document.getElementById('modeGrid'),
  engineProfileSelect: document.getElementById('engineProfileSelect'),
  protocolSelect: document.getElementById('protocolSelect'),
  customServerBox: document.getElementById('customServerBox'),
  customServerInput: document.getElementById('customServerInput'),
  customServerErr: document.getElementById('customServerErr'),
  saveServerBtn: document.getElementById('saveServerBtn'),
  residentTitle: document.getElementById('residentTitle'),
  residentSub: document.getElementById('residentSub'),
  toggleResidentBtn: document.getElementById('toggleResidentBtn'),
  logBox: document.getElementById('logBox'),

  modemHint: document.getElementById('modemHint'),
  dnsInfo: document.getElementById('dnsInfo'),
  ipv6Info: document.getElementById('ipv6Info'),

  probeBtn: document.getElementById('probeBtn'),
  probeGrid: document.getElementById('probeGrid'),
  traceSummary: document.getElementById('traceSummary'),
  bypassPresets: document.getElementById('bypassPresets'),
  tunnelPresets: document.getElementById('tunnelPresets'),
  customBypassInput: document.getElementById('customBypassInput'),
  bypassErr: document.getElementById('bypassErr'),
  addBypassBtn: document.getElementById('addBypassBtn'),
  customBypassChips: document.getElementById('customBypassChips'),
  customTunnelInput: document.getElementById('customTunnelInput'),
  tunnelErr: document.getElementById('tunnelErr'),
  addTunnelBtn: document.getElementById('addTunnelBtn'),
  customTunnelChips: document.getElementById('customTunnelChips'),
};

function nowStamp() {
  try {
    return new Date().toLocaleTimeString('tr-TR', { hour12: false });
  } catch {
    return '';
  }
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

/* Global Error Boundary */
window.addEventListener('error', (e) => {
  if (e && e.message) addLog('Beklenmeyen arayüz hatası: ' + e.message);
});
window.addEventListener('unhandledrejection', (e) => {
  const reason = e && e.reason ? (e.reason.message || String(e.reason)) : 'Bilinmeyen hata';
  addLog('Beklenmeyen işlem hatası: ' + reason);
});

function showInlineError(el, inputEl, msg) {
  if (el) {
    el.textContent = msg || '';
    el.classList.toggle('hidden', !msg);
  }
  if (inputEl) {
    if (msg) {
      inputEl.setAttribute('aria-invalid', 'true');
    } else {
      inputEl.removeAttribute('aria-invalid');
    }
  }
}

/* ---------- Theme Manager (Light / Dark + System + Persistence) ---------- */
function applyTheme(theme, persist) {
  const t = theme === 'light' ? 'light' : 'dark';
  document.documentElement.dataset.theme = t;
  if (persist) {
    try { localStorage.setItem('ozevpn_theme', t); } catch {}
  }
  if (els.themeToggleBtn) {
    const isLight = t === 'light';
    els.themeToggleBtn.textContent = isLight ? '🌙 Koyu Tema' : '☀️ Açık Tema';
    els.themeToggleBtn.setAttribute('aria-pressed', String(isLight));
  }
}

applyTheme(document.documentElement.dataset.theme || 'dark', false);

if (els.themeToggleBtn) {
  els.themeToggleBtn.addEventListener('click', () => {
    const next = document.documentElement.dataset.theme === 'light' ? 'dark' : 'light';
    applyTheme(next, true);
  });
}

if (window.matchMedia) {
  window.matchMedia('(prefers-color-scheme: light)').addEventListener('change', (e) => {
    let saved = null;
    try { saved = localStorage.getItem('ozevpn_theme'); } catch {}
    if (!saved) {
      applyTheme(e.matches ? 'light' : 'dark', false);
    }
  });
}

/* ---------- Privacy IP / MAC Masking Toggle ---------- */
function updateMaskButtonUI() {
  if (!els.maskIpBtn) return;
  els.maskIpBtn.textContent = maskSensitiveIP ? '🙈 IP Gizli' : '👁️ IP Gizle';
  els.maskIpBtn.setAttribute('aria-pressed', String(maskSensitiveIP));
}

function renderNetInfo() {
  if (!lastNetInfo) return;
  const ni = lastNetInfo;
  const ctry = ni.country && ni.country !== 'AUTO' ? ` • Ülke: ${ni.country}` : '';
  const hint = maskSensitiveIP
    ? `Yerel Ağ Gizli${ctry}`
    : ((ni.modem_hint || 'Tespit edilemedi') + ctry);
  const dns = maskSensitiveIP
    ? 'Gizli (Maskeli)'
    : ((ni.dns_servers && ni.dns_servers.length) ? ni.dns_servers.slice(0, 2).join(', ') : 'Otomatik');
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
    const viaTxt = rep.via_proxy ? 'OzeVPN Tüneli Aktif' : 'Direkt Hat';
    const shownIP = maskSensitiveIP ? '•••.•••.•••.•••' : t.ip;
    els.coloBadge.textContent = `Çıkış: ${t.colo || '—'} (${t.loc || '—'})`;
    els.traceSummary.textContent = `IP: ${shownIP} • Bölge: ${t.loc || '?'} • Sunucu: ${t.colo || '?'} • ${viaTxt} (${rep.timestamp})`;
  } else {
    els.traceSummary.textContent = 'Trace bilgisi alınamadı.';
  }
}

updateMaskButtonUI();
if (els.maskIpBtn) {
  els.maskIpBtn.addEventListener('click', () => {
    maskSensitiveIP = !maskSensitiveIP;
    try { localStorage.setItem('ozevpn_mask_ip', maskSensitiveIP ? '1' : '0'); } catch {}
    updateMaskButtonUI();
    renderNetInfo();
    renderProbeSummary();
  });
}

/* ---------- Accessible Modal Manager (Focus Trap, Inert Feature Check, Escape, Focus Restore) ---------- */
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
  lastFocusedBeforeModal = triggerEl || document.activeElement;
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
  if (!anyOpen) {
    setAppInert(false);
  }
  if (lastFocusedBeforeModal && typeof lastFocusedBeforeModal.focus === 'function') {
    lastFocusedBeforeModal.focus();
  }
  lastFocusedBeforeModal = null;
}

document.addEventListener('keydown', (e) => {
  if (!activeModal) return;
  if (e.key === 'Escape') {
    e.preventDefault();
    if (activeModal === els.confirmOverlay) {
      finishConfirm(false);
    } else if (activeModal === els.wizardOverlay) {
      closeModal(els.wizardOverlay);
    }
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
});

els.wizardOverlay.addEventListener('click', (e) => {
  if (e.target === els.wizardOverlay) {
    closeModal(els.wizardOverlay);
  }
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

/* ---------- Safe API Client (IPC Session Token + Timeout + Safe JSON/Text Parse + 429) ---------- */
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
    const res = await fetch(path, {
      ...opts,
      headers: hdrs,
      signal: controller.signal,
    });
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

/* ---------- Config Sync with Rollback ---------- */
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
        addLog(r.message || 'Kurallar anında yenilendi.');
      } catch (rErr) {
        addLog('Canlı kural yenileme uyarısı: ' + rErr.message);
      }
    }
    return true;
  } catch (err) {
    appConfig = snapshot;
    renderConfig();
    addLog('Ayar kaydedilemedi: ' + err.message);
    return false;
  }
}

/* ---------- Renderers ---------- */
function renderInstall() {
  if (!installStatus) return;
  if (installStatus.resident_enabled) {
    els.installBadge.textContent = 'Çalışma: ⚡ Yerleşik';
    els.residentTitle.textContent = 'Yerleşik Mod: Aktif';
    els.residentSub.textContent = 'Windows her açıldığında sessizce başlar ve bağlanır.';
    els.toggleResidentBtn.textContent = 'Aç/Kapa Yap';
  } else {
    els.installBadge.textContent = 'Çalışma: 🔘 Aç/Kapa';
    els.residentTitle.textContent = 'Yerleşik Mod: Kapalı';
    els.residentSub.textContent = 'Sadece sen açıp güç düğmesine bastığında çalışır.';
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
  els.brandDot.className = 'logo-dot ' + (isConn ? 'connected' : isConnIng ? 'connecting' : '');

  if (isConn) {
    const scopeTxt = engineStatus.is_admin ? 'Tam Sistem TUN (IPv4+IPv6+UDP)' : 'Tarayıcı/Proxy Korumalı';
    const ksTxt = engineStatus.kill_switch_active ? ' • 🔒 Kill Switch Aktif' : '';
    els.statusText.textContent = `Bağlı — ${scopeTxt}`;
    els.statusSub.textContent = `${engineStatus.capture_layer || 'sing-box Aktif'} • Motor: ${engineStatus.active_profile || 'hybrid_eu'}${ksTxt}`;
  } else if (isConnIng) {
    els.statusText.textContent = 'Bağlanıyor...';
    els.statusSub.textContent = 'Hibrit sing-box motoru başlatılıyor...';
  } else {
    els.statusText.textContent = 'Bağlı Değil';
    els.statusSub.textContent = engineStatus.last_error
      ? `Hata: ${engineStatus.last_error}`
      : 'Tek tıkla Discord, Roblox ve tüm engelli siteleri aç';
  }

  const layer = engineStatus.is_admin ? 'TUN (Yönetici)' : 'Proxy (Standart)';
  const up = engineStatus.running && engineStatus.uptime_sec > 0 ? ` • ${engineStatus.uptime_sec} sn` : '';
  els.protoBadge.textContent = `Katman: ${engineStatus.running ? engineStatus.capture_layer : layer}${up}`;
  if (els.relaunchAdminBtn) {
    if (engineStatus.is_admin) {
      els.relaunchAdminBtn.textContent = '🛡️ TUN Aktif';
      els.relaunchAdminBtn.disabled = true;
      els.relaunchAdminBtn.title = 'Yönetici (Oyun TUN) modu zaten aktif';
    } else {
      els.relaunchAdminBtn.textContent = '🎮 Oyun TUN';
      els.relaunchAdminBtn.disabled = false;
      els.relaunchAdminBtn.title = 'Roblox masaüstü ve oyun UDP trafiği için Yönetici (TUN) modunda aç';
    }
  }
}

function renderConfig() {
  if (!appConfig) return;
  document.querySelectorAll('.mode-card').forEach(btn => {
    const isAct = btn.dataset.mode === appConfig.mode;
    btn.classList.toggle('active', isAct);
    btn.setAttribute('aria-pressed', String(isAct));
  });
  els.engineProfileSelect.value = appConfig.engine_profile || 'hybrid_eu';
  els.protocolSelect.value = appConfig.protocol || 'Auto';
  els.customServerInput.placeholder = appConfig.custom_server_set
    ? '🔒 Kayıtlı özel sunucu aktif (değiştirmek için yenisini yapıştır)'
    : 'vless://, socks5:// veya WireGuard .conf yapıştır';
  els.customServerBox.classList.toggle('hidden', els.engineProfileSelect.value !== 'custom');
  if (els.adBlockToggle) {
    els.adBlockToggle.checked = !!appConfig.ad_block_speed;
  }
  if (els.killSwitchToggle) {
    els.killSwitchToggle.checked = !!appConfig.kill_switch;
  }
  if (els.blockLanToggle) {
    els.blockLanToggle.checked = !!appConfig.block_lan;
  }
  if (els.disableNetProbeToggle) {
    els.disableNetProbeToggle.checked = !!appConfig.disable_startup_net_probe;
  }
  if (els.clearServerBtn) {
    els.clearServerBtn.classList.toggle('hidden', !appConfig.custom_server_set);
  }
  if (els.torBridgeToggle) {
    els.torBridgeToggle.checked = !!appConfig.tor_bridge;
  }
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
    row.className = 'preset-item';
    const info = document.createElement('div');
    info.className = 'preset-info';
    const title = document.createElement('strong');
    title.textContent = `${p.name} (${(p.domains || []).length} site)`;
    const desc = document.createElement('span');
    desc.textContent = p.description || '';
    info.append(title, desc);

    const cb = document.createElement('input');
    cb.type = 'checkbox';
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
    chip.className = 'chip';
    const label = document.createTextNode(d + ' ');
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.title = `${d} sil`;
    btn.setAttribute('aria-label', `${d} sitesini listeden sil`);
    btn.textContent = '×';
    btn.addEventListener('click', async () => {
      btn.disabled = true;
      showInlineError(kind === 'bypass' ? els.bypassErr : els.tunnelErr, kind === 'bypass' ? els.customBypassInput : els.customTunnelInput, '');
      await commitConfig((cfg) => {
        if (kind === 'bypass') {
          cfg.custom_bypass = (cfg.custom_bypass || []).filter(x => x !== d);
        } else {
          cfg.custom_tunnel = (cfg.custom_tunnel || []).filter(x => x !== d);
        }
      }, `Site silindi (${kind}): ${d}`, true);
    });
    chip.append(label, btn);
    container.appendChild(chip);
  });
}

/* ---------- Wizard State Synchronization ---------- */
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

/* ---------- Status & Network Polling (Race-Safe + Visibility-Aware) ---------- */
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
    }
    renderStatus();
  } catch (err) {
    if (seq !== statusSeq || appShuttingDown || relaunchingAdmin) return;
    consecutiveStatusFails++;
    if (consecutiveStatusFails === 1) {
      addLog('Durum alınamadı: ' + err.message);
    }
    if (consecutiveStatusFails >= 2) {
      setOfflineState(true);
    }
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
  if (!document.hidden) {
    refreshStatus(true);
  }
});

async function refreshNetInfo() {
  try {
    lastNetInfo = await api('/api/netinfo', {}, 10000);
    renderNetInfo();
  } catch {
    els.modemHint.textContent = 'Alınamadı';
    els.modemHint.title = 'Ağ bilgisi alınamadı';
  }
}

async function togglePower() {
  if (busy) return;
  busy = true;
  els.powerBtn.disabled = true;
  try {
    const isRunning = engineStatus && engineStatus.running;
    if (isRunning) {
      addLog('OzeVPN motoru durduruluyor...');
      await api('/api/disconnect', { method: 'POST' }, 20000);
      addLog('Bağlantı kesildi, sistem normal hatta döndü.');
    } else {
      els.powerBtn.className = 'power-btn connecting';
      els.statusText.textContent = 'Bağlanıyor...';
      els.statusSub.textContent = 'sing-box + WARP WireGuard (TCP+UDP) başlatılıyor (~5 sn)...';
      addLog('OzeVPN v6 (sing-box Hibrit Motor) başlatılıyor...');
      const res = await api('/api/connect', { method: 'POST' }, 60000);
      (res.logs || []).forEach(addLog);
    }
  } catch (err) {
    addLog('Hata: ' + err.message);
  } finally {
    busy = false;
    els.powerBtn.disabled = false;
    await refreshStatus(true);
  }
}

async function runProbe() {
  els.probeBtn.disabled = true;
  els.probeBtn.textContent = 'Ölçülüyor...';
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
    (rep.targets || []).forEach(item => {
      const card = document.createElement('div');
      card.className = 'probe-item ' + (item.reachable ? 'ok' : 'fail');
      const top = document.createElement('div');
      top.className = 'probe-top';
      const nameSpan = document.createElement('span');
      nameSpan.textContent = item.name || '';
      const msSpan = document.createElement('span');
      msSpan.className = 'probe-ms';
      msSpan.textContent = item.reachable ? `${item.latency_ms} ms` : 'Engelli/Hata';
      top.append(nameSpan, msSpan);

      const errDiv = document.createElement('div');
      errDiv.className = 'probe-err';
      errDiv.textContent = item.reachable ? `Erişilebilir (HTTP ${item.status_code})` : (item.error || 'Ulaşılamadı');
      card.append(top, errDiv);
      els.probeGrid.appendChild(card);
    });
  } catch (err) {
    els.probeGrid.replaceChildren();
    const errBox = document.createElement('div');
    errBox.className = 'probe-empty';
    errBox.textContent = 'Test hatası: ' + err.message;
    els.probeGrid.appendChild(errBox);
  } finally {
    els.probeBtn.disabled = false;
    els.probeBtn.textContent = 'Test Et';
  }
}

/* ---------- Event Listeners ---------- */
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
    }, 45000);
    appConfig = res.config;
    installStatus = res.install;
    renderConfig();
    renderInstall();
    (res.logs || []).forEach(addLog);
    closeModal(els.wizardOverlay);
    await refreshStatus(true);
  } catch (err) {
    showInlineError(els.wizardError, null, 'Kurulum hatası: ' + err.message);
    addLog('Kurulum hatası: ' + err.message);
  } finally {
    els.wizardFinishBtn.disabled = false;
    els.wizardSkipBtn.disabled = false;
    els.wizardFinishBtn.textContent = 'Kurulumu Tamamla ve Başlat';
  }
});

els.openWizardBtn.addEventListener('click', () => openWizard(els.openWizardBtn));

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
    const running = !!(engineStatus && engineStatus.running);
    const desc = running
      ? 'Aktif OzeVPN tüneli kesilecek, Windows proxy normale döndürülecek ve arka plan uygulaması tamamen kapanacak.'
      : 'OzeVPN arka plan uygulaması kapatılacak. Tekrar açmak için masaüstü kısayolunu kullanabilirsin.';
    const ok = await askConfirm('OzeVPN Kapatılsın mı?', desc, 'Evet, Kapat', els.quitAppBtn);
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
    addLog(targetStyle === 'resident' ? 'Yerleşik Mod etkinleştirildi.' : 'Aç/Kapa Moduna geçildi.');
  } catch (err) {
    els.toggleResidentBtn.textContent = prevTxt;
    addLog('Kurulum değişikliği hatası: ' + err.message);
  } finally {
    els.toggleResidentBtn.disabled = false;
  }
});

els.powerBtn.addEventListener('click', togglePower);
els.probeBtn.addEventListener('click', runProbe);

els.modeGrid.addEventListener('click', async (e) => {
  const card = e.target.closest('.mode-card');
  if (!card || !appConfig) return;
  const nextMode = card.dataset.mode;
  await commitConfig((cfg) => { cfg.mode = nextMode; }, `Mod seçildi: ${nextMode}`, true);
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
    const ok = await commitConfig((cfg) => { cfg.custom_server = val; }, 'Özel sunucu bağlantısı kaydedildi.');
    if (ok) {
      els.customServerInput.value = '';
    } else {
      showInlineError(els.customServerErr, els.customServerInput, 'Özel sunucu kaydedilemedi; bağlantıyı kontrol et.');
    }
  } finally {
    els.saveServerBtn.disabled = false;
    els.saveServerBtn.textContent = 'Kaydet';
  }
}

els.saveServerBtn.addEventListener('click', saveCustomServer);
els.customServerInput.addEventListener('keydown', (e) => {
  if (e.key === 'Enter') saveCustomServer();
});

if (els.adBlockToggle) {
  els.adBlockToggle.addEventListener('change', async () => {
    const nextVal = els.adBlockToggle.checked;
    els.adBlockToggle.disabled = true;
    await commitConfig(
      (cfg) => { cfg.ad_block_speed = nextVal; },
      `Reklam & Takipçi Hız Kalkanı: ${nextVal ? 'Açık' : 'Kapalı'}`,
      true
    );
    els.adBlockToggle.disabled = false;
  });
}

if (els.torBridgeToggle) {
  els.torBridgeToggle.addEventListener('change', async () => {
    const nextVal = els.torBridgeToggle.checked;
    els.torBridgeToggle.disabled = true;
    await commitConfig(
      (cfg) => { cfg.tor_bridge = nextVal; },
      `Tor Snowflake Köprü Modu: ${nextVal ? 'Açık (Yeniden bağlanınca aktif olur)' : 'Kapalı'}`
    );
    els.torBridgeToggle.disabled = false;
  });
}

if (els.killSwitchToggle) {
  els.killSwitchToggle.addEventListener('change', async () => {
    const nextVal = els.killSwitchToggle.checked;
    els.killSwitchToggle.disabled = true;
    await commitConfig(
      (cfg) => { cfg.kill_switch = nextVal; },
      `Kill Switch: ${nextVal ? 'Açık (Oyun TUN bağlantısında etkinleşir)' : 'Kapalı'}`
    );
    els.killSwitchToggle.disabled = false;
  });
}

if (els.blockLanToggle) {
  els.blockLanToggle.addEventListener('change', async () => {
    const nextVal = els.blockLanToggle.checked;
    els.blockLanToggle.disabled = true;
    await commitConfig(
      (cfg) => { cfg.block_lan = nextVal; },
      `Yerel Ağ (LAN) İzolasyonu: ${nextVal ? 'Açık' : 'Kapalı'}`,
      true
    );
    els.blockLanToggle.disabled = false;
  });
}

if (els.disableNetProbeToggle) {
  els.disableNetProbeToggle.addEventListener('change', async () => {
    const nextVal = els.disableNetProbeToggle.checked;
    els.disableNetProbeToggle.disabled = true;
    await commitConfig(
      (cfg) => { cfg.disable_startup_net_probe = nextVal; },
      `Başlangıç harici ülke sorgusu: ${nextVal ? 'Kapalı (Gizlilik)' : 'Açık'}`
    );
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
    } catch (err) {
      addLog('Özel sunucu silinemedi: ' + err.message);
    } finally {
      els.clearServerBtn.disabled = false;
    }
  });
}

if (els.newIpBtn) {
  els.newIpBtn.addEventListener('click', async () => {
    if (!engineStatus || !engineStatus.running) {
      addLog('Yeni Avrupa IP almak için önce güç düğmesine basıp bağlan.');
      return;
    }
    els.newIpBtn.disabled = true;
    els.newIpBtn.textContent = '🔄 Yenileniyor...';
    try {
      const r = await api('/api/new-identity', { method: 'POST' }, 35000);
      addLog(r.message || 'Yeni Avrupa IP alındı.');
    } catch (err) {
      addLog('IP yenileme hatası: ' + err.message);
    } finally {
      els.newIpBtn.disabled = false;
      els.newIpBtn.textContent = '🔄 Yeni Avrupa IP';
    }
  });
}

if (els.speedBtn) {
  els.speedBtn.addEventListener('click', async () => {
    els.speedBtn.disabled = true;
    els.speedBtn.textContent = '⚡ Ölçülüyor...';
    try {
      const s = await api('/api/speedtest', {}, 30000);
      if (s.error) {
        els.traceSummary.textContent = 'Hız testi hatası: ' + s.error;
      } else {
        const mode = s.via_proxy ? 'OzeVPN Tüneli' : 'Direkt Hat';
        const ts = nowStamp();
        els.traceSummary.textContent = `⚡ Hız (${mode}): ${s.download_mbps} Mbps • Ping: ${s.ping_ms} ms • Jitter: ${s.jitter_ms} ms (${ts})`;
        addLog(`Hız testi (${mode}): ${s.download_mbps} Mbps | Ping ${s.ping_ms} ms | Jitter ${s.jitter_ms} ms`);
      }
    } catch (err) {
      els.traceSummary.textContent = 'Hız testi hatası: ' + err.message;
      addLog('Hız testi hatası: ' + err.message);
    } finally {
      els.speedBtn.disabled = false;
      els.speedBtn.textContent = '⚡ Hız Ölç';
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
    const parts = (r.paths || []).map(p => `${p.path}: ${p.reachable ? '✅ ' + p.latency_ms + ' ms' : '❌ ' + (p.error || 'Engelli')}`);
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
    els.rescueResult.textContent = `✅ ${r.domain} Avrupa çıkışına eklendi ("PAC & Router Listesi" sekmesinde yönetebilirsin) — ${r.message}`;
    addLog(`${r.domain} Avrupa çıkışına alındı (${r.message})`);
  } catch (err) {
    els.rescueResult.textContent = 'Hata: ' + err.message;
  } finally {
    els.rescueTestBtn.disabled = false;
    els.rescueAddBtn.disabled = false;
    els.rescueAddBtn.textContent = "Avrupa'ya Al";
  }
}

if (els.rescueTestBtn) els.rescueTestBtn.addEventListener('click', runRescueTest);
if (els.rescueAddBtn) els.rescueAddBtn.addEventListener('click', runRescueAdd);
if (els.rescueInput) {
  els.rescueInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') runRescueTest();
  });
}

if (els.emergencyResetBtn) {
  els.emergencyResetBtn.addEventListener('click', async () => {
    const ok = await askConfirm(
      'Windows Proxy Sıfırlansın mı?',
      'Aktif OzeVPN tünelleri durdurulacak ve Windows sistem proxy ayarları tamamen temizlenerek normal internete dönülecek.',
      'Evet, Sıfırla',
      els.emergencyResetBtn
    );
    if (!ok) return;
    els.emergencyResetBtn.disabled = true;
    try {
      const r = await api('/api/emergency-reset', { method: 'POST' }, 15000);
      addLog(r.message || 'Proxy sıfırlandı.');
      await refreshStatus(true);
    } catch (err) {
      addLog('Sıfırlama hatası: ' + err.message);
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
    els.relaunchAdminBtn.textContent = '🛡️ UAC Bekleniyor...';
    setOfflineState(false);
    addLog('Yönetici (Oyun TUN) izni isteniyor — Windows UAC penceresinden "Evet" onayla...');
    try {
      await api('/api/relaunch-admin', { method: 'POST' }, 60000);
      els.relaunchAdminBtn.textContent = '🛡️ TUN Geçiliyor...';
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
        } catch {
          // Expected briefly (~300ms) while old process releases port and admin process binds it
        }
      }
      relaunchingAdmin = false;
      consecutiveStatusFails = 0;
      setOfflineState(false);
      if (switched) {
        renderStatus();
        addLog('✅ Yönetici (Oyun TUN) modu aktif! Artık Bağlan düğmesine basarak gerçek TUN ağ kartını açabilirsin.');
      } else {
        await refreshStatus(true);
        addLog('Yönetici geçişi tamamlandı ancak durum doğrulanamadı — sayfayı yenileyebilirsin.');
      }
    } catch (err) {
      relaunchingAdmin = false;
      addLog('Yönetici başlatma iptal edildi: ' + err.message);
      renderStatus();
    }
  });
}

/* ---------- WAI-ARIA Tabs (Click + ArrowLeft/ArrowRight/Home/End) ---------- */
const tabButtons = Array.from(document.querySelectorAll('.tab[role="tab"]'));

function activateTab(btn, moveFocus) {
  tabButtons.forEach(b => {
    const isSel = b === btn;
    b.classList.toggle('active', isSel);
    b.setAttribute('aria-selected', String(isSel));
    b.setAttribute('tabindex', isSel ? '0' : '-1');
    const panel = document.getElementById('tab-' + b.dataset.tab);
    if (panel) {
      panel.classList.toggle('active', isSel);
      panel.hidden = !isSel;
    }
  });
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

/* ---------- Custom Domains (Client Validation + Duplicate Guard + Safe Preservation) ---------- */
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
      if (isBypass) {
        cfg.custom_bypass = [...(cfg.custom_bypass || []), val];
      } else {
        cfg.custom_tunnel = [...(cfg.custom_tunnel || []), val];
      }
    }, `Site eklendi (${kind}): ${val}`, true);
    if (ok) {
      input.value = '';
    } else {
      showInlineError(errEl, input, 'Site kaydedilemedi; geçerli bir alan adı girdiğinden emin ol.');
    }
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

/* ---------- App Init ---------- */
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
