# FTB Infrastructure — Session Export (2026-10-14)

Complete record of the freethebird.co IPFS deployment session.
Everything below is reproducible from this document plus the `ftb` repo.

---

## 1. Infrastructure

| Item | Value |
|---|---|
| Provider | 1984 Hosting ehf., Reykjavík, Iceland (privacy jurisdiction, green power, AS44925) |
| Plan | VPS #2 — 1 vCPU / 2 GB RAM / 50 GB NVMe (~€17.44/mo) |
| IP | `185.112.147.245` (PTR: vps-185-112-147-245.1984.is) |
| OS | Ubuntu 24.04 LTS |
| SSH alias (on Mac) | `ftb-node` → root@185.112.147.245, key `~/.ssh/freethebird` |
| Kubo (IPFS) | 0.43.1, runs as `ipfs` user, `IPFS_PATH=/var/lib/ipfs`, systemd unit `ipfs.service` |
| Caddy | 2.6.2, reverse proxy `freethebird.co` + `www` → `127.0.0.1:8080` (Kubo gateway) |
| TLS | Let's Encrypt, auto-renewed by Caddy, valid to 2027-01-12 |
| Firewall (ufw) | Open: 22 (SSH), 80/443 (Caddy), 4001 (IPFS swarm). API 5001 + gateway 8080 are localhost-only |
| Peer ID | `12D3KooWE57LCSGAzpBY81Hu425yF4dBT4kCGNBB4xaQdmr7LtXB` |

## 2. Content identifiers

| Item | Value |
|---|---|
| IPNS name (stable pointer) | `k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq` |
| Live CID as of 2026-10-21 | `QmcfK1eRcJgeP48vpSwZqPbE8NNzNBwJYp9hEVA95TMB5z` |
| CIDs from this session | `QmdKwwMEqAeWP8tokfDmiEAxqDYDF6DeAuuA6cDWktRPaw` (dark-default) · `QmVYbZj6WUnxjGf86uzEVonXihNtJkZjysQ95sLfmWS6yn` (template 3) · `QmXpGF99ioKJfDskdkcBg9gLL4iSaM6RAsCnqv9mwLBx87` (template 2, first deploy) |
| IPNS key backup | `ftb/ops/ipfs/keys/ftb-ipns.key` (chmod 600, local Mac only — the ONLY irreplaceable artifact) |
| Deploy history | `ftb/ops/ipfs/DEPLOYS.md` (repo, Mac-push era) + `/var/log/ftb-deploys.md` on the node (current, git-deploy era). 60+ deploys in the first week. |

## 3. DNS (hosted at WordPress.com DNS)

| Record | Value |
|---|---|
| `A @` | `185.112.147.245` |
| `CNAME www` | `freethebird.co` |
| `TXT _dnslink` | `dnslink=/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq` |
| `TXT _dnslink.www` | same as above |

Rules learned the hard way:
- Exactly **one** dnslink TXT per name — duplicates make Kubo refuse resolution.
- No dnslink record on `@` (only `_dnslink.` names are read).
- NS stays at WordPress.com (registrar-side); 1984 only hosts the VPS.

## 4. Repo layout (github.com/freethebird/ftb)

```
ftb/web/                     static front-end (no build step; proofs.json is a symlink)
ftb/ops/ipfs/
  ipfs-node-setup.sh         full node bootstrap (Kubo + systemd + ufw) for any Ubuntu 24.04 VM
  ftb-deploy-from-git.sh     CURRENT deploy model: runs ON the node, pulls canonical git,
                             pins, republishes IPNS. MainNet rule: deploy governance-tagged
                             refs (gov-N), never ad-hoc pushes.
  ftb-push-ipfs.sh           legacy Mac-push deploy (superseded by deploy-from-git)
  ftb-follow.sh              mirror script for supporters (resolve IPNS, pin new, unpin old)
  DEPLOYS.md                 deploy log + rollback instructions
  REPLICATE.md               "run a mirror" guide to hand to the community
  keys/ftb-ipns.key          IPNS key backup (local only)
ftb/ops/                     expanded during the week of Oct 14–21:
  NODE_OPERATOR_RUNBOOK.md, DECENTRALIZED_OPS.md, ARCHIVAL_NODE_PLAN.md,
  1984_SETUP.md, T0_ONBOARDING.md, VALIDATE_WITH_CI.md, ftb-env-setup.sh
```

Git identity is locked repo-local: `freethebird <community@freethebird.invalid>`.
(2026-10-14: history rewritten with git filter-repo to purge a personal name
from 4 commits; force-pushed; remote verified clean.)

## 5. Daily operations

**Deploy a new version (current model — node pulls from git):**
```bash
# from any machine: just push to the repo
git push origin main
# the node self-deploys origin/main via ftb-deploy-from-git.sh
# MainNet: deploy a governance-tagged ref instead:
ssh ftb-node "sudo ftb-deploy-from-git.sh gov-N"
```

**Legacy deploy (Mac-push, still works):**
```bash
cd "/Users/merlin/Desktop/FLM KIMI" && ./ftb/ops/ipfs/ftb-push-ipfs.sh ftb-node
```

**Roll back to any previous version:**
```bash
ssh ftb-node "sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs name publish --key=ftb --lifetime=720h --ttl=15m /ipfs/<CID>"
```

**Health checks:**
```bash
ssh ftb-node 'systemctl is-active ipfs caddy; sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs swarm peers | wc -l'
curl -sI https://freethebird.co/ | head -1
dig TXT _dnslink.freethebird.co +short
```

**Restore the IPNS key on a fresh machine:**
```bash
ipfs key import ftb ftb-ipns.key   # reproduces the same k51qzi5... name
```

## 6. Architecture in one paragraph

The site is a static bundle pinned on IPFS, addressed by content hash. A stable
IPNS name always points at the latest CID; `_dnslink` TXT records connect the
domain to that name. Caddy on the VM terminates HTTPS and proxies to the local
Kubo gateway, which resolves the DNSLink chain and serves the content. Any
third party can mirror the site by pinning the IPNS name — no credentials
needed — so availability scales with the community, not with one provider.
The VM dying is an inconvenience (re-point A record, re-import key, re-pin),
not a data loss: content and identity both live outside the machine.

## 7. Open items / next candidates

- [ ] Second mirror node (community-run, via REPLICATE.md + ftb-follow.sh)
- [ ] `MIRROR.md` at repo root as the shareable quick-start block
- [ ] Optional: NNS (.neo) naming instead of ENS (FTB is NEO-native; ENS declined)
- [ ] Optional: purge dangling pre-rewrite commits on GitHub via support ticket
- [ ] Public gateways (ipfs.io etc.) were rate-limiting during setup — re-test
      `https://dweb.link/ipns/freethebird.co/` as a fallback access path
