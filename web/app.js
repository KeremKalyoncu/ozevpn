let appConfig = null;
let engineStatus = null;
let installStatus = null;
let busy = false;

let wizardInstallChoice = 'resident';
let wizardModeChoice = 'smart';

const els = {
  wizardOverlay: document.getElementById('wizardOverlay'),
  wizardInstallChoices: document.getElementById('wizardInstallChoices'),
  wizardModeChoices: document.getElementById('wizardModeChoices'),
  wizardSkipBtn: document.getElementById('wizardSkipBtn'),
  wizardFinishBtn: document.getElementById('wizardFinishBtn'),
  openWizardBtn: document.getElementById('openWizardBtn'),
  quitAppBtn: document.getElementById('quitAppBtn'),
  newIpBtn: document.getElementById('newIpBtn'),
  relaunchAdminBtn: document.getElementById('relaunchAdminBtn'),
  emergencyResetBtn: document.getElementById('emergencyResetBtn'),
  adBlockToggle: document.getElementById('adBlockToggle'),
  torBridgeToggle: document.getElementById('torBridgeToggle'),
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
  addBypassBtn: document.getElementById('addBypassBtn'),
  customBypassChips: document.getElementById('customBypassChips'),
  customTunnelInput: document.getElementById('customTunnelInput'),
  addTunnelBtn: document.getElementById('addTunnelBtn'),
  customTunnelChips: document.getElementById('customTunnelChips'),
};

function addLog(msg) {
  const div = document.createElement('div');
  div.className = 'log-line';
  div.textContent = msg;
  els.logBox.appendChild(div);
  els.logBox.scrollTop = els.logBox.scrollHeight;
}

async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  const data = await res.json();
  if (!res.ok) throw new Error(data.error || 'İstek başarısız');
  return data;
}

function renderInstall() {
  if (!installStatus) return;
  if (installStatus.resident_enabled) {
    els.installBadge.textContent = 'Mod: ⚡ Yerleşik';
    els.residentTitle.textContent = 'Yerleşik Mod: Aktif';
    els.residentSub.textContent = 'Windows her açıldığında sessizce başlar ve bağlanır.';
    els.toggleResidentBtn.textContent = 'Aç/Kapa Yap';
  } else {
    els.installBadge.textContent = 'Mod: 🔘 Aç/Kapa';
    els.residentTitle.textContent = 'Yerleşik Mod: Kapalı';
    els.residentSub.textContent = 'Sadece sen açıp güç düğmesine bastığında çalışır.';
    els.toggleResidentBtn.textContent = 'Yerleşik Yap';
  }
}

function renderStatus() {
  if (!engineStatus) return;
  const st = (engineStatus.state || 'Disconnected').toLowerCase();
  els.powerBtn.className = 'power-btn ' + (st === 'connected' ? 'connected' : st === 'connecting' ? 'connecting' : 'disconnected');
  els.brandDot.className = 'logo-dot ' + (st === 'connected' ? 'connected' : st === 'connecting' ? 'connecting' : '');

  if (st === 'connected') {
    els.statusText.textContent = 'Bağlı — Tüm Dünya Engelsiz';
    els.statusSub.textContent = `${engineStatus.capture_layer || 'sing-box Aktif'} • Motor: ${engineStatus.active_profile || 'hybrid_eu'}`;
  } else if (st === 'connecting') {
    els.statusText.textContent = 'Bağlanıyor...';
    els.statusSub.textContent = 'Hibrit sing-box motoru başlatılıyor...';
  } else {
    els.statusText.textContent = 'Bağlı Değil';
    els.statusSub.textContent = engineStatus.last_error
      ? `Hata: ${engineStatus.last_error}`
      : 'Tek tıkla Discord, Roblox ve tüm engelli siteleri aç';
  }

  const layer = engineStatus.is_admin ? 'TUN (Yönetici)' : 'Proxy (Standart)';
  const up = engineStatus.running && engineStatus.uptime_sec > 0 ? ` • ${engineStatus.uptime_sec}sn` : '';
  els.protoBadge.textContent = `Katman: ${engineStatus.running ? engineStatus.capture_layer : layer}${up}`;
  if (els.relaunchAdminBtn) {
    els.relaunchAdminBtn.classList.toggle('hidden', !!engineStatus.is_admin);
  }
}

