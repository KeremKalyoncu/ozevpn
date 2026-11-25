# OzeVPN — Mimari Karar Kaydı (ADR)

## ADR-001: Tek Binary Go Monoliti + Gömülü Vanilla Web UI
- **Karar:** Electron/Chromium yerine Go standart kütüphanesi + `//go:embed web/*` kullanıldı.
- **Gerekçe:** ~12–15 MB RAM kullanımı, <200 ms başlangıç süresi, 0 üçüncü parti Go/npm bağımlılığı (sıfır tedarik zinciri şişkinliği).

## ADR-002: `sing-box` Hibrit Yönlendirme (WARP + Avrupa Tor + Direkt TR/4K Bypass)
- **Karar:** Tüm trafik yerel `sing-box` çekirdeğinden geçer; TR resmi (`.gov.tr`/`.edu.tr`), bankalar ve engelsiz 4K video/indirme CDN'leri (`googlevideo.com`, `steamcontent.com` vb.) `direct` çıkar; TR çıkışında takılan servisler (`roblox.com`, `wattpad.com`, özel kullanıcı listesi) `tor-eu` (`DE/NL/BG/RO/AT`) üzerinden; geri kalan dünya düşük pingli WARP veya özel VLESS/WireGuard sunucusundan çıkar.
- **Gerekçe:** Canlı ölçümlerde ücretsiz WARP İstanbul (`IST`) düğümü Discord/Pastebin'i açarken Roblox'u açamadı; hibrit yönlendirme hem hızı koruyor hem tüm engelli siteleri tek tıkla açıyor.

## ADR-003: Akıllı TUN (Yönetici) + Otomatik Sistem Proxy Yedeği
- **Karar:** Yönetici izni varsa `sing-box` TUN sanal ağ kartı (`ozevpn0`) açılır (oyun UDP trafiği dahil); standart kullanıcıda UAC sormadan HKCU Windows Sistem Proxy (`127.0.0.1:10808`) kullanılır.
- **Gerekçe:** En az yetki ilkesi (C047) ve sıfır kurulum sürtünmesi.

## ADR-004: Tek Kaynaklı `BuildSingBoxConfig` + Kesintisiz Hot-Reload
- **Karar:** `Start()` ve `ReloadRouting()` aynı `BuildSingBoxConfig()` fonksiyonunu kullanır ve `engine_test.go` içinde resmi `sing-box check` ile doğrulanır.
- **Gerekçe:** Yapılandırma sapmasını (drift) önler ve kural değişikliklerini ~320 ms'de uygular.
