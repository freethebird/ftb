# Run an FTB mirror node

Free the Bird's front-end lives on IPFS. Any IPFS node — a VPS, a home server, a
Raspberry Pi, even a laptop — can mirror it and make it more available for everyone.
You do **not** need any credentials or permission from the FTB operators.

There are two levels. Level 1 is 10 minutes and is what we ask of supporters.

---

## Level 1 — mirror the site (recommended for most people)

**1. Install Kubo** (the IPFS node software):

```bash
# Linux (amd64):
VER=$(curl -fsSL https://api.github.com/repos/ipfs/kubo/releases/latest | grep -o '"tag_name": *"[^"]*"' | cut -d'"' -f4)
curl -fsSL "https://github.com/ipfs/kubo/releases/download/${VER}/kubo_${VER}_linux-amd64.tar.gz" | tar -xz
sudo install -m 0755 kubo/ipfs /usr/local/bin/ipfs

# macOS:
brew install ipfs
```

**2. Initialize and start:**

```bash
ipfs init --profile=server   # 'server' profile = safe for machines with a public IP
ipfs daemon &                # or run under systemd/launchd for permanence
```

**3. Pin the current FTB front-end:**

```bash
ipfs pin add /ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq
```

Done. Your node now holds a copy and serves it to anyone on the swarm.
Even if every FTB-operated machine disappears, the site survives on yours.

**4. (Optional) Follow future updates automatically:**

Grab `ftb-follow.sh` from this directory and run it every 15 minutes from cron:

```bash
chmod +x ftb-follow.sh
( crontab -l 2>/dev/null; echo '*/15 * * * * IPFS_PATH=$HOME/.ipfs /path/to/ftb-follow.sh >> $HOME/.ftb-follow.log 2>&1' ) | crontab -
```

It resolves the IPNS name, pins new versions, and unpins superseded ones —
your mirror tracks releases within minutes, hands-free.

---

## Level 2 — full replica of the FTB node

If you want the exact setup of the primary node (Kubo as a hardened systemd
service on a VPS, Caddy serving your own hostname over HTTPS):

1. Provision any small Ubuntu 24.04 VM (1 vCPU / 2 GB RAM / 40 GB is plenty).
2. Run [`ipfs-node-setup.sh`](./ipfs-node-setup.sh) on it.
3. Run `ftb-follow.sh` via cron to mirror content.
4. To serve `https://<your-domain>` from your node, add a Caddy block proxying
   to `127.0.0.1:8080` (the local gateway) — Caddy handles TLS automatically.

The full architecture and deploy history of the primary node are documented
in [`DEPLOYS.md`](./DEPLOYS.md).

---

## Why this works

IPFS content is addressed by hash (CID), not by location. The FTB site is
currently `QmdKwwMEqAeWP8tokfDmiEAxqDYDF6DeAuuA6cDWktRPaw` — whoever pins that
CID serves bit-identical content, verifiable by anyone. The IPNS name above is
the stable pointer that always resolves to the latest release; DNSLink on
`freethebird.co` points at the same name. There is no central server in the
critical path — only publishers and mirrors.
