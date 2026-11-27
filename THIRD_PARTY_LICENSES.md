# Üçüncü Parti Bileşenler ve Lisans Bildirimi (SBOM)

OzeVPN Go kaynak kodu **sıfır üçüncü parti Go modülü** kullanır (`go.mod` yalnızca Go standart kütüphanesi).
Tek-dosya masaüstü derlemesi (`scripts/build-single-exe.ps1`) tüm gömülü ikilileri `BINARY_CHECKSUMS.txt` üzerinden SHA256 ile doğrular ve makine-okunur CycloneDX 1.5 SBOM (`dist/sbom.cdx.json`) üretir.

| Bileşen | Sürüm | Kaynak | Lisans | Bütünlük |
| :--- | :--- | :--- | :--- | :--- |
| **sing-box** (`bin/sing-box.exe`) | `v1.14.4` (`go1.26.10`) | https://github.com/SagerNet/sing-box | GNU General Public License v3.0 (GPL-3.0) | `BINARY_CHECKSUMS.txt` (SHA256) |
| **Tor** (`bin/tor/tor/tor.exe`) | `0.4.9.17` (`OpenSSL 3.5.9`) | https://dist.torproject.org/torbrowser/ | 3-Clause BSD License | `BINARY_CHECKSUMS.txt` (SHA256) |
| **lyrebird (Snowflake PT)** (`bin/tor/tor/pluggable_transports/lyrebird.exe`) | Tor Bundle `16.0.3` | https://dist.torproject.org/torbrowser/ | 3-Clause BSD License | `BINARY_CHECKSUMS.txt` (SHA256) |
| **Tor GeoIP / GeoIPv6** (`bin/tor/data/geoip*`) | Tor Bundle `16.0.3` | https://dist.torproject.org/torbrowser/ | 3-Clause BSD / CC | `BINARY_CHECKSUMS.txt` (SHA256) |
| **Cloudflare WARP CLI** *(isteğe bağlı sistem aracı)* | Sistem kurulu sürüm | https://developers.cloudflare.com/warp-client/ | Cloudflare Terms (paketlenmez) | Sistem kurulumu |
