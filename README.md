# OzeVPN v6.1

> **Sıfır üçüncü parti Go/npm bağımlılıklı, tek-dosya (`.exe`), yerleşik Cloudflare WARP WireGuard (TCP+UDP) + Avrupa Tor hibrit motorlu, Canlı Sinyal Kokpiti arayüzlü yerel Windows masaüstü VPN uygulaması.**

[![Platform](https://img.shields.io/badge/Platform-Windows%2010%2F11%20x64-0078D6)](#kurulum--hızlı-başlangıç)
[![Core](https://img.shields.io/badge/Core-sing--box%201.14%2B%20%7C%20WARP%20WG%20UDP%20%7C%20Tor-0EA5E9)](#mimari)
[![Dependencies](https://img.shields.io/badge/Go%20%2F%20NPM%20Deps-0%20External-6366F1)](#mimari)
[![Security](https://img.shields.io/badge/Master%20VPN%20Audit-88.7%2F100%20(0%20FAIL)-22C55E)](#güvenlik-ve-gizlilik-mimarisi)
[![UI Craft](https://img.shields.io/badge/Frontend%20Craft%20v4-85.4%2F100%20(Craft%2095.7)-0EA5E9)](docs/frontend-craft/DESIGN.md)

---

## Canlı Sinyal Kokpiti (Signal Cockpit UI)

| Koyu Tema (Obsidian Cockpit) | Açık Tema (Cool Porcelain) |
| :---: | :---: |
| ![OzeVPN v6.1 Koyu Tema](docs/screenshots/signal-cockpit-dark.png) | ![OzeVPN v6.1 Açık Tema](docs/screenshots/signal-cockpit-light.png) |

---

## Öne Çıkan Özellikler

- **Tek Dosya Masaüstü Uygulaması (`dist/OzeVPN.exe`):** `sing-box 1.14.4`, `Tor 0.4.9.17`, `lyrebird` (Snowflake WebRTC PT) ve GeoIP veritabanlarını 5/5 SHA256 doğrulamayla tek ikilide toplar. Electron/Chromium şişkinliği olmadan yerel Edge/Chrome `--app` penceresi ve Win32 System Tray (özel kalkan ikonu) ile çalışır.
- **Yerleşik Cloudflare WARP WireGuard (Tam Layer-3 TCP + UDP — Harici Kurulumsuz):**
  - `sing-box 1.14+` içinde doğrudan çalışan yerel WireGuard uç noktası (`warp-fast`) otomatik X25519 anahtar çiftiyle kaydolur ve özel anahtarı Windows **DPAPI + NTFS DACL** ile saklar.
  - **Roblox (RakNet UDP / Error 279 koruması)**, **Discord Ses (UDP)** ve online oyunlar için tam UDP tüneli, `MTU 1280` eşitlemesi ve `25 sn` CGNAT keepalive ile çalışır; WireGuard UDP engellenen ağlarda otomatik `warp-cli` / Tor yedeğine geçer.
- **Paralel Hibrit Çıkış Motoru (4 Profil):**
  - **Hibrit (`hybrid_eu`, Varsayılan):** TR resmi (`.gov.tr`, `.edu.tr`), bankalar ve 4K/oyun indirme CDN'leri direkt tam hızda; Roblox, Discord ve genel internet yerleşik WARP WireGuard (TCP+UDP) üzerinden; AB konumu gerektiren siteler (`paypal.com`, `booking.com`, `wattpad.com` ve kullanıcı listesi) yalnızca TCP olarak Avrupa Tor üzerinden akar.
  - **Saf Avrupa Tor (`tor_eu`):** Harici kurulum gerektirmeden tüm TCP trafiğini Avrupa Tor (+ isteğe bağlı Snowflake köprü) üzerinden çıkarır.
  - **Sadece Hızlı WARP (`warp_fast`):** En düşük gecikmeli tam TCP+UDP tüneli.
  - **Özel Sunucu (`custom`):** `VLESS+TLS/Reality`, **WireGuard `.conf` (Post-Quantum `PresharedKey` destekli)** veya izinli `sing-box` JSON.
- **Standart Proxy + Yönetici (`Oyun TUN`) Akıllı Geçişi:**
  - **Standart Mod:** UAC izni istemeden Windows WinINET proxy (`127.0.0.1:10808`) üzerinden tarayıcı ve proxy uyumlu uygulamaları korur.
  - **Yönetici Modu (`Oyun TUN`):** `-no-browser -takeover` ile mevcut pencereyi kapatmadan devralır; çift yığınlı (`IPv4 + IPv6`, WireGuard ile uyumlu `MTU 1280`) `ozevpn0` TUN arayüzü açar, oyun UDP trafiğini yakalar ve isteğe bağlı **Windows Firewall Fail-Closed Kill Switch**'i etkinleştirir.
- **Canlı Sinyal Kokpiti Arayüzü ([DESIGN.md](docs/frontend-craft/DESIGN.md)):** Tek satır `47px` üst çubuk + erişilebilir `Araçlar` menüsü, gerçek verili **Canlı Tünel Rota Şeması** (`Cihaz → Katman → Tünel → Çıkış`), bağlanınca otomatik çıkış konumu tespiti, 27 adet tutarlı `1.75px` inline SVG ikon (`0` OS emojisi), erişilebilir CSS Switch'ler, 3'lü `aria-live` Toast yığını, hız/gecikme telemetri şeridi, klavye kısayolları (`Alt+C`, `Alt+T`, `/`) ve `<120 KB` toplam web bütçesi.

---

## Mimari

```mermaid
flowchart LR
    subgraph User["Yerel Windows Kullanıcısı"]
        UI["Masaüstü Paneli (Edge/Chrome --app)\nWebRTC non-proxied UDP kapalı"]
        TRAY["Win32 System Tray\nWM_POWERBROADCAST Uyanış"]
        CFG["ozevpn.json (DPAPI + NTFS DACL)\n.ozevpn_token (256-bit, NTFS DACL)"]
    end

    subgraph Control["Kontrol Düzlemi (127.0.0.1:7777)"]
        API["Go HTTP API (0 Harici Modül)\n256-bit X-OzeVPN-Token + CSRF + Rate Limit"]
    end

    subgraph Data["Veri Düzlemi (sing-box 1.14+)"]
        PROXY["mixed-in 127.0.0.1:10808\n(Standart Mod)"]
        TUN["tun-in ozevpn0 (IPv4+IPv6, MTU 1400)\n+ Windows Firewall Kill Switch (Admin)"]
        DNS["DoH 1.1.1.1 (Tünel Detour)\n+ 28 Reklam/Takipçi Engeli"]
    end

    subgraph Exit["Çıkış Katmanları"]
        WARP["Cloudflare WARP\n(MASQUE HTTP/3 / WireGuard)"]
        TOR["Tor 0.4.9.17 + Snowflake\n(Avrupa Çıkış + Watchdog)"]
        CUST["Özel Sunucu\n(VLESS+TLS/Reality | WG+PQ PSK)"]
        DIR["Direkt Hat (TR & 4K CDN Bypass)"]
    end

    UI -->|"HttpOnly Cookie + X-OzeVPN-Token"| API
    TRAY -.-> API
    API --> CFG
    API -.->|"sing-box check + Watchdog"| TUN
    PROXY --> DNS
    TUN --> DNS
    DNS --> WARP
    DNS --> TOR
    DNS --> CUST
    DNS --> DIR
```

---

## Güvenlik ve Gizlilik Mimarisi

OzeVPN, kapsamlı **Master VPN Audit v2.0** (18 kategori, 155 kontrol) kapsamında sertleştirilmiş ve **`88.7 / 100` (`122 PASS`, `0 FAIL`)** doğrulanmış puanla test edilmiştir:

| Katman | Uygulanan Güvenlik Kontrolü |
| :--- | :--- |
| **Yerel IPC & Kimlik** | Her oturumda `crypto/rand` 256-bit token (`/auth?t=...` → `HttpOnly; SameSite=Strict` + tüm mutasyonlarda zorunlu `X-OzeVPN-Token`), `Host` DNS-Rebinding & `Origin`/`Sec-Fetch-Site` CSRF engeli, rate/concurrency limit. |
| **Anahtar & Dosya İzolasyonu** | `custom_server` sırları Windows **DPAPI** (`CryptProtectData`) ile şifrelenir; `ozevpn.json`, `.ozevpn_token`, runtime ve log dosyalarına NTFS DACL (yalnız sahip + `SYSTEM` + `Administrators`, miras kapalı) uygulanır. Bağlantı kesildiğinde `sing-box-active.json` silinir. |
| **Protokol Sertleştirme** | Şifresiz `vless://`, uzak düz metin `socks5://`, `allowInsecure=1` ve ham JSON `tls.insecure:true` reddedilir; WireGuard resmi Post-Quantum `PresharedKey` desteklenir; hata çıktılarında sırlar maskelenir (`RedactSecrets`). |
| **Sızıntı Koruması** | Dual-stack `IPv4 + IPv6` TUN (`fdfe:dcba:9876::1/126`), aktif tünel `detour`'lu şifreli DoH DNS (`1.1.1.1`), bağlantı geçişlerinde `ipconfig /flushdns`, UI WebRTC non-proxied UDP engeli ve opsiyonel LAN izolasyonu. |
| **Kill Switch & Dayanıklılık** | Yönetici TUN modunda WFP tabanlı Windows Firewall `OzeVPN-KS-*` + `blockoutbound` Kill Switch, çift `sing-box` + `tor` Watchdog (5s→60s üstel backoff), uyku/uyanma (`WM_POWERBROADCAST`) ve ağ değişimi kurtarması. |
| **Tedarik Zinciri & Gizlilik** | `0` harici Go/npm bağımlılığı, 5/5 çalışma zamanı SHA256 sabitlemesi (`BINARY_CHECKSUMS.txt`), CycloneDX 1.5 SBOM, CI `govulncheck` + fuzzing kapısı, sıfır telemetri ve başlangıç harici sorgu kapatma anahtarı. |

> [!NOTE]
> **Kapsam Şeffaflığı:** Standart (yönetici olmayan) mod yalnızca Windows WinINET proxy kullanan tarayıcı/uygulamaları korur; tüm sistem (IPv4+IPv6+UDP) ve Kill Switch koruması için paneldeki **🎮 Oyun TUN** (Yönetici) modunu kullanın. Ayrıntılar için bkz. [PRIVACY.md](PRIVACY.md).

---

## Ölçülen Performans ve Kaynak Profili

| Metrik | Doğrulanmış Ölçüm |
| :--- | :--- |
| **WARP Tünel Hızı / Gecikme** | `58.1 Mbps`, `30 ms` ping, `11 ms` jitter *(canlı E2E)* |
| **Tor EU Yedek Tünel (`TXL`)** | `4.3 Mbps`, `p50=131.3 ms`, `%0.0` kayıp *(canlı `tools/bench`)* |
| **Aktif Yük Altında CPU** | `OzeVPN` `%0.17` • `sing-box` `%0.49` • `tor` `%1.98` *(toplam `< %2.7`)* |
| **Bellek Kullanımı** | `sing-box` `~36.5 MB` • `OzeVPN` `~73.5 MB` *(57 MB gömülü EXE dahil)* |
| **Hot-Reload / Admin Geçişi** | Hot-Reload `< 1 sn` • `-takeover` Admin geçişi `1.39 sn` |
| **Fuzzing & Race Kararlılığı** | 14/14 paket `go test -race` yeşil • `595.453` fuzz girdisi, `0` çökme |

---

## Kurulum & Hızlı Başlangıç

### Son Kullanıcı (Tek Dosya `.exe`)
1. `dist\OzeVPN.exe` dosyasını çalıştırın (veya masaüstü kısayoluna çift tıklayın).
2. Kontrol paneli otomatik olarak güvenli oturumla açılır ve Sistem Tepsisine (Tray) yerleşir.
3. Tam sistem / oyun UDP / Kill Switch koruması için üst çubuktan **🎮 Oyun TUN** butonuna tıklayın.
4. **Tam Temiz Kaldırma:**
   ```powershell
   .\dist\OzeVPN.exe -uninstall
   ```

### Geliştirici: Test, Benchmark ve Tek-EXE Derleme
Gereksinimler: **Go 1.22+** (Windows `amd64`) ve `BINARY_CHECKSUMS.txt` ile eşleşen `bin/` ikilileri.

```powershell
# Statik analiz, race detector, tüm birim/güvenlik/lab/sing-box şema testleri
go vet ./...
go test -race -cover ./...

# Canlı Baseline vs Tünel performans & p50/p95/p99 benchmark
go run ./tools/bench -samples 10 -bytes 3000000

# 5/5 SHA256 doğrulamalı tek-dosya EXE + CycloneDX SBOM derlemesi
.\build-single-exe.ps1
```

---

## Faz 2: Tüm Ev (OpenWrt Router) & Kendi VPS'in

- **OpenWrt Router (`sing-box 1.14+` + SQM/CAKE):** Panelden `sing-box-openwrt.json` dışa aktarın ve [router/openwrt-setup.sh](router/openwrt-setup.sh) ile [router/README.md](router/README.md) adımlarını izleyin.
- **Kendi Avrupa VPS'in (WireGuard + PQ PSK + BBR + UFW):** Ubuntu/Debian sunucuda [vps/setup-vps.sh](vps/setup-vps.sh) betiğini çalıştırın — default-deny UFW, SSH sertleştirme, Cloud Metadata (`169.254.0.0/16`) engeli, `chrony` ve `unattended-upgrades` otomatik kurulur.

---

## Dokümantasyon

| Belge | İçerik |
| :--- | :--- |
| [SECURITY.md](SECURITY.md) | Güvenlik savunma katmanları, acil Proxy/Kill Switch kurtarma ve güncelleme prosedürü |
| [PRIVACY.md](PRIVACY.md) | Veri akışları, harici çağrılar, anonimlik sınırları ve saklama politikası |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Mimari Karar Kayıtları (`ADR-001` – `ADR-007`) |
| [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) | Varlık envanteri, güven sınırları ve tam STRIDE matrisi |
| [docs/OPERATIONS.md](docs/OPERATIONS.md) | İstemci SLO hedefleri, `/api/healthz`, `/api/metrics` ve anahtar rotasyon runbook'u |
| [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md) | Gömülü üçüncü parti bileşenler ve lisans bildirimi (SBOM) |
