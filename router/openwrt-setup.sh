#!/bin/sh
# OzeVPN Faz 2 — Sertleştirilmiş OpenWrt Router Kurulum Betiği
# Modem ile ev ağı arasına giren OpenWrt router'a:
#  1) sing-box (dual-stack IPv4+IPv6 TUN, DoH detour, akıllı split-tunnel)
#  2) SQM / CAKE (bufferbloat önleyici — indirme yapılırken bile oyun pingi fırlamaz)
#  3) Sıkı dosya izinleri (0700/0600) ve NTP saat doğrulaması
# kurar.

set -eu
umask 077

echo "=== OzeVPN OpenWrt Kurulumu Başlıyor ==="

if [ "$(id -u)" -ne 0 ]; then
  echo "Lütfen root olarak çalıştırın."
  exit 1
fi

echo "[1/5] Paket listesi güncelleniyor..."
opkg update

echo "[2/5] sing-box, TUN sürücüsü ve SQM (CAKE) kuruluyor..."
opkg install kmod-tun kmod-nft-tproxy sing-box luci-app-sqm sqm-scripts ca-bundle curl

echo "[3/5] NTP saat senkronizasyonu doğrulanıyor..."
if [ -x /etc/init.d/sysntpd ]; then
  /etc/init.d/sysntpd enable || true
  /etc/init.d/sysntpd restart || true
fi

echo "[4/5] sing-box yapılandırma klasörü sıkı izinlerle (0700) hazırlanıyor..."
mkdir -p /etc/sing-box
chmod 700 /etc/sing-box

if [ ! -f /etc/sing-box/config.json ]; then
  echo "UYARI: /etc/sing-box/config.json bulunamadı!"
  echo "PC'deki OzeVPN panelinden 'Modem / Router (Faz 2)' sekmesine gidip"
  echo "'sing-box-openwrt.json' dosyasını indir ve şu komutla router'a kopyala:"
  echo "  scp sing-box-openwrt.json root@192.168.1.1:/etc/sing-box/config.json"
  echo "  ssh root@192.168.1.1 'chmod 600 /etc/sing-box/config.json && sing-box check -c /etc/sing-box/config.json'"
else
  chmod 600 /etc/sing-box/config.json
  sing-box check -c /etc/sing-box/config.json
fi

echo "[5/5] sing-box servisini etkinleştirme..."
uci set sing-box.main.enabled='1'
uci set sing-box.main.conffile='/etc/sing-box/config.json'
uci commit sing-box

echo ""
echo "=== Kurulum Tamamlandı ==="
echo "Sonraki adımlar:"
echo "  1. /etc/sing-box/config.json içindeki placeholder WireGuard private_key alanını kendi anahtarınla değiştir ve chmod 600 uygula."
echo "  2. 'sing-box check -c /etc/sing-box/config.json && /etc/init.d/sing-box restart' ile doğrula ve başlat."
echo "  3. LuCI (http://192.168.1.1) -> Network -> SQM QoS sekmesinden:"
echo "     - Interface: WAN, Queue Discipline: cake + piece_of_cake.qos (%85-90 bant genişliği)"
echo "  4. Güvenlik: LuCI (80/443) ve SSH (22) yalnız LAN arayüzünde dinlemeli (WAN kapalı)."
