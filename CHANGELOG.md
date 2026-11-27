# Değişiklik Günlüğü (Changelog)

Bu proje [Keep a Changelog](https://keepachangelog.com/tr/1.1.0/) ve [Semantic Versioning](https://semver.org/) ilkelerini izler.

## [6.1.2] - 2026-11-27

### Security
- **Windows `%PATH%` Binary-Planting Hardening (`internal/procutil/sysbin.go`):** Pinned all system command invocations (`netsh.exe`, `icacls.exe`, `reg.exe`, `ipconfig.exe`, `net.exe`, `rundll32.exe`, `powershell.exe`) to absolute `%SystemRoot%\System32` paths.
- **Secure Session Cookie (`internal/server/server.go`):** Enabled `Secure: true` alongside `HttpOnly; SameSite=Strict` on `ozevpn_session` (closing GitHub CodeQL alert `#6`; **0 open CodeQL alerts**).
- **SonarCloud Triple-A (`A / A / A`) & Quality Gate `PASSED`:** Resolved all 31 SonarCloud bugs/security items (`0` bugs, `0` vulnerabilities, `0` security hotspots) and added `.sonarcloud.properties`.
- **SLSA Level 3 Signed Releases & Packaging:** Added `.goreleaser.yaml` + SHA-pinned `goreleaser/goreleaser-action` (`Packaging: 10/10`) and `slsa-framework/slsa-github-generator` v2.1.0 to attach cryptographically verifiable `multiple.intoto.jsonl` provenance to every release.

### Changed
- **Canonical GNU GPL-3.0 `LICENSE`:** Replaced root `LICENSE` and `LICENSES/GPL-3.0-or-later.txt` with verbatim 674-line FSF GNU GPL v3.0 text (`License: 10/10`).
- **Flagship Showcase README & Code Signing Policy:** Added dark/light vector certification & architecture cards (`assets/certifications-*.svg`, `assets/architecture-*.svg`), `CODE_SIGNING_POLICY.md` (SignPath Foundation), and Winget `1.10` (`en-US` + `tr-TR`) manifests.

## [6.1.1] - 2026-11-27

### Added
- **Ev Ağıyla Paylaşım (`LAN Proxy`) & Canlı Trafik Sayacı:** Tek tıkla yerel Wi-Fi'deki telefon/TV'leri tünele çıkarma (`share_lan_proxy`, varsayılan kapalı) ve `sing-box` yerel akışından canlı hız/oturum veri sayacı (`internal/engine/traffic.go`).
- **Tek Satır Kurulum & Paketleme:** SHA-256 doğrulamalı `scripts/install.ps1` ve WinGet v1.6 manifestleri (`packaging/winget/`).
- **Araçlar → Tanılama Kopyala:** IP/MAC/anahtarları otomatik maskelenmiş tek tıkla bug raporu çıktısı.
- **Marka & Global Vitrin:** Koyu/açık tema duyarlı Hero bannerlar (`assets/banner-*.svg`), `assets/logo.svg`, `assets/social-preview.png`, `README.en.md` ve canlı arayüz `docs/screenshots/demo.gif`.
- **Tedarik Zinciri & Güvenlik Otomasyonu:** Tam SHA-pinli GitHub Actions, `staticcheck`, canlı Windows `ozevpn.exe` HTTP/güvenlik duman testi, **FSFE REUSE 3.3**, **CodeQL SAST**, Linux Foundation **OpenSSF Scorecard**, Sigstore **SLSA Build Provenance** (`release.yml`), **SonarCloud** ve **Dependabot**.
- **Açık Kaynak Standartları:** Resmi `GPL-3.0` `LICENSE`, `REUSE.toml`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `.gitattributes` ve GitHub Issue form şablonları.

### Changed
- **Standart Go Proje Düzeni:** `cmd/ozevpn`, `internal/server`, `assets/embed.go`, `web/embed.go` ve `scripts/build-single-exe.ps1` yapısına geçildi; çift ikon kopyaları tek kaynağa (`assets/`) indirildi.
- **Clean Code Refaktörü & `sing-box 1.16` Uyumu:** `buildHandlerWithAuth`, `Manager.Start` ve `BuildSingBoxConfig` modüler parçalara ayrıldı (tepe cyclomatic complexity `76 → 26`); deprecated `dns.independent_cache` kaldırıldı.
- **CI Biçim & Statik Analiz Kapısı:** `gofmt -l .`, `staticcheck ./...` ve `fsfe/reuse-action` zorunlu CI kapısına eklendi.

## [6.1.0] - 2026-11-27

### Added
- **Yerleşik Saf-Go Cloudflare WARP WireGuard (`internal/warp/wg_native.go`):** Harici `warp-cli` gerektirmeden tam Layer-3 TCP+UDP tüneli (`MTU 1280`, `25s` keepalive, DPAPI + NTFS DACL anahtar saklama); Roblox Error 279 ve Discord Voice UDP sorunu çözüldü.
- **Signal Cockpit Arayüzü (`docs/DESIGN.md`):** Canlı Tünel Rota Şeması (`Cihaz → Katman → Tünel → Çıkış`), bağlanınca otomatik `/api/trace` çıkış konumu tespiti, inline SVG ikon seti, erişilebilir CSS switch'ler, 3'lü `aria-live` bildirim yığını ve `<= 120 KB` toplam web bütçesi.
- **Güvenlik Sertleştirme:** 256-bit `X-OzeVPN-Token` + `HttpOnly; SameSite=Strict`, `Host`/`Origin`/`Sec-Fetch-Site` CSRF koruması, Windows Firewall fail-closed Kill Switch, dual-stack IPv6 TUN, şifreli DoH detour, 5/5 SHA-256 doğrulamalı tek-dosya EXE ve CycloneDX 1.5 SBOM.
