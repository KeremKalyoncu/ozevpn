#!/usr/bin/env bash
# OzeVPN İsteğe Bağlı Kendi VPS Kurulum & Sertleştirme Betiği (Ubuntu / Debian)
# - WireGuard (Post-Quantum PresharedKey dahil) + TCP BBR
# - UFW varsayılan-red (default-deny) güvenlik duvarı (yalnız SSH + 51820/udp)
# - Cloud Metadata (169.254.169.254) ve iç ağ (RFC1918) peer izolasyonu
# - SSH sertleştirme (sshd -t doğrulamalı) + unattended-upgrades + NTP (chrony)
# - umask 077 & chmod 600 (özel anahtarlar terminale basılmaz)

set -euo pipefail
umask 077

if [ "$(id -u)" -ne 0 ]; then
  echo "Lütfen sudo/root ile çalıştırın."
  exit 1
fi

SSH_PORT="${SSH_PORT:-22}"
WG_PORT="${WG_PORT:-51820}"

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y wireguard qrencode curl iptables ufw unattended-upgrades chrony

# 1. Saat senkronizasyonu (WireGuard TAI64N & TLS geçerliliği için zorunlu)
systemctl enable --now chrony || true

# 2. Otomatik güvenlik yamaları (unattended-upgrades)
cat <<'EOF' >/etc/apt/apt.conf.d/20auto-upgrades
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
EOF
systemctl enable --now unattended-upgrades || true

# 3. Kernel ağ sertleştirmesi + IP forwarding + BBR
cat <<'EOF' >/etc/sysctl.d/99-ozevpn.conf
net.ipv4.ip_forward=1
net.core.default_qdisc=fq
net.ipv4.tcp_congestion_control=bbr
net.ipv4.conf.all.rp_filter=1
net.ipv4.conf.default.rp_filter=1
net.ipv4.conf.all.accept_redirects=0
net.ipv4.conf.all.send_redirects=0
EOF
sysctl --system

# 4. WireGuard anahtarları (Post-Quantum PresharedKey dahil)
mkdir -p /etc/wireguard
chmod 700 /etc/wireguard

SERVER_PRIV=$(wg genkey)
SERVER_PUB=$(wg pubkey <<<"$SERVER_PRIV")
CLIENT_PRIV=$(wg genkey)
CLIENT_PUB=$(wg pubkey <<<"$CLIENT_PRIV")
CLIENT_PSK=$(wg genpsk)

WAN_IF=$(ip route list default | awk '{print $5; exit}')
PUBLIC_IP=$(curl -fsS --max-time 8 https://api.ipify.org)

# 5. WireGuard yapılandırması — Cloud Metadata (169.254.169.254) & RFC1918 engelli + Stateful NAT
cat <<EOF >/etc/wireguard/wg0.conf
[Interface]
Address = 10.66.66.1/24
ListenPort = ${WG_PORT}
PrivateKey = ${SERVER_PRIV}
PostUp = iptables -I FORWARD 1 -i %i -d 169.254.0.0/16 -j REJECT; iptables -I FORWARD 2 -i %i -d 10.0.0.0/8 -j REJECT; iptables -I FORWARD 3 -i %i -d 172.16.0.0/12 -j REJECT; iptables -I FORWARD 4 -i %i -d 192.168.0.0/16 -j REJECT; iptables -A FORWARD -i %i -o ${WAN_IF} -j ACCEPT; iptables -A FORWARD -i ${WAN_IF} -o %i -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT; iptables -t nat -A POSTROUTING -s 10.66.66.0/24 -o ${WAN_IF} -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -d 169.254.0.0/16 -j REJECT || true; iptables -D FORWARD -i %i -d 10.0.0.0/8 -j REJECT || true; iptables -D FORWARD -i %i -d 172.16.0.0/12 -j REJECT || true; iptables -D FORWARD -i %i -d 192.168.0.0/16 -j REJECT || true; iptables -D FORWARD -i %i -o ${WAN_IF} -j ACCEPT || true; iptables -D FORWARD -i ${WAN_IF} -o %i -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT || true; iptables -t nat -D POSTROUTING -s 10.66.66.0/24 -o ${WAN_IF} -j MASQUERADE || true

[Peer]
PublicKey = ${CLIENT_PUB}
PresharedKey = ${CLIENT_PSK}
AllowedIPs = 10.66.66.2/32
EOF

chmod 600 /etc/wireguard/wg0.conf
systemctl enable --now wg-quick@wg0

# 6. Varsayılan-red (default-deny) Host Güvenlik Duvarı (UFW)
ufw default deny incoming
ufw default allow outgoing
ufw default deny routed
ufw allow "${SSH_PORT}/tcp" comment 'OzeVPN SSH Management'
ufw allow "${WG_PORT}/udp" comment 'OzeVPN WireGuard'
ufw --force enable

# 7. SSH Sertleştirme (yalnız yetkili anahtar varsa parola girişini kapatır, kilitlemeyi önler)
mkdir -p /etc/ssh/sshd_config.d
PASS_AUTH="yes"
if [ -s /root/.ssh/authorized_keys ] || [ -s "${SUDO_USER:+/home/${SUDO_USER}/.ssh/authorized_keys}" ]; then
  PASS_AUTH="no"
fi
cat <<EOF >/etc/ssh/sshd_config.d/99-ozevpn-hardening.conf
PermitRootLogin prohibit-password
PasswordAuthentication ${PASS_AUTH}
KbdInteractiveAuthentication no
MaxAuthTries 3
X11Forwarding no
AllowAgentForwarding no
EOF
chmod 600 /etc/ssh/sshd_config.d/99-ozevpn-hardening.conf
if sshd -t; then
  systemctl reload ssh || systemctl reload sshd || true
fi

# 8. İstemci yapılandırması — chmod 600, özel anahtar terminale BASILMAZ!
cat <<EOF >/root/ozevpn-client.conf
[Interface]
PrivateKey = ${CLIENT_PRIV}
Address = 10.66.66.2/32
DNS = 1.1.1.1

[Peer]
PublicKey = ${SERVER_PUB}
PresharedKey = ${CLIENT_PSK}
Endpoint = ${PUBLIC_IP}:${WG_PORT}
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
EOF
chmod 600 /root/ozevpn-client.conf

echo ""
echo "=== OzeVPN Sertleştirilmiş VPS (WireGuard + PQ PSK + BBR + UFW + Metadata Koruması) Hazır! ==="
echo "Endpoint          : ${PUBLIC_IP}:${WG_PORT}"
echo "Server Public Key : ${SERVER_PUB}"
echo "İstemci Dosyası   : /root/ozevpn-client.conf (izin: 0600 — güvenli SCP ile indirin)"
echo "Güvenlik Notu     : Özel anahtar ve PresharedKey terminal geçmişine yazılmadı."
echo "Peer İptali       : wg set wg0 peer ${CLIENT_PUB} remove"
echo "Aktif Portlar     :"
ss -tulnp | grep -E ":(${SSH_PORT}|${WG_PORT})\b" || true
