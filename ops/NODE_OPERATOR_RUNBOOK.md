# FTB operator runbook — fresh VM to verified node

One page. Copy-paste. If a step fails, stop and ask — do not improvise with keys.

## Step zero — secure the two keys (do this FIRST, once, on your own machine)

Two secrets control the project. Neither is in git. Neither ever touches a VM.
Back both up to a USB stick stored somewhere physical before anything deploys:

| secret | file | what it controls |
|---|---|---|
| Deploy wallet | `cmd/deploy/testnet.wallet.json` **+** `cmd/deploy/.pass` (useless without each other) | deploying contracts; token admin until `renounceOwnership` |
| IPNS key | `ops/ipfs/keys/ftb-ipns.key` | what the site's stable IPNS name points to |

## Step 1 — IPFS pinning node (each of the 5 VMs)

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

## Step 2 — Neo N3 TestNet RPC node (same VM)

Install neo-cli (TestNet config), run under systemd, expose RPC only after it syncs.
Report your RPC URL (`http://<vm-ip>:20332`) to the coordinator — the five URLs go
into `web/config.js` so the front end reads from the community, not public nodes.

## Step 3 — pin the front end (coordinator runs this, not the operators)

From the coordinator's Mac, once per VM:

```sh
ops/ipfs/ftb-push-ipfs.sh <user>@<vm-ip>
```

The same IPNS key is imported on every node, so all five publish **one** name.
The front end is a static build — what's on IPFS is byte-for-byte what was reviewed.

### the mirrors tile (how the page counts live nodes)

Every deploy stamps `beat.json` (`project`, `commit`, `published`) into the build.
The front end compares its own stamp against each mirror listed in `web/config.js`
(`mirrors:`) — reachable AND same commit shows as live. If you serve the site from
your VM's web server, allow cross-origin reads of that one file so the counter can
see you from any gateway or local preview:

```caddy
# Caddy: one header is enough
header /beat.json Access-Control-Allow-Origin *
```

Report your web URL to the coordinator alongside your RPC URL; both go into
`web/config.js`.

## Step 4 — DNSLink (domain owner, once)

```
_dnslink.freethebird.co  TXT  "dnslink=/ipns/<the shared IPNS name>"
```

Any IPFS gateway in the world then resolves freethebird.co. No single server —
including all five VMs at once — can take the site down by going offline.

## Rules of the house

- Never expose IPFS API (5001) or gateway (8080) publicly.
- Never put any key file on a VM other than the imported IPNS key, and that one
  arrives via the push script, never by hand.
- If a VM is lost or seized: nothing is lost. Content persists by CID; the name is
  republishable from the backup key.
