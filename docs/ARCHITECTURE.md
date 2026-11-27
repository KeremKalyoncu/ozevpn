# OzeVPN — Mimari Karar Kaydı (ADR)

## ADR-001: Tek Binary Go Monoliti + Gömülü Vanilla Web UI
- **Karar:** Electron/Chromium yerine Go standart kütüphanesi + `//go:embed web/*` kullanıldı.
- **Gerekçe:** Düşük RAM kullanımı, `<200 ms` başlangıç süresi, `0` üçüncü parti Go/npm bağımlılığı (sıfır tedarik zinciri şişkinliği).

## ADR-002: `sing-box` 1.14+ Hibrit Yönlendirme (Yerleşik WARP WireGuard + Avrupa Tor + Direkt TR/4K Bypass)
- **Karar:** Tüm trafik yerel `sing-box 1.14+` çekirdeğinden geçer; TR resmi (`.gov.tr`/`.edu.tr`), bankalar ve engelsiz 4K video/indirme CDN'leri `direct` çıkar; Roblox, Discord ve genel internet yerleşik Cloudflare WARP WireGuard (`warp-fast`, tam TCP+UDP) üzerinden; AB konumu gerektiren siteler (`paypal.com`, `booking.com`, `wattpad.com`, özel kullanıcı listesi) yalnızca TCP olarak `tor-eu` (`DE/NL/BG/RO/AT`) üzerinden akar.
- **Gerekçe:** Hem oyun/ses UDP uyumluluğu ve yüksek hız korunur hem de bölgesel/engelli siteler tek tıkla açılır.

## ADR-003: Akıllı TUN (Yönetici) + Otomatik Sistem Proxy Yedeği
- **Karar:** Yönetici izni varsa `sing-box` çift yığınlı (`IPv4+IPv6`) TUN sanal ağ kartı (`ozevpn0`, WireGuard ile `MTU 1280`, diğer profillerde `MTU 1400`) açılır; standart kullanıcıda UAC sormadan HKCU Windows Sistem Proxy (`127.0.0.1:10808`) kullanılır ve kapsam farkı UI'da açıkça etiketlenir.
- **Gerekçe:** En az yetki ilkesi ve sıfır kurulum sürtünmesi.

## ADR-004: Tek Kaynaklı `BuildSingBoxConfig` + Ön Doğrulamalı Hot-Reload
- **Karar:** `Start()` ve `ReloadRouting()` aynı `BuildSingBoxConfig()` fonksiyonunu kullanır, başlatmadan önce resmi `sing-box check` ile doğrular ve `engine_test.go` içinde 36 kombinasyonluk matrisle sürekli test edilir.
- **Gerekçe:** Yapılandırma sapmasını (drift) ve bozuk config başlatmayı önler.

## ADR-005: Dual-Stack IPv6 TUN + DoH Tünel Detour + OS DNS Önbellek Temizliği
- **Karar:** `tun-in` içine IPv6 CIDR (`fdfe:dcba:9876::1/126`) + dinamik MTU (`1280` WireGuard / `1400` diğerleri) eklendi; `dns-doh` aktif tünel outbound'una `detour` edildi; `dns-direct` düz UDP yerine şifreli HTTPS (`1.1.1.1`) yapıldı; bağlantı geçişlerinde `ipconfig /flushdns` çalıştırılır ve UI penceresinde `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` uygulanır.
- **Gerekçe:** Çift yığınlı Windows hostlarda native IPv6 sızıntısını ve ISP DNS/WebRTC sızıntılarını kapatır.

## ADR-006: 256-Bit Oturum IPC Tokeni + Windows DPAPI + NTFS DACL
- **Karar:** `127.0.0.1:7777` kontrol API'si her başlangıçta üretilen 256-bit `crypto/rand` tokeni (`HttpOnly; SameSite=Strict` + `X-OzeVPN-Token`) ile korunur (`-takeover` geçişinde DACL-korumalı `.ozevpn_token` ile devralınır). `custom_server` ve yerleşik WARP WireGuard özel anahtarları Windows DPAPI ile şifrelenir; config, runtime ve log dosyaları `secfs` ile yalnızca sahip + `SYSTEM` + `Administrators` erişimine kısıtlanır.
- **Gerekçe:** Ayrı Windows Servisi karmaşıklığına girmeden çok kullanıcılı Windows makinelerde yerel IPC/LPE ve özel anahtar okuma risklerini kapatır.

## ADR-007: Opsiyonel Windows Firewall/WFP Fail-Closed Kill Switch + Çift Watchdog
- **Karar:** Yönetici (`Oyun TUN`) modunda isteğe bağlı olarak `OzeVPN-KS-*` allowlist + `blockoutbound` (WFP tabanlı Windows Güvenlik Duvarı) kilidi kurulur; `watchdogLoop` hem `sing-box` hem `tor` süreçlerini üstel backoff ile izler ve `WM_POWERBROADCAST` uyku/uyanma olaylarını yakalar.
- **Gerekçe:** `CGO=0` tek-ikili sadeliğini koruyarak tünel kopması, çökme ve Hot-Reload anlarında fail-closed koruma ve tek komutluk acil kurtarma sağlar.

## ADR-008: Yerleşik Saf-Go Cloudflare WARP WireGuard (`internal/warp/wg_native.go`)
- **Karar:** Harici `warp-cli` zorunluluğu ve SOCKS5 UDP eksikliğini (Roblox Error 279 / Discord Voice UDP) aşmak için Go `crypto/ecdh` (X25519) ile doğrudan Cloudflare WARP cihaz kaydı yapılır, UDP uç noktaları gerçek `sing-box` tünelleme ile doğrulanır ve `MTU 1280` + `25s` keepalive ile bağlanır.
- **Gerekçe:** Harici istemci kurmadan tam Layer-3 TCP+UDP oyun tüneli sağlar; UDP engellenen ağlarda `warp-cli` veya Tor yedeğine düşer.