function renderConfig() {
  if (!appConfig) return;
  document.querySelectorAll('.mode-card').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.mode === appConfig.mode);
  });
  els.engineProfileSelect.value = appConfig.engine_profile || 'hybrid_eu';
  els.protocolSelect.value = appConfig.protocol || 'Auto';
  els.customServerInput.value = '';
  els.customServerInput.placeholder = appConfig.custom_server_set
    ? '🔒 Kayıtlı özel sunucu aktif (değiştirmek için yenisini yapıştır)'
    : 'vless://, socks5:// veya WireGuard .conf yapıştır';
  els.customServerBox.classList.toggle('hidden', els.engineProfileSelect.value !== 'custom');
  if (els.adBlockToggle) {
    els.adBlockToggle.checked = !!appConfig.ad_block_speed;
  }
  if (els.torBridgeToggle) {
    els.torBridgeToggle.checked = !!appConfig.tor_bridge;
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
    title.textContent = `${p.name} (${(p.domains || []).length} domain)`;
    const desc = document.createElement('span');
    desc.textContent = p.description || '';
    info.append(title, desc);

    const cb = document.createElement('input');
    cb.type = 'checkbox';
    cb.checked = !!p.enabled;
    cb.addEventListener('change', async () => {
      p.enabled = cb.checked;
      await saveConfig();
      addLog(`Liste güncellendi: ${p.name}`);
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
    btn.title = 'Sil';
    btn.textContent = '×';
    btn.addEventListener('click', async () => {
      if (kind === 'bypass') {
        appConfig.custom_bypass = appConfig.custom_bypass.filter(x => x !== d);
      } else {
        appConfig.custom_tunnel = appConfig.custom_tunnel.filter(x => x !== d);
      }
      await saveConfig();
      renderConfig();
    });
    chip.append(label, btn);
    container.appendChild(chip);
  });
}

async function saveConfig() {
  appConfig = await api('/api/config', {
    method: 'POST',
    body: JSON.stringify(appConfig),
  });
}

async function refreshStatus() {
  try {
    engineStatus = await api('/api/status');
    renderStatus();
  } catch (err) {
    addLog('Durum alınamadı: ' + err.message);
  }
}

