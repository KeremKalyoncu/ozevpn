# OzeVPN — Mimari Karar Kaydı (ADR)

## ADR-001: Tek Binary Go Monoliti + Gömülü Vanilla Web UI
- **Karar:** Electron/Chromium yerine Go standart kütüphanesi + `//go:embed web/*` kullanıldı.
- **Gerekçe:** ~12–15 MB RAM kullanımı, <200 ms başlangıç süresi, 0 üçüncü parti Go/npm bağımlılığı (sıfır tedarik zinciri şişkinliği).

## ADR-002: `sing-box` 1.14+ Hibrit Yönlendirme (WARP + Avrupa Tor + Direkt TR/4K Bypass)
- **Karar:** Tüm trafik yerel `sing-box 1.14+` çekirdeğinden geçer; TR resmi (`.gov.tr`/`.edu.tr`), bankalar ve engelsiz 4K video/indirme CDN'leri `direct` çıkar; TR çıkışında takılan servisler (`roblox.com`, `wattpad.com`, özel kullanıcı listesi) `tor-eu` (`DE/NL/BG/RO/AT`) üzerinden; geri kalan dünya düşük pingli WARP veya özel VLESS/WireGuard (`endpoints`) sunucusundan çıkar.
- **Gerekçe:** Canlı ölçümlerde ücretsiz WARP İstanbul (`IST`) düğümü Discord/Pastebin'i açarken Roblox'u açamadı; hibrit yönlendirme hem hızı koruyor hem tüm engelli siteleri tek tıkla açıyor.

## ADR-003: Akıllı TUN (Yönetici) + Otomatik Sistem Proxy Yedeği
- **Karar:** Yönetici izni varsa `sing-box` çift yığınlı (`IPv4+IPv6`, `MTU 1400`) TUN sanal ağ kartı (`ozevpn0`) açılır (oyun UDP trafiği dahil); standart kullanıcıda UAC sormadan HKCU Windows Sistem Proxy (`127.0.0.1:10808`) kullanılır ve kapsam farkı UI'da açıkça etiketlenir.
- **Gerekçe:** En az yetki ilkesi ve sıfır kurulum sürtünmesi.

## ADR-004: Tek Kaynaklı `BuildSingBoxConfig` + Ön Doğrulamalı Hot-Reload
- **Karar:** `Start()` ve `ReloadRouting()` aynı `BuildSingBoxConfig()` fonksiyonunu kullanır, başlatmadan önce resmi `sing-box check` ile doğrular ve `engine_test.go` içinde sürekli test edilir.
- **Gerekçe:** Yapılandırma sapmasını (drift) ve bozuk config başlatmayı önler.

## ADR-005: Dual-Stack IPv6 TUN + DoH Tünel Detour + OS DNS Önbellek Temizliği (`PR-VPN2`)
- **Karar:** `tun-in` içine IPv6 CIDR (`fdfe:dcba:9876::1/126`) + `mtu: 1400` eklendi; `dns-doh` aktif tünel outbound'una `detour` edildi; `dns-direct` düz UDP yerine şifreli HTTPS (`1.1.1.1`) yapıldı; bağlantı geçişlerinde `ipconfig /flushdns` çalıştırılır ve UI penceresinde `--force-webrtc-ip-handling-policy=disable_non_proxied_udp` uygulanır.
- **Gerekçe:** Çift yığınlı Windows hostlarda native IPv6 sızıntısını ve ISP DNS/WebRTC sızıntılarını kapatır.

## ADR-006: 256-Bit Oturum IPC Tokeni + Windows DPAPI + NTFS DACL (`PR-VPN1`)
- **Karar:** `127.0.0.1:7777` kontrol API'si her başlangıçta üretilen 256-bit `crypto/rand` tokeni (`HttpOnly; SameSite=Strict` + `X-OzeVPN-Token`) ile korunur (`-takeover` geçişinde DACL-korumalı `.ozevpn_token` ile devralınır). `custom_server` sırları Windows DPAPI ile şifrelenir; config, runtime ve log dosyaları `secfs` ile yalnızca sahip + `SYSTEM` + `Administrators` erişimine kısıtlanır.
- **Gerekçe:** Ayrı Windows Servisi overengineering'ine gitmeden çok kullanıcılı Windows makinelerde yerel IPC/LPE ve özel anahtar okuma risklerini kapatır.

## ADR-007: Opsiyonel Windows Firewall/WFP Fail-Closed Kill Switch + Çift Watchdog (`PR-VPN3`)
- **Karar:** Yönetici (`Oyun TUN`) modunda isteğe bağlı olarak `OzeVPN-KS-*` allowlist + `blockoutbound` (WFP tabanlı Windows Güvenlik Duvarı) kilidi kurulur; `watchdogLoop` hem `sing-box` hem `tor` süreçlerini üstel backoff ile izler ve `WM_POWERBROADCAST` uyku/uyanma olaylarını yakalar.
- **Gerekçe:** `CGO=0` tek-ikili sadeliğini koruyarak tünel kopması, çökme ve Hot-Reload anlarında fail-closed koruma ve tek komutluk acil kurtarma sağlar.
