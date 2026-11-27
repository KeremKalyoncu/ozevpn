# Değişiklik Günlüğü (Changelog)

Bu proje [Keep a Changelog](https://keepachangelog.com/tr/1.1.0/) ve [Semantic Versioning](https://semver.org/) ilkelerini izler.

## [Unreleased]

### Added
- **Marka & Vitrin:** Koyu/açık tema duyarlı README Hero bannerları (`assets/banner-dark.svg`, `assets/banner-light.svg`), `assets/logo.svg` ve `1280×640` sosyal önizleme kartı (`assets/social-preview.png`).
- **Tedarik Zinciri & Güvenlik Otomasyonu:** Tam 40-karakter SHA-pinli GitHub Actions, `staticcheck`, canlı Windows `ozevpn.exe` HTTP/güvenlik duman testi, `CodeQL` SAST, Linux Foundation **OpenSSF Scorecard**, Sigstore **SLSA Build Provenance** (`release.yml`) ve `Dependabot`.
- **Açık Kaynak Standartları:** Resmi `GPL-3.0` `LICENSE`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `.gitattributes` ve GitHub Issue form şablonları.

### Changed
- **Standart Go Proje Düzeni:** `cmd/ozevpn`, `internal/server`, `assets/embed.go`, `web/embed.go` ve `scripts/build-single-exe.ps1` yapısına geçildi; çift ikon kopyaları tek kaynağa (`assets/`) indirildi.
- **Clean Code Refaktörü:** `buildHandlerWithAuth`, `Manager.Start` ve `BuildSingBoxConfig` davranış korunarak modüler parçalara ayrıldı (tepe cyclomatic complexity `76 → 26`).
- **CI Biçim Kapısı:** `gofmt -l .` ve `staticcheck ./...` (sıfır uyarı) zorunlu CI kapısına eklendi.

## [6.1.0] - 2026-11-27

### Added
- **Yerleşik Saf-Go Cloudflare WARP WireGuard (`internal/warp/wg_native.go`):** Harici `warp-cli` gerektirmeden tam Layer-3 TCP+UDP tüneli (`MTU 1280`, `25s` keepalive, DPAPI + NTFS DACL anahtar saklama); Roblox Error 279 ve Discord Voice UDP sorunu çözüldü.
- **Signal Cockpit Arayüzü (`docs/DESIGN.md`):** Canlı Tünel Rota Şeması (`Cihaz → Katman → Tünel → Çıkış`), bağlanınca otomatik `/api/trace` çıkış konumu tespiti, inline SVG ikon seti, erişilebilir CSS switch'ler, 3'lü `aria-live` bildirim yığını ve `<= 120 KB` toplam web bütçesi.
- **Güvenlik Sertleştirme:** 256-bit `X-OzeVPN-Token` + `HttpOnly; SameSite=Strict`, `Host`/`Origin`/`Sec-Fetch-Site` CSRF koruması, Windows Firewall fail-closed Kill Switch, dual-stack IPv6 TUN, şifreli DoH detour, 5/5 SHA-256 doğrulamalı tek-dosya EXE ve CycloneDX 1.5 SBOM.