async function refreshNetInfo() {
  try {
    const ni = await api('/api/netinfo');
    const ctry = ni.country && ni.country !== 'AUTO' ? ` • Ülke: ${ni.country}` : '';
    els.modemHint.textContent = (ni.modem_hint || 'Tespit edilemedi') + ctry;
    els.dnsInfo.textContent = (ni.dns_servers && ni.dns_servers.length) ? ni.dns_servers.slice(0, 2).join(', ') : 'Otomatik';
    els.ipv6Info.textContent = ni.ipv6_active ? 'Aktif' : 'Kapalı';
  } catch {
    els.modemHint.textContent = 'Alınamadı';
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
      await api('/api/disconnect', { method: 'POST' });
      addLog('Bağlantı kesildi, sistem normal hatta döndü.');
    } else {
      els.powerBtn.className = 'power-btn connecting';
      els.statusText.textContent = 'Bağlanıyor...';
      els.statusSub.textContent = 'sing-box + Avrupa çıkış motoru başlatılıyor (~10 sn)...';
      addLog('OzeVPN v3 (sing-box Hibrit Motor) başlatılıyor...');
      const res = await api('/api/connect', { method: 'POST' });
      (res.logs || []).forEach(addLog);
    }
  } catch (err) {
    addLog('Hata: ' + err.message);
  } finally {
    busy = false;
    els.powerBtn.disabled = false;
    await refreshStatus();
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
    const rep = await api('/api/probe');
    const t = rep.trace || {};
    if (t.ip) {
      const viaTxt = rep.via_proxy ? 'OzeVPN Tüneli Aktif' : 'Direkt Hat';
      els.coloBadge.textContent = `Çıkış: ${t.colo || '—'} (${t.loc || '—'})`;
      els.traceSummary.textContent = `IP: ${t.ip} • Bölge: ${t.loc || '?'} • Sunucu: ${t.colo || '?'} • ${viaTxt} (${rep.timestamp})`;
    } else {
      els.traceSummary.textContent = 'Trace bilgisi alınamadı.';
    }

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

// Wizard bindings
els.wizardInstallChoices.addEventListener('click', (e) => {
  const btn = e.target.closest('.wizard-choice');
  if (!btn) return;
  wizardInstallChoice = btn.dataset.install;
  els.wizardInstallChoices.querySelectorAll('.wizard-choice').forEach(b => b.classList.toggle('active', b === btn));
});

els.wizardModeChoices.addEventListener('click', (e) => {
  const btn = e.target.closest('.wizard-choice');
  if (!btn) return;
  wizardModeChoice = btn.dataset.mode;
  els.wizardModeChoices.querySelectorAll('.wizard-choice').forEach(b => b.classList.toggle('active', b === btn));
});

els.wizardSkipBtn.addEventListener('click', async () => {
  els.wizardOverlay.classList.add('hidden');
  if (appConfig && !appConfig.setup_completed) {
    appConfig.setup_completed = true;
    await saveConfig();
  }
});

els.wizardFinishBtn.addEventListener('click', async () => {
  els.wizardFinishBtn.disabled = true;
  els.wizardFinishBtn.textContent = 'Kuruluyor ve Bağlanılıyor...';
  try {
    const res = await api('/api/install/setup', {
      method: 'POST',
      body: JSON.stringify({
        install_style: wizardInstallChoice,
        mode: wizardModeChoice,
        engine_profile: 'hybrid_eu',
        connect_now: true,
      }),
    });
    appConfig = res.config;
    installStatus = res.install;
    renderConfig();
    renderInstall();
    (res.logs || []).forEach(addLog);
    els.wizardOverlay.classList.add('hidden');
    await refreshStatus();
  } catch (err) {
    addLog('Kurulum hatası: ' + err.message);
  } finally {
    els.wizardFinishBtn.disabled = false;
    els.wizardFinishBtn.textContent = 'Kurulumu Tamamla ve Başlat';
  }
});

els.openWizardBtn.addEventListener('click', () => {
  els.wizardOverlay.classList.remove('hidden');
});

if (els.quitAppBtn) {
  els.quitAppBtn.addEventListener('click', async () => {
    try {
      addLog('OzeVPN kapatılıyor...');
      await api('/api/shutdown', { method: 'POST' });
      window.close();
    } catch (err) {
      addLog('Kapatma hatası: ' + err.message);
    }
  });
}

els.toggleResidentBtn.addEventListener('click', async () => {
  const targetStyle = (installStatus && installStatus.resident_enabled) ? 'on_demand' : 'resident';
  try {
    const res = await api('/api/install/setup', {
      method: 'POST',
      body: JSON.stringify({
        install_style: targetStyle,
        mode: appConfig ? appConfig.mode : 'smart',
        connect_now: false,
      }),
    });
    appConfig = res.config;
    installStatus = res.install;
    renderInstall();
    addLog(targetStyle === 'resident' ? 'Yerleşik Mod etkinleştirildi.' : 'Aç/Kapa Moduna geçildi.');
  } catch (err) {
    addLog('Kurulum değişikliği hatası: ' + err.message);
  }
});

els.powerBtn.addEventListener('click', togglePower);
els.probeBtn.addEventListener('click', runProbe);

els.modeGrid.addEventListener('click', async (e) => {
  const card = e.target.closest('.mode-card');
  if (!card || !appConfig) return;
  appConfig.mode = card.dataset.mode;
  renderConfig();
  await saveConfig();
  addLog(`Mod seçildi: ${appConfig.mode}`);
});

els.engineProfileSelect.addEventListener('change', async () => {
  appConfig.engine_profile = els.engineProfileSelect.value;
  els.customServerBox.classList.toggle('hidden', appConfig.engine_profile !== 'custom');
  await saveConfig();
  addLog(`Motor profili seçildi: ${appConfig.engine_profile}`);
});

els.protocolSelect.addEventListener('change', async () => {
  appConfig.protocol = els.protocolSelect.value;
  await saveConfig();
  addLog(`WARP protokolü seçildi: ${appConfig.protocol}`);
});

els.saveServerBtn.addEventListener('click', async () => {
  appConfig.custom_server = els.customServerInput.value.trim();
  await saveConfig();
  addLog('Özel sunucu linki kaydedildi.');
});

if (els.adBlockToggle) {
  els.adBlockToggle.addEventListener('change', async () => {
    appConfig.ad_block_speed = els.adBlockToggle.checked;
    await saveConfig();
    addLog(`Reklam & Takipçi Hız Kalkanı: ${appConfig.ad_block_speed ? 'Açık' : 'Kapalı'}`);
    if (engineStatus && engineStatus.running) {
      try {
        const r = await api('/api/reload', { method: 'POST' });
        addLog(r.message || 'Kurallar anında yenilendi.');
      } catch {}
    }
  });
}

if (els.torBridgeToggle) {
  els.torBridgeToggle.addEventListener('change', async () => {
    appConfig.tor_bridge = els.torBridgeToggle.checked;
    await saveConfig();
    addLog(`Tor Snowflake Köprü Modu: ${appConfig.tor_bridge ? 'Açık (Yeniden bağlanınca aktif olur)' : 'Kapalı'}`);
  });
}

if (els.newIpBtn) {
  els.newIpBtn.addEventListener('click', async () => {
    els.newIpBtn.disabled = true;
    els.newIpBtn.textContent = '🔄 Yenileniyor...';
    try {
      const r = await api('/api/new-identity', { method: 'POST' });
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
      const s = await api('/api/speedtest');
      if (s.error) {
        els.traceSummary.textContent = 'Hız testi hatası: ' + s.error;
      } else {
        const mode = s.via_proxy ? 'OzeVPN Tüneli' : 'Direkt Hat';
        els.traceSummary.textContent = `⚡ Hız (${mode}): ${s.download_mbps} Mbps • Ping: ${s.ping_ms} ms • Jitter: ${s.jitter_ms} ms`;
        addLog(`Hız testi (${mode}): ${s.download_mbps} Mbps | Ping ${s.ping_ms} ms | Jitter ${s.jitter_ms} ms`);
      }
    } catch (err) {
      addLog('Hız testi hatası: ' + err.message);
    } finally {
      els.speedBtn.disabled = false;
      els.speedBtn.textContent = '⚡ Hız Ölç';
    }
  });
}

if (els.rescueTestBtn) {
  els.rescueTestBtn.addEventListener('click', async () => {
    const dom = els.rescueInput.value.trim();
    if (!dom) return;
    els.rescueResult.textContent = 'Direkt, Tünel ve Avrupa Tor yolları test ediliyor...';
    try {
      const r = await api('/api/rescue', { method: 'POST', body: JSON.stringify({ domain: dom, add_to_eu: false }) });
      const parts = (r.paths || []).map(p => `${p.path}: ${p.reachable ? '✅ ' + p.latency_ms + 'ms' : '❌ ' + (p.error || 'Engelli')}`);
      els.rescueResult.textContent = `${r.domain} → ${parts.join(' | ')} (Öneri: ${r.recommended})`;
    } catch (err) {
      els.rescueResult.textContent = 'Test hatası: ' + err.message;
    }
  });
}

if (els.rescueAddBtn) {
  els.rescueAddBtn.addEventListener('click', async () => {
    const dom = els.rescueInput.value.trim();
    if (!dom) return;
    try {
      const r = await api('/api/rescue', { method: 'POST', body: JSON.stringify({ domain: dom, add_to_eu: true }) });
      appConfig = r.config;
      renderConfig();
      els.rescueInput.value = '';
      els.rescueResult.textContent = `✅ ${r.domain} Avrupa çıkışına eklendi — ${r.message}`;
      addLog(`${r.domain} Avrupa çıkışına alındı (${r.message})`);
    } catch (err) {
      els.rescueResult.textContent = 'Hata: ' + err.message;
    }
  });
}

if (els.emergencyResetBtn) {
  els.emergencyResetBtn.addEventListener('click', async () => {
    try {
      const r = await api('/api/emergency-reset', { method: 'POST' });
      addLog(r.message || 'Proxy sıfırlandı.');
      await refreshStatus();
    } catch (err) {
      addLog('Sıfırlama hatası: ' + err.message);
    }
  });
}

if (els.relaunchAdminBtn) {
  els.relaunchAdminBtn.addEventListener('click', async () => {
    try {
      addLog('Yönetici (Oyun TUN) izni isteniyor...');
      await api('/api/relaunch-admin', { method: 'POST' });
    } catch (err) {
      addLog('Yönetici başlatma iptal edildi: ' + err.message);
    }
  });
}

document.querySelectorAll('.tab').forEach(btn => {
  btn.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
    btn.classList.add('active');
    document.getElementById('tab-' + btn.dataset.tab).classList.add('active');
  });
});

