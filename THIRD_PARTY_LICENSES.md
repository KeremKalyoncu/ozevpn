# Üçüncü Parti Bileşenler ve Lisans Bildirimi (SBOM)

OzeVPN Go kodu sıfır üçüncü parti Go modülü kullanır (`go.mod` yalnızca Go standart kütüphanesi).
Taşınabilir dağıtımda (`dist/OzeVPN-v6-Portable.zip`) aşağıdaki açık kaynaklı çalışma zamanı bileşenleri paketlenir:

| Bileşen | Sürüm | Kaynak | Lisans |
| :--- | :--- | :--- | :--- |
| **sing-box** (`bin/sing-box.exe`) | `v1.14.4` | https://github.com/SagerNet/sing-box | GNU General Public License v3.0 (GPL-3.0) |
| **Tor Expert Bundle** (`bin/tor/tor/tor.exe`, `lyrebird.exe`, GeoIP) | `16.0.3` | https://dist.torproject.org/torbrowser/ | 3-Clause BSD License |
| **Cloudflare WARP CLI** *(isteğe bağlı, sistemde kuruluysa)* | Sistem sürümü | https://developers.cloudflare.com/warp-client/ | Cloudflare Terms (harici sistem aracı, paketlenmez) |
