# OzeVPN v6 — Operasyon, SLO ve Kurtarma Rehberi

## 1. İstemci Hizmet Düzeyi Hedefleri (SLO)

| Metrik | Hedef (SLO) | Ölçüm Yöntemi |
| :--- | :--- | :--- |
| **Hot-Reload Süresi (`/api/reload`)** | `< 1.0 sn` (p95) | `tools/bench` / `ReloadRouting` |
| **Admin `Oyun TUN` `-takeover` Geçişi** | `< 2.5 sn` | Canlı E2E handoff ölçümü |
| **`sing-box` Çökme Toparlanması (Watchdog)** | `≤ 10 sn` (ilk yeniden başlatma 5 sn) | `GET /api/metrics` → `watchdog_restarts` |
| **`Tor` Çökme Toparlanması (Watchdog)** | `≤ 35 sn` (bootstrap dahil) | `GET /api/metrics` → `tor_restarts` |
| **Boşta Bellek Kullanımı (`ozevpn.exe`)** | `< 30 MB` RSS | `tools/bench` bellek profili |
| **Acil Kurtarma Süresi (RTO — Proxy + Kill Switch)** | `< 3 sn` | `POST /api/emergency-reset` veya tek satır PowerShell |
| **Ayar Kurtarma (RPO — Bozuk `ozevpn.json`)** | Sıfır veri kaybı (`.corrupt.bak` yedek) | `config.NewStore` otomatik yedekleme |

## 2. Yerel Gözlemlenebilirlik Uç Noktaları
- `GET /api/healthz` — Kimlik gerektirmeyen hafif liveness/readiness yanıtı (`ok`, `ready`, `running`, `singbox_alive`, `tor_alive`, `build_commit`, `build_date`).
- `GET /api/metrics` *(oturum tokeni gerekir)* — Atomik sayaçlar (`connects`, `disconnects`, `reloads`, `config_updates`, `security_blocks`, `watchdog_restarts`, `tor_restarts`, `uptime_sec`).

## 3. Anahtar Rotasyonu & Olay Müdahalesi (Runbook)
1. **Özel Sunucu Anahtar İhlali (VLESS / WireGuard):**
   - Panel → Gelişmiş Ağ Ayarları → Özel Sunucu → **Temizle** (`clear_custom_server: true`).
   - VPS üzerinde eski peer'ı hemen iptal et: `wg set wg0 peer <ESKI_PUBKEY> remove`.
   - Yeni anahtar çifti (`wg genkey` + `wg genpsk`) üretip yeniden kaydet.
2. **Kill Switch Kilitli Kalırsa (Uygulama Çökmesi Sonrası Acil İnternet Kurtarma):**
   - Uygulamayı açıp **Araçlar → Proxy Sıfırla** seçeneğine tıkla (veya `OzeVPN.exe -uninstall`).
   - Uygulama hiç açılmıyorsa Yönetici PowerShell'de şu iki komutu çalıştır:
     ```powershell
     netsh advfirewall set allprofiles firewallpolicy blockinbound,allowoutbound
     Get-NetFirewallRule -DisplayName 'OzeVPN-KS-*' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
     ```
3. **Tam Temiz Kaldırma (Uninstall):**
   - `OzeVPN.exe -uninstall` — Başlangıç kaydını (`HKCU\...\Run\OzeVPN`), Masaüstü kısayolunu, Windows Sistem Proxy'yi, Kill Switch kurallarını ve `%LOCALAPPDATA%\OzeVPN` çalışma verilerini temizler.
