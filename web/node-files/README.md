# Run an FTB node (mirror)

Free the Bird's app is a bundle of static files published to IPFS and addressed by its
content hash. Any IPFS node can hold a copy and serve it. When you pin it, FTB lives on
your machine too -- one more copy nobody can switch off. You are supporting FTB (its app),
not running a Neo blockchain node. No credentials, no permission, no sign-up. There is no
token reward: FTB mints nothing for mirrors. You do it so the app cannot be taken down.

The stable pointer to the latest release:

    /ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq

## Files

| file | what it is |
|---|---|
| `ftb-follow.sh`       | tracks FTB releases -- pins the new one, unpins the old. Run from cron. |
| `ipfs-node-setup.sh`  | Level 2: a hardened Kubo node on a fresh Ubuntu VM (systemd + firewall). |

## Level 1 -- mirror the site (about 10 minutes)

1. Install Kubo (the IPFS software):

   ```sh
   # Linux (amd64):
   VER=$(curl -fsSL https://api.github.com/repos/ipfs/kubo/releases/latest | grep -o '"tag_name": *"[^"]*"' | cut -d'"' -f4)
   curl -fsSL "https://github.com/ipfs/kubo/releases/download/${VER}/kubo_${VER}_linux-amd64.tar.gz" | tar -xz
   sudo install -m 0755 kubo/ipfs /usr/local/bin/ipfs
   # macOS:
   brew install ipfs
   ```

2. Start your node:

   ```sh
   ipfs init --profile=server
   ipfs daemon &
   ```

3. Pin the FTB front-end (the IPNS name always resolves to the latest release):

   ```sh
   ipfs pin add /ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq
   ```

4. (Optional) Follow updates automatically -- run ftb-follow.sh every 15 minutes:

   ```sh
   chmod +x ftb-follow.sh
   ( crontab -l 2>/dev/null; echo '*/15 * * * * IPFS_PATH=$HOME/.ipfs /path/to/ftb-follow.sh >> $HOME/.ftb-follow.log 2>&1' ) | crontab -
   ```

## Level 2 -- a public mirror on your own domain

1. Provision a small Ubuntu 24.04 VM (1 vCPU / 2 GB / 40 GB is plenty).
2. Run `ipfs-node-setup.sh` on it (installs Kubo SHA-512-verified, a locked-down ipfs user,
   a systemd service, and a firewall: SSH + swarm 4001 only; API and gateway stay private).
3. Add `ftb-follow.sh` to cron so your copy tracks every release.
4. Serve your domain over HTTPS from the node's local gateway with Caddy, and allow the one
   header that lets FTB count you as a live mirror:

   ```
   your-domain.xyz {
       reverse_proxy 127.0.0.1:8080
       header /beat.json Access-Control-Allow-Origin *
   }
   ```

   Point the gateway at FTB's IPNS name (DNSLink or a Kubo gateway host mapping). The primary
   node's exact Caddy + gateway config is in `ops/ipfs/DEPLOYS.md`.
5. Share your URL (GitHub or Discord) to be added to the shipped mirror list.

## Check you are mirroring

```sh
ipfs name resolve /ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq
ipfs pin ls --type=recursive | head        # the resolved CID should be here
```

Or open `https://ipfs.io/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq`
in a browser -- the same FTB app your node now helps serve.

---

These steps mirror the repo's `ops/ipfs/REPLICATE.md`. IPNS name taken from the shipped
config on 2026-11-03. Volunteer infrastructure, not financial advice.
