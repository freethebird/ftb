#!/usr/bin/env bash
# FTB IPFS pinning node setup — run ON a fresh Ubuntu 24.04 VM as root (or via sudo).
# Installs Kubo, creates a dedicated 'ipfs' user, systemd service, and firewall.
set -euo pipefail

echo "==> Installing dependencies"
apt-get update -qq
apt-get install -y -qq curl tar ufw jq >/dev/null

echo "==> Creating ipfs user"
id ipfs &>/dev/null || useradd -r -m -d /var/lib/ipfs -s /usr/sbin/nologin ipfs

echo "==> Installing Kubo (latest stable)"
ARCH="$(dpkg --print-architecture)"           # amd64 or arm64
VER="$(curl -fsSL https://dist.ipfs.tech/kubo/versions | tail -n1 | sed 's/^v//' | tr -d '[:space:]')"
echo "    Kubo version: v${VER} (${ARCH})"
curl -fsSL "https://dist.ipfs.tech/kubo/v${VER}/kubo_v${VER}_linux-${ARCH}.tar.gz" -o /tmp/kubo.tgz
tar -xzf /tmp/kubo.tgz -C /tmp
install -m 0755 /tmp/kubo/ipfs /usr/local/bin/ipfs
rm -rf /tmp/kubo /tmp/kubo.tgz

echo "==> Initializing IPFS repo (server profile: refuses to dial private/LAN addresses)"
sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs init --profile=server
# Faster IPNS resolution via pubsub; keep API + gateway bound to localhost only.
sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs config --json Experimental.AcceleratedDHTClient true

echo "==> Creating systemd service"
cat > /etc/systemd/system/ipfs.service <<'UNIT'
[Unit]
Description=IPFS daemon (Kubo)
After=network-online.target
Wants=network-online.target

[Service]
Environment=IPFS_PATH=/var/lib/ipfs
User=ipfs
Group=ipfs
ExecStart=/usr/local/bin/ipfs daemon --enable-gc --enable-namesys-pubsub
Restart=always
RestartSec=5
LimitNOFILE=1000000
# Hardening
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now ipfs

echo "==> Configuring firewall (SSH + IPFS swarm 4001 only)"
ufw allow OpenSSH >/dev/null
ufw allow 4001 >/dev/null        # swarm, tcp + udp
ufw --force enable >/dev/null

sleep 3
echo
echo "==> Node identity:"
sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs id --format='<id>  <addrs>' | head -5
echo
echo "DONE. Node is running."
echo "  - Swarm port 4001 is open to the world (required)."
echo "  - API (5001) and gateway (8080) are localhost-only — do NOT expose them publicly."
echo "Next: from your Mac, run ftb-push-ipfs.sh <user>@<this-vm-ip> to pin the FTB front-end."
