# OzeVPN v6 — Hızlı, Güvenli & Sansürsüz İnternet Motoru

**OzeVPN v6**, `sing-box 1.14+` çekirdeği üzerine kurulu; **paralel hibrit motor (WARP + Avrupa Tor)**, **dual-stack IPv4+IPv6 TUN**, **tünel içi şifreli DoH DNS**, **Windows Firewall Fail-Closed Kill Switch**, **256-bit oturum tokenli & DPAPI/NTFS DACL korumalı** sıfır bağımlılıklı yerel masaüstü VPN kontrol panelidir.

## Hemen Çalıştır
- **[dist/OzeVPN.exe](dist/OzeVPN.exe)** (veya geliştirme ortamında `go run .`) dosyasını çalıştır — uygulama 256-bit oturum anahtarıyla yerel paneli (`http://127.0.0.1:7777`) otomatik açar ve Sistem Tepsisine (System Tray) yerleşir.
- **Standart Mod vs Yönetici (`🎮 Oyun TUN`) Modu:**
  - **Standart Mod (Proxy):** UAC istemez; Windows WinINET proxy kullanan tarayıcı ve uygulamaları korur (UDP oyunlar ve proxy-dışı uygulamalar kapsam dışıdır).
  - **Yönetici Modu (`🎮 Oyun TUN`):** Çift yığınlı (`IPv4 + IPv6`, `MTU 1400`) sanal TUN ağ kartı (`ozevpn0`) açar, tüm sistem/UDP oyun trafiğini yakalar ve isteğe bağlı **🔒 Kill Switch** (Windows Firewall fail-closed) korumasını destekler.
- **Temiz Kaldırma:** `OzeVPN.exe -uninstall` komutu başlangıç kaydını, kısayolu, proxy/Kill Switch kurallarını ve yerel verileri tamamen temizler.

## Güvenlik & Teknik Özellikler
1. **🔒 Yerel IPC & Sır Koruması (`PR-VPN1`):** 256-bit `X-OzeVPN-Token` + `HttpOnly; SameSite=Strict` oturum doğrulaması, `Host`/`Origin`/`Sec-Fetch-Site` koruması, `custom_server` için Windows **DPAPI** şifrelemesi, NTFS DACL kısıtlaması (`secfs`) ve başlatma öncesi `sing-box check` doğrulaması.
2. **🛡️ Sızıntı Koruması (`PR-VPN2`):** Dual-stack IPv4+IPv6 TUN (`fdfe:dcba:9876::1/126`), aktif tünel `detour`'lu DoH DNS (`1.1.1.1`), bağlantı geçişlerinde `ipconfig /flushdns` ve UI penceresinde WebRTC non-proxied UDP engeli.
3. **⚡ Fail-Closed Kill Switch & Çift Watchdog (`PR-VPN3`):** Yönetici TUN modunda `OzeVPN-KS-*` + `blockoutbound` Windows Güvenlik Duvarı kilidi, `sing-box` + `tor` üstel backoff Watchdog'u ve `WM_POWERBROADCAST` uyku/uyanma kurtarması.
4. **🔐 Protokol Sertleştirme (`PR-VPN4`):** Şifresiz `vless://`, uzak düz metin `socks5://` ve `tls.insecure` reddedilir; WireGuard Post-Quantum `PresharedKey` desteklenir; hata loglarında özel anahtarlar maskelenir (`RedactSecrets`).
5. **📡 Sertleştirilmiş OpenWrt Router & VPS (`PR-VPN5`):** [router/openwrt-setup.sh](router/openwrt-setup.sh) ve [vps/setup-vps.sh](vps/setup-vps.sh) (UFW default-deny, SSH sertleştirme, Cloud Metadata `169.254.0.0/16` engeli, Post-Quantum PSK, `chrony`, `unattended-upgrades`, `chmod 600`).
6. **📦 Tedarik Zinciri & Belgeler (`PR-VPN6`):** 5 dosyalı [BINARY_CHECKSUMS.txt](BINARY_CHECKSUMS.txt), CycloneDX SBOM, [SECURITY.md](SECURITY.md), [PRIVACY.md](PRIVACY.md), [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md), [docs/OPERATIONS.md](docs/OPERATIONS.md), [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Test, Benchmark & Tek-EXE Derleme
```powershell
go vet ./...
go test -race -cover ./...
go run ./tools/bench
.\build-single-exe.ps1
```
