#!/bin/sh
# OzeVPN Faz 2 — OpenWrt Router Kurulum Betiği
# Modem ile ev ağı arasına giren OpenWrt router'a:
#  1) sing-box (akıllı split-tunnel: sadece seçili siteler/oyunlar tünelden geçer)
#  2) SQM / CAKE (bufferbloat önleyici — indirme yapılırken bile oyun pingi fırlamaz)
#  3) AdGuard Home veya şifreli DNS desteği için temel paketler
# kurar.

set -e

echo "=== OzeVPN OpenWrt Kurulumu Başlıyor ==="

if [ "$(id -u)" -ne 0 ]; then
  echo "Lütfen root olarak çalıştırın."
  exit 1
fi

echo "[1/4] Paket listesi güncelleniyor..."
opkg update

echo "[2/4] sing-box, TUN sürücüsü ve SQM (CAKE) kuruluyor..."
opkg install kmod-tun kmod-nft-tproxy sing-box luci-app-sqm sqm-scripts ca-bundle curl

echo "[3/4] sing-box yapılandırma klasörü hazırlanıyor..."
mkdir -p /etc/sing-box

if [ ! -f /etc/sing-box/config.json ]; then
  echo "UYARI: /etc/sing-box/config.json bulunamadı!"
  echo "PC'deki OzeVPN panelinden 'Modem / Router (Faz 2)' sekmesine gidip"
  echo "'sing-box-openwrt.json' dosyasını indir ve şu komutla router'a kopyala:"
  echo "  scp sing-box-openwrt.json root@192.168.1.1:/etc/sing-box/config.json"
fi

echo "[4/4] sing-box servisini etkinleştirme..."
uci set sing-box.main.enabled='1'
uci set sing-box.main.conffile='/etc/sing-box/config.json'
uci commit sing-box

echo ""
echo "=== Kurulum Tamamlandı ==="
echo "Sonraki adımlar:"
echo "  1. /etc/sing-box/config.json içine WireGuard/WARP (wgcf) veya VPS anahtarını gir."
echo "  2. '/etc/init.d/sing-box restart' komutuyla başlat."
echo "  3. LuCI (http://192.168.1.1) -> Network -> SQM QoS sekmesinden:"
echo "     - Interface: WAN"
echo "     - Download/Upload: İnternet hızının %85-90'ı (örn. 100 Mbps için 90000 kbit/s)"
echo "     - Queue Discipline: cake + piece_of_cake.qos"
echo "     Bu ayar, evde biri indirme yaparken bile pinginin sabit kalmasını sağlar!"
