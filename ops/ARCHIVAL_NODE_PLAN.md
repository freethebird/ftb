# FTB Archival Node - Commissioning Checklist (Explorer Backend)

Status: **planned, post-mainnet**. Decision recorded 2026-10-17: skip TestNet
archival entirely; commission a MainNet archival node only, when explorer work
begins after the Nov 7 launch.

## Why a separate VM

- The DEX node (`ftb-node`, pruned) serves live swap/lend/governance reads and
  sits near the deploy wallet's operational path. It must stay minimal.
- The explorer node is internet-facing, high-traffic, and disposable. If it is
  compromised or overloaded, the DEX must not notice.
- Pruned and archival are mutually exclusive config modes
  (`RemoveUntraceableBlocks: true` deletes old state; archival keeps all of it).
  They can never be the same process.

## VM spec

| Resource | Spec | Notes |
|---|---|---|
| Disk | 200 GB SSD | MainNet archival ~40–60 GB today, grows daily; 200 GB avoids mid-sync resize |
| RAM | 4 GB | Full-chain verification is memory-heavier than pruned operation |
| vCPU | 2 | Sync is CPU-bound on block verification |
| Network | Public IP, RPC behind reverse proxy | Explorer backend only |

## Binary - critical

Do **not** install the official neo-go v0.123.0 release binary. It ships the
lz4 v2.6.1 amd64 assembly decoder bug that freezes sync (see
`ops/NODE_LZ4_AMD64_FIX.md` and `ops/lz4-amd64-repro/`). An archival node reads
every block, so it hits the freeze with certainty.

Use the clean-room rebuild with `-tags noasm`:

- Source: official v0.123.0 tag, `go build -tags noasm`
- Known-good sha256 (TestNet node build):
  `b4515e878e2fd48f7b71540e9d84cb64acb88983126f54407175981028ea0b7c`
- Rebuild from source and compare; do not scp the binary blindly across trust
  domains without verifying the hash.

## Config deltas vs the DEX node

- `protocol.mainnet.yml` (not testnet)
- `RemoveUntraceableBlocks` **absent or false** - this is the whole point
- No `GarbageCollectionPeriod`
- NeoFS BlockFetcher: leave disabled/absent - its endpoints are dead globally,
  so archival sync is full P2P from genesis regardless
- RPC: bind localhost, front with nginx + rate limit for the explorer

## Sync expectation

Full P2P from genesis at observed rates: 1–3 days for MainNet archival.
Start the sync before explorer development needs data, not after.

## Checklist

- [ ] VM provisioned (200 GB / 4 GB / 2 vCPU), separate provider account or at
      least separate VM from `ftb-node`
- [ ] Hardening per `ops/` baseline (ssh keys only, ufw, no password auth)
- [ ] neo-go v0.123.0 built with `-tags noasm`, sha256 verified
- [ ] `protocol.mainnet.yml` with pruning OFF
- [ ] systemd unit `ftb-explorer-node.service` (mirror `ftb-node.service`)
- [ ] Sync from genesis to tip verified (`getblockcount` vs a public seed)
- [ ] RPC exposed only via reverse proxy with rate limiting
- [ ] No deploy keys, no IPNS keys, no wallet material on this box - ever
