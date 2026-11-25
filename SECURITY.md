# Güvenlik Politikası, Tehdit Modeli ve Acil Kurtarma Runbook'u

## 1. Tehdit Modeli ve Savunma Katmanları
OzeVPN yalnızca yerel cihazda (`127.0.0.1:7777`) çalışan, bulut sunucusu ve telemetrisi olmayan bir masaüstü kontrol panelidir.

| Tehdit | Uygulanan Savunma |
| :--- | :--- |
| **Harici web sitesinden CSRF** | `Origin` ve `Sec-Fetch-Site` doğrulaması (`403 Forbidden`) + `slog` güvenlik kaydı. |
| **DNS Rebinding** | `r.Host` yalnızca `127.0.0.1` / `localhost` / `::1` kabul edilir (`403`). |
| **DOM XSS** | Arayüzde `innerHTML` yerine `textContent`/`createElement` + sıkı `CSP` (`default-src 'self'`). |
| **Yerel SSRF (`/api/rescue`)** | Loopback, RFC1918, link-local (`169.254.0.0/16`) ve `.local`/`.internal` hedefleri reddedilir. |
| **Özel Sunucu Anahtar Sızıntısı** | `ozevpn.json` `0600` izniyle yazılır; `GET/POST /api/config` yanıtlarında `custom_server` maskelenir (`custom_server_set: true`). |
| **DNS Zehirleme & Takipçiler** | `sing-box` içi şifreli DoH (`1.1.1.1`) + `hijack-dns` + 28 ağır reklam/izleyici ağı engeli. |
| **İkili Bütünlük** | `BINARY_CHECKSUMS.txt` ile `sing-box.exe` ve `tor.exe` SHA256 doğrulaması. |

## 2. Acil Kurtarma Runbook'u (İnternet / Proxy Takılı Kalırsa)
1. OzeVPN panelinden **🧯 Proxy Sıfırla** butonuna tıkla (veya uygulamayı yeniden aç; `CleanupStaleOnBoot` otomatik temizler).
2. Uygulama hiç açılmıyorsa Windows PowerShell'de şu tek komutu çalıştır:
   ```powershell
   reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings" /v ProxyEnable /t REG_DWORD /d 0 /f
   ```
3. Yerleşik başlangıcı manuel kaldırmak için:
   ```powershell
   reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v OzeVPN /f
   ```

## 3. İkili Güncelleme Prosedürü (`sing-box` & `tor`)
1. Yeni resmi sürümü yalnızca `SagerNet/sing-box` veya `dist.torproject.org` üzerinden indir.
2. `Get-FileHash .\bin\sing-box.exe -Algorithm SHA256` ile yeni özeti alıp `BINARY_CHECKSUMS.txt` ve `THIRD_PARTY_LICENSES.md` dosyalarını güncelle.
3. `go test -v ./...` (`sing-box check` şema testi dahil) ve `.\build-portable.ps1` çalıştırarak doğrula.

## 4. Güvenlik Bildirimi
Güvenlik açığı tespit ederseniz doğrudan proje sahibine özel mesaj/issue üzerinden tekrar adımlarıyla bildirebilirsiniz; kritik proxy/API açıkları öncelikli olarak yamanır.
