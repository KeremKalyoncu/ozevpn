#!/usr/bin/env bash
# OzeVPN İsteğe Bağlı Kendi VPS Kurulum Betiği (Ubuntu / Debian)
# İleride aylık ~3-5€'ya Bulgaristan/Romanya/Almanya'da kendi sunucunu kiralarsan,
# bu betik tek komutla yüksek hızlı WireGuard sunucusu kurar.

set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Lütfen sudo/root ile çalıştırın."
  exit 1
fi

apt-get update -y
apt-get install -y wireguard qrencode curl iptables

# IP forwarding etkinleştir
cat <<EOF >/etc/sysctl.d/99-ozevpn.conf
net.ipv4.ip_forward=1
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
EOF
sysctl --system

mkdir -p /etc/wireguard
chmod 700 /etc/wireguard

SERVER_PRIV=$(wg genkey)
SERVER_PUB=$(echo "$SERVER_PRIV" | wg pubkey)
CLIENT_PRIV=$(wg genkey)
CLIENT_PUB=$(echo "$CLIENT_PRIV" | wg pubkey)

WAN_IF=$(ip route list default | awk '{print $5; exit}')
PUBLIC_IP=$(curl -s https://api.ipify.org)

cat <<EOF >/etc/wireguard/wg0.conf
[Interface]
Address = 10.66.66.1/24
ListenPort = 51820
PrivateKey = ${SERVER_PRIV}
PostUp = iptables -A FORWARD -i %i -j ACCEPT; iptables -t nat -A POSTROUTING -o ${WAN_IF} -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT; iptables -t nat -D POSTROUTING -o ${WAN_IF} -j MASQUERADE

[Peer]
PublicKey = ${CLIENT_PUB}
AllowedIPs = 10.66.66.2/32
EOF

chmod 600 /etc/wireguard/wg0.conf
systemctl enable --now wg-quick@wg0

cat <<EOF >/root/ozevpn-client.conf
[Interface]
PrivateKey = ${CLIENT_PRIV}
Address = 10.66.66.2/32
DNS = 1.1.1.1

[Peer]
PublicKey = ${SERVER_PUB}
Endpoint = ${PUBLIC_IP}:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
EOF

echo ""
echo "=== OzeVPN VPS (WireGuard + BBR) Hazır! ==="
echo "İstemci yapılandırması: /root/ozevpn-client.conf"
echo "Server Public Key: ${SERVER_PUB}"
echo "Client Private Key: ${CLIENT_PRIV}"
echo "Endpoint: ${PUBLIC_IP}:51820"
