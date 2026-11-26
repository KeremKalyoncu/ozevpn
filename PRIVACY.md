# OzeVPN v6 — Gizlilik Bildirimi ve Veri Akışı Şeffaflığı

## 1. Yerel Çalışma & Sıfır Telemetri
- OzeVPN'in **kendi merkezi sunucusu, kullanıcı hesabı veritabanı, analitik SDK'sı veya reklam/izleyici kütüphanesi yoktur.**
- Kontrol paneli yalnızca yerel makinede (`127.0.0.1:7777`) çalışır ve `go.mod` sıfır üçüncü parti Go modülü içerir.
- Web arayüzü harici CDN, font veya script yüklemez (`Content-Security-Policy: default-src 'self'`).

## 2. Harici Ağ Çağrıları ve Güven Sınırları
| Bileşen / Özellik | Kime Bağlanır? | Ne Zaman Çalışır? | Kapatılabilir mi? |
| :--- | :--- | :--- | :--- |
| **Başlangıç Ağ/Ülke Tespiti** | `https://cloudflare.com/cdn-cgi/trace` | Uygulama açılışında 1 kez (60 sn önbellekli) | **Evet** — *Gelişmiş Ağ Ayarları → Başlangıçta Otomatik Harici Ülke Sorgusunu Kapat* (`disable_startup_net_probe: true`) |
| **Cloudflare WARP Çıkışı** | Cloudflare Edge (MASQUE HTTP/3 / WireGuard) | Yalnız kullanıcı `Bağlan` dediğinde (`hybrid_eu` veya `warp_fast`) | **Evet** — `tor_eu` veya `custom` profili seçilebilir |
| **Tor Avrupa Çıkışı** | Tor ağı (+ isteğe bağlı Snowflake WebRTC köprüsü) | Yalnız kullanıcı `Bağlan` dediğinde (`hybrid_eu` veya `tor_eu`) | **Evet** — `warp_fast` veya `custom` profili seçilebilir |
| **Şifreli DNS (DoH)** | `https://1.1.1.1` (aktif tünel outbound `detour` üzerinden) | Yalnız tünel aktifken | Özel sunucu/router yapılandırmasıyla değiştirilebilir |
| **Canlı Test & Hız Ölçümü** | Discord, Roblox, Pastebin, Google, e-Devlet, Cloudflare Speed | Yalnız kullanıcı `Test Et` / `⚡ Hız Ölç` butonuna bastığında | Manuel tetiklenir |

## 3. Anonimlik ve Sızıntı Sınırları (Dürüst Beyan)
- OzeVPN bir **sansür aşma ve trafik şifreleme aracıdır**; devlet düzeyi küresel trafik korelasyonuna karşı mutlak anonimlik garantisi vermez.
- **Standart Mod (Proxy):** Yalnızca Windows WinINET proxy kullanan tarayıcı ve uygulamaları korur; proxy-dışı uygulamalar ve UDP oyun trafiği doğrudan fiziksel hattan çıkar.
- **Yönetici Modu (Oyun TUN):** Çift yığınlı (`IPv4 + IPv6`) sanal ağ kartı (`ozevpn0`) açar ve **Kill Switch** etkinleştirildiğinde Windows Güvenlik Duvarı üzerinden fail-closed koruma uygular.
- **Çıkış Sağlayıcıları:** Cloudflare WARP veya üçüncü parti özel sunucu kullanıldığında çıkış trafiği o sağlayıcının politikasına tabidir.

## 4. Yerel Veri ve Log Saklama (Retention)
- `ozevpn.json` içindeki `custom_server` alanı Windows **DPAPI** (`CryptProtectData`) ile kullanıcıya özel şifrelenir ve NTFS DACL ile yalnızca mevcut kullanıcı + `SYSTEM` + `Administrators` erişimine kısıtlanır.
- `bin/sing-box.log` ve `bin/tor.log` dosyaları **en fazla 2 MB** ile sınırlandırılır (`cappedFileWriter`), `sing-box` yalnızca `warn` düzeyinde log tutar (ziyaret edilen site listesi saklanmaz) ve dosya izinleri sahibiyle kısıtlanır.
- `bin/sing-box-active.json` bağlantı kesildiğinde diskten otomatik silinir.
- Tüm yerel verileri tek komutla kaldırmak için: `OzeVPN.exe -uninstall`.
