# OzeVPN v4 — Hızlı, Güvenli & Sansürsüz İnternet Motoru

**OzeVPN v4**, `sing-box` çekirdeği üzerine kurulu; **paralel hibrit motor (WARP + Avrupa Tor)**, **şifreli DoH DNS**, **Reklam & Takipçi Hız Kalkanı**, **Akıllı TUN / Sistem Proxy** ve **CSRF korumalı** yerel kontrol panelidir.

## Hemen Çalıştır
- **[ozevpn.exe](ozevpn.exe)** dosyasına çift tıkla (tarayıcıda `http://127.0.0.1:7777` otomatik açılır).
- **Oyun / Roblox masaüstü istemcisi (UDP) için:** Sağ tık → **Yönetici olarak çalıştır** veya paneldeki **🎮 Oyun TUN (Yönetici)** butonuna tıkla.
- İlk açılışta **Kurulum Sihirbazı** sorar:
  - **⚡ Yerleşik Mod:** Kendini `%LOCALAPPDATA%\OzeVPN` altına kurar, Masaüstüne kısayol atar ve Windows her açıldığında sessizce (`-silent -autoconnect`) başlar.
  - **🔘 Aç / Kapa Modu:** Sadece sen açtığında çalışır.

## Neler İçeriyor?
1. **⚡ 6.4 Saniyede Paralel Bağlantı:** WARP Proxy ve Avrupa Tor (`DE/NL/BG/RO/AT`) eşzamanlı başlatılır; protokol otomatik seçilir (`MASQUE ↔ WireGuard`).
2. **🌍 4 Çıkış Motoru:**
   - **Hibrit (Varsayılan):** `.gov.tr`, `.edu.tr`, `.bel.tr`, TR bankalar ve Steam/Epic/Riot indirmeleri **%100 direkt hızda**; Roblox, Wattpad, PayPal, Booking ve eklediğin domainler **Avrupa Tor** üzerinden; geri kalan tüm dünya düşük pingli WARP'tan çıkar.
   - **Ultra Global (Saf Avrupa Tor):** Bilgisayarda hiçbir ek yazılım olmadan tüm dünyayı %100 Avrupa IP'siyle açar.
   - **Sadece Hızlı WARP:** En düşük pingli DPI aşma modu.
   - **Özel Sunucu:** `vless://`, `socks5://`, **WireGuard `.conf`** (örn. ProtonVPN Free veya kendi VPS'in) veya `sing-box` JSON yapıştırıp tek tıkla 40–50 ms Avrupa hızına geç.
3. **🛡️ Reklam & Takipçi Hız Kalkanı + Şifreli DoH DNS:** 28 ağır reklam/takipçi ağını paket çıkmadan keser; ISP DNS zehirlemesini önler.
4. **🔒 Güvenlik & Kendi Kendini Onarma:**
   - Harici web sitelerinin `127.0.0.1:7777` API'sine müdahale etmesini engelleyen **CSRF / Same-Origin** koruması.
   - Çökme durumunda `sing-box`'ı otomatik dirilten **Watchdog** ve başlangıçta takılı kalmış proxy'yi otomatik temizleyen **Auto-Heal** + **🧯 Proxy Sıfırla** acil butonu.
5. **📡 Faz 2: Modem Arkası OpenWrt Router & VPS:** [router/README.md](router/README.md), [router/openwrt-setup.sh](router/openwrt-setup.sh), [vps/setup-vps.sh](vps/setup-vps.sh).

## Testleri Çalıştır
```powershell
go vet ./...
go test -v ./...
```
