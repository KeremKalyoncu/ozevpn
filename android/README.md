# OzeVPN Android (Anadolu Vaşağı)

Root gerektirmeyen, tek dokunuşla **Discord (sesli UDP dahil)**, **Roblox** ve tüm internete şifreli erişim sağlayan minimalist Android istemcisi.

## Özellikler (Sıfır Şişkinlik)
- **Tek Dokunuş Anadolu Vaşağı:** Ortadaki Vaşak düğmesine dokun → 8 resmi Cloudflare WARP uç noktasını yarıştırır, en düşük pingli noktaya **WireGuard (TCP+UDP)** ile bağlanır.
- **Root'suz Tam TUN (`VpnService`):** Resmi `sing-box 1.14.6` + `hev-socks5-tunnel 2.20.0` (SHA-256 doğrulamalı); kendi paketini hariç tutarak sıfır yönlendirme döngüsüyle tüm uygulamalara tam UDP+TCP tüneli sağlar.
- **2 Temiz Mod:** `Tam Koruma` (her şey şifreli tünelden) veya `Hibrit TR` (e-Devlet ve TR bankaları direkt normal hattan).
- **Reklam & Takipçi Kalkanı:** 28 ağır reklam/izleyici alan adını DNS katmanında engeller.
- **Hızlı Ayarlar (Quick Settings) Karosu & Canlı Erişim Testi:** Üst bildirim panelinden tek dokunuşla aç/kapa ve uygulama içi canlı Discord/Roblox/Google/Cloudflare doğrulaması.

## Kurulum (Telefon)
- Modern Android telefonların %99'u için **`OzeVPN-Android-arm64.apk`** (~27.8 MB) yeterlidir (`Android 10+` / API 29+).

## Kaynaktan Derleme
```powershell
# 1. Resmi upstream sing-box + hev-socks5-tunnel çekirdeklerini SHA-256 doğrulayarak indir
.\android\scripts\fetch-cores.ps1

# 2. Test et ve Release APK üret (JDK 21 + Android SDK)
cd android
.\gradlew.bat testDebugUnitTest assembleRelease
```
