# FTB operator runbook - fresh VM to verified node

One page. Copy-paste. If a step fails, stop and ask - do not improvise with keys.

## Step zero - secure the two keys (do this FIRST, once, on your own machine)

Two secrets control the project. Neither is in git. Neither ever touches a VM.
Back both up to a USB stick stored somewhere physical before anything deploys:

| secret | file | what it controls |
|---|---|---|
| Deploy wallet | `cmd/deploy/testnet.wallet.json` **+** `cmd/deploy/.pass` (useless without each other) | deploying contracts; token admin until `renounceOwnership` |
| IPNS key | `ops/ipfs/keys/ftb-ipns.key` | what the site's stable IPNS name points to |

## Step 1 - IPFS pinning node (each of the 5 VMs)

On a fresh Ubuntu 24.04 VM, as root:

```sh
curl -fsSL <repo-raw-url>/ops/ipfs/ipfs-node-setup.sh | less   # read it first
# then:
bash ipfs-node-setup.sh
```

This installs Kubo (SHA-512 verified, fail-closed), creates the locked-down `ipfs`
user, the systemd service, and the firewall (SSH + swarm 4001 only; API and gateway
stay on localhost).

Verify:

```sh
sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs id        # prints peer id + addresses
sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs swarm peers | head   # should see peers
```

## Step 2 - Neo N3 TestNet RPC node (same VM)

Install neo-cli (TestNet config), run under systemd, expose RPC only after it syncs.
Report your RPC URL (`http://<vm-ip>:20332`) to the coordinator - the five URLs go
into `web/config.js` so the front end reads from the community, not public nodes.

## Step 3 - pin the front end (coordinator runs this, not the operators)

From the coordinator's Mac, once per VM:

```sh
ops/ipfs/ftb-push-ipfs.sh <user>@<vm-ip>
```

The same IPNS key is imported on every node, so all five publish **one** name.
The front end is a static build - what's on IPFS is byte-for-byte what was reviewed.

### the mirrors tile (how the page counts live nodes)

Every deploy stamps `beat.json` (`project`, `commit`, `published`) into the build.
The front end compares its own stamp against each mirror listed in `web/config.js`
(`mirrors:`) - reachable AND same commit shows as live. If you serve the site from
your VM's web server, allow cross-origin reads of that one file so the counter can
see you from any gateway or local preview:

```caddy
# Caddy: one header is enough
header /beat.json Access-Control-Allow-Origin *
```

Report your web URL to the coordinator alongside your RPC URL; both go into
`web/config.js`.

## Step 4 - DNSLink (domain owner, once)

```
_dnslink.freethebird.co  TXT  "dnslink=/ipns/<the shared IPNS name>"
```

Any IPFS gateway in the world then resolves freethebird.co. No single server -
including all five VMs at once - can take the site down by going offline.

## Rules of the house

- Never expose IPFS API (5001) or gateway (8080) publicly.
- Never put any key file on a VM other than the imported IPNS key, and that one
  arrives via the push script, never by hand.
- If a VM is lost or seized: nothing is lost. Content persists by CID; the name is
  republishable from the backup key.

## The node that serves the site (ftb-node, 1984 Reykjavik)

The reference deployment runs the full stack on one box. Everything below is in git under
`ops/ipfs/` (deploy pipeline) and `ops/node/` (monitoring + security), so the box is
rebuildable from the repo. Install paths: scripts -> `/usr/local/bin/`, units ->
`/etc/systemd/system/`, drop-ins -> their matching `*.d/` dirs.

### Deploy pipeline (gated, self-healing)

Full model in `ops/ipfs/DEPLOYS.md`. In one line: `main` integrates, `live` ships, and a
publish-time gate (`ftb-deploy-gate.sh`) blocks a broken tree - syntax error, wrong config
profile, bad `proofs.json`, missing assets, or a leaked secret - **before** anything is
pinned, so a bad deploy never touches the live site. `ftb-autodeploy.timer` polls
`origin/live` every 2 min and deploys advances through the gate; a gate reject is flagged in
`/var/lib/ftb-deploy/gate-failed` and not retried until `live` moves.

### Hardening

- **SSH: key-only.** `ops/node/sshd-10-ftb-hardening.conf` -> `/etc/ssh/sshd_config.d/`:
  password and keyboard-interactive auth off, root by key only - nothing to brute-force.
- **Firewall.** ufw allows only 22 (SSH), 4001 (IPFS swarm), 80/443 (Caddy). neo-go RPC
  (20332/10332), the IPFS API (5001) and gateway (8080) stay bound to localhost, reached only
  through Caddy's `/rpc` and the gateway proxy.
- **Auto-updates.** unattended-upgrades applies security patches. If `apt-get update` ever
  errors on a third-party repo key, that silently stops security updates - fix the key first.
- **Caddy.** `/rpc` -> 20332 (CORS, no-store), `/status.json` -> file_server (no-cache),
  everything else -> the IPFS gateway. Static assets get a short `max-age`; HTML and
  `beat.json` stay `no-cache` so a deploy propagates within a minute; HSTS is on; the
  gateway's own `Cache-Control` is stripped so ours is the only one. `ops/node/caddy-restart.conf`
  adds `Restart=on-abnormal` so Caddy returns after a crash or reboot.

### Monitoring and self-healing

- **Status feed.** `ops/node/ftb-status.py` (via `ftb-status.timer`, every 1 min) writes
  `/var/lib/ftb-status/status.json` - both nodes' height/peers and the watchdog state - which
  Caddy serves at `/status.json` and the analytics page renders live.
- **Node watchdogs.** `ftb-mainnet-stallcheck.sh` (every 5 min) restarts the MainNet node if
  its height has not advanced across ~10 min (2 checks). `ftb-testnet-stallcheck.py` (every
  5 min) is peer-aware: it restarts the TestNet node only if it is frozen **and** behind its
  peers by more than 50 blocks for ~10 min, so a quiet network at the tip never needlessly
  restarts the node that serves the live dApp.

### Keys and backups

- **IPNS identity** (`ftb` key) is the site's stable name. An **encrypted** backup lives
  off-box (`ipfs-identity-backup.enc`, AES-256) with the passphrase stored separately. If the
  box is lost, content persists by CID and the name is republishable from the backup key.
- **Deploy key** (`/etc/ftb/deploy_key`) is read-only - it pulls the repo, never pushes.

### Recovery

- **A gate blocked a deploy.** Read `/var/lib/ftb-deploy/gate-failed` for the reason and
  commit; fix on `main`, re-point `live`. Nothing was published - the site is on the last-good
  CID.
- **Roll back the site.** `git branch -f live <good-commit> && git push --force-with-lease
  origin live`; the node redeploys it. Or republish a past CID directly (see DEPLOYS.md).
- **Restart a service.** `systemctl restart ftb-node` (TestNet) / `ftb-node-mainnet` / `ipfs`
  / `caddy`. The watchdogs do this automatically for a stalled node.
- **SSH lockout** (revert from an already-open session): `sudo rm
  /etc/ssh/sshd_config.d/10-ftb-hardening.conf && sudo systemctl reload ssh`.
