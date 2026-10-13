#!/bin/sh
# FTB community node installer — Neo N3 TestNet (neo-go + Caddy).
# Run as root:  sudo sh node/setup.sh
# Installs a pinned neo-go release, a systemd unit, and Caddy as the TLS front.
set -eu

NEOGO_VERSION="0.123.0"
NEOGO_ORG="nspcc-dev"
CONF_DIR="/etc/ftb-node"
DATA_DIR="/var/lib/ftb-node"
BIN="/usr/local/bin/neo-go"

if [ "$(id -u)" -ne 0 ]; then
  echo "run as root: sudo sh $0" >&2
  exit 1
fi
if [ "$(uname -s)" != "Linux" ]; then
  echo "this installer targets Linux VMs; on macOS run neo-go manually" >&2
  exit 1
fi

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported arch: $ARCH" >&2; exit 1 ;;
esac

echo "==> neo-go v${NEOGO_VERSION} (linux-${ARCH})"
URL="https://github.com/${NEOGO_ORG}/neo-go/releases/download/v${NEOGO_VERSION}/neo-go-linux-${ARCH}"
curl -fL "$URL" -o "$BIN"
chmod 755 "$BIN"
"$BIN" --version || true

echo "==> ftb user and directories"
if ! id ftb >/dev/null 2>&1; then
  useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin ftb
fi
mkdir -p "$CONF_DIR" "$DATA_DIR/chain"
chown -R ftb:ftb "$DATA_DIR"

echo "==> protocol config"
cat > "$CONF_DIR/protocol.testnet.yml" <<'YML'
# Neo N3 TestNet protocol config for the FTB community node (neo-go v0.123+).
ProtocolConfiguration:
  Magic: 894710606
  SeedList:
    - seed1t.neo.org:20333
    - seed2t.neo.org:20333
    - seed3t.neo.org:20333
    - seed4t.neo.org:20333
    - seed5t.neo.org:20333
  StandbyCommittee: []
  ValidatorsCount: 7

ApplicationConfiguration:
  DBConfiguration:
    Type: leveldb
    LevelDBOptions:
      DataDirectoryPath: /var/lib/ftb-node/chain
  # RPC is bound to localhost only; Caddy fronts it with TLS and CORS.
  RPC:
    Enabled: true
    Bind: 127.0.0.1
    Port: 10332
    MaxGasInvoke: 50
    MaxIteratorResultItems: 100
  # No wallet unlocking, no sessions: this node never holds keys.
  UnlockWallet:
    Enabled: false
YML

echo "==> systemd unit"
cat > /etc/systemd/system/ftb-node.service <<'UNIT'
[Unit]
Description=FTB community Neo N3 TestNet node (neo-go)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ftb
ExecStart=/usr/local/bin/neo-go node --config-file /etc/ftb-node/protocol.testnet.yml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
# hardening: the node needs its chain dir and nothing else
ProtectSystem=full
ReadWritePaths=/var/lib/ftb-node
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now ftb-node

echo "==> Caddy (TLS front)"
if ! command -v caddy >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update
    apt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl gnupg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
      | gpg --dearmor --yes -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
      > /etc/apt/sources.list.d/caddy-stable.list
    apt-get update
    apt-get install -y caddy
  else
    echo "no apt-get found — install Caddy manually: https://caddyserver.com/docs/install" >&2
  fi
fi

if [ ! -f /etc/caddy/Caddyfile ] || ! grep -q "ftb-node" /etc/caddy/Caddyfile 2>/dev/null; then
  cat > /etc/caddy/Caddyfile <<'CADDY'
# ftb-node: replace node.example.invalid with your domain (A record -> this VM),
# then: sudo systemctl reload caddy
node.example.invalid

# browsers preflight cross-origin RPC calls; answer them here
@options method OPTIONS
respond @options 204

header {
	Access-Control-Allow-Origin *
	Access-Control-Allow-Methods "POST, OPTIONS"
	Access-Control-Allow-Headers "Content-Type"
	-Server
}

reverse_proxy 127.0.0.1:10332
CADDY
fi
systemctl enable caddy >/dev/null 2>&1 || true
systemctl restart caddy || true

echo
echo "done. next:"
echo "  1. edit /etc/caddy/Caddyfile — replace node.example.invalid with your domain"
echo "  2. sudo systemctl reload caddy"
echo "  3. verify: curl -s -X POST https://<your-domain>/ -H 'Content-Type: application/json' \\"
echo "       -d '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"getblockcount\",\"params\":[]}'"
echo "  4. hand your URL to the front-end maintainer for web/config.js"
echo
echo "node logs: journalctl -u ftb-node -f"
