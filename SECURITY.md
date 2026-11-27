# Güvenlik Politikası, Tehdit Modeli ve Acil Kurtarma Runbook'u

## 1. Tehdit Modeli ve Savunma Katmanları
OzeVPN yalnızca yerel cihazda (`127.0.0.1:7777`) çalışan, bulut sunucusu ve telemetrisi olmayan bir masaüstü kontrol panelidir. Detaylı STRIDE analizi için bkz. [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) ve [PRIVACY.md](PRIVACY.md).

| Tehdit | Uygulanan Savunma |
| :--- | :--- |
| **Yerel Yetkisiz IPC / Admin LPE** | 256-bit `crypto/rand` oturum tokeni (`/auth` → `HttpOnly; SameSite=Strict` + tüm mutasyonlarda zorunlu `X-OzeVPN-Token` + `subtle.ConstantTimeCompare`) + `withRateLimit`. |
| **Harici Web Sitesinden CSRF / DNS Rebinding** | `r.Host` yalnız loopback (`127.0.0.1`/`localhost`/`::1`), `Origin` & `Sec-Fetch-Site` doğrulaması (`403`) + `slog` güvenlik kaydı. |
| **Özel Sunucu Anahtar Sızıntısı** | Windows **DPAPI** (`CryptProtectData`) şifrelemesi + NTFS DACL (yalnız mevcut kullanıcı + `SYSTEM` + `Administrators`), API yanıtlarında maskeleme (`custom_server_set: true`), hata loglarında `RedactSecrets` ve kapatmada `sing-box-active.json` silme. |
| **Protokol Downgrade & Zayıf TLS** | Plaintext `vless://`, uzak düz metin `socks5://`, `allowInsecure=1` ve ham JSON `tls.insecure:true` reddedilir; WireGuard Post-Quantum `PresharedKey` desteklenir. |
| **Sızıntı Koruması** | Dual-stack IPv4+IPv6 TUN (`fdfe:dcba:9876::1/126`, WireGuard ile `MTU 1280` / diğerleri `MTU 1400`), DoH tünel `detour`, şifreli `dns-direct`, `ipconfig /flushdns`, UI WebRTC non-proxied UDP engeli. |
| **Tünel Kopması / Çökme Sızıntısı** | Yönetici (`Oyun TUN`) modunda Windows Firewall fail-closed **Kill Switch** (`OzeVPN-KS-*` + `blockoutbound`), çift `sing-box`+`tor` Watchdog ve `WM_POWERBROADCAST` uyanış kurtarması. |
| **Yerel SSRF (`/api/rescue`) & DOM XSS** | Loopback/RFC1918/link-local/`.local`/`.internal` engeli; DOM'da `textContent`/`createElement` + sıkı `CSP` (`default-src 'self'`). |
| **Tedarik Zinciri & İkili Bütünlük** | `BINARY_CHECKSUMS.txt` (5 dosya: `sing-box.exe`, `tor.exe`, `lyrebird.exe`, `geoip`, `geoip6`), CI `govulncheck`, fuzzing, CycloneDX SBOM ve commit provenance (`-ldflags`). |

## 2. Acil Kurtarma Runbook'u (İnternet / Proxy / Kill Switch Takılı Kalırsa)
1. OzeVPN panelinden **Araçlar → Proxy Sıfırla** seçeneğine tıkla (veya uygulamayı yeniden aç; `CleanupStaleOnBoot` otomatik temizler).
2. Uygulama hiç açılmıyorsa **Yönetici PowerShell**'de şu komutları çalıştır:
   ```powershell
   reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f
   netsh advfirewall set allprofiles firewallpolicy blockinbound,allowoutbound
   Get-NetFirewallRule -DisplayName 'OzeVPN-KS-*' -ErrorAction SilentlyContinue | Remove-NetFirewallRule
   ipconfig /flushdns
   ```
3. Tam kaldırma için: `OzeVPN.exe -uninstall` (veya manuel: `reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v OzeVPN /f`).

## 3. İkili Güncelleme & Rollback Koruma Prosedürü
1. Yeni resmi sürümleri yalnızca `SagerNet/sing-box` veya `dist.torproject.org` üzerinden GPG/SHA256 doğrulayarak indir.
2. Tüm 5 dosya (`sing-box.exe`, `tor.exe`, `lyrebird.exe`, `geoip`, `geoip6`) için `Get-FileHash -Algorithm SHA256` özetlerini `BINARY_CHECKSUMS.txt` ve `THIRD_PARTY_LICENSES.md` dosyalarına işle.
3. Bilinen kritik zafiyet içeren eski sürümlere geri dönüş (rollback) yasaktır; her yayın öncesi `go vet ./...`, `go test -race ./...`, `govulncheck ./...` ve `.\build-single-exe.ps1` zorunlu yayın kapısıdır.
4. Dağıtılan `dist\OzeVPN.exe` dosyası `dist\OzeVPN.exe.sha256` ve `dist\sbom.cdx.json` ile yayımlanır (sertifika mevcutsa `$env:OZEVPN_SIGN_THUMBPRINT` ile Authenticode imzalanır).

## 4. Güvenlik Bildirimi
Güvenlik açığı tespit ederseniz proje sahibine tekrar adımlarıyla bildirebilirsiniz; P0/P1 proxy, sızıntı, Kill Switch ve IPC açıkları öncelikli yamanır.