async function addCustomDomain(kind) {
  const input = kind === 'bypass' ? els.customBypassInput : els.customTunnelInput;
  const val = input.value.trim();
  if (!val) return;
  if (kind === 'bypass') {
    appConfig.custom_bypass = [...(appConfig.custom_bypass || []), val];
  } else {
    appConfig.custom_tunnel = [...(appConfig.custom_tunnel || []), val];
  }
  input.value = '';
  await saveConfig();
  renderConfig();
  addLog(`Domain eklendi (${kind}): ${val}`);
}

els.addBypassBtn.addEventListener('click', () => addCustomDomain('bypass'));
els.addTunnelBtn.addEventListener('click', () => addCustomDomain('tunnel'));
els.customBypassInput.addEventListener('keydown', e => { if (e.key === 'Enter') addCustomDomain('bypass'); });
els.customTunnelInput.addEventListener('keydown', e => { if (e.key === 'Enter') addCustomDomain('tunnel'); });

(async function init() {
  try {
    [appConfig, installStatus] = await Promise.all([
      api('/api/config'),
      api('/api/install/status'),
    ]);
    renderConfig();
    renderInstall();
    if (!appConfig.setup_completed) {
      els.wizardOverlay.classList.remove('hidden');
    }
    refreshNetInfo();
    await refreshStatus();
    setInterval(refreshStatus, 5000);
  } catch (err) {
    addLog('Başlatma hatası: ' + err.message);
  }
})();
