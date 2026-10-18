# FTB Deploy Log — freethebird.co

Every deploy pins a new CID on the IPFS node (`ftb-node` / 185.112.147.245, 1984.is Reykjavík)
and repoints the stable IPNS name:

- IPNS: `k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq`
- DNSLink: `_dnslink.freethebird.co TXT "dnslink=/ipns/k51qzi5uqu5diamhszrr4bca4ncq45a5xcp0fsh2mvpy0namgg0m3h147f6cfq"`
- IPNS key backup: `ftb/ops/ipfs/keys/ftb-ipns.key` (local only, never on the VM image)

## Rollback

Any version below is permanently retrievable at its CID. To roll back, repoint IPNS:

```bash
ssh ftb-node "sudo -u ipfs IPFS_PATH=/var/lib/ipfs ipfs name publish --key=ftb --lifetime=720h --ttl=15m /ipfs/<CID>"
```

## Deploys

| # | Date (local) | CID | Git commit | Notes |
|---|---|---|---|---|
| 3 | 2026-10-14 ~15:35 | `QmdKwwMEqAeWP8tokfDmiEAxqDYDF6DeAuuA6cDWktRPaw` | `a9edb03` | Dark theme default for everyone; toggle still allows light. **Current.** |
| 2 | 2026-10-14 ~15:19 | `QmVYbZj6WUnxjGf86uzEVonXihNtJkZjysQ95sLfmWS6yn` | `82e7549` | Template 3 promoted to / (flamingo blend, phosphor keys). |
| 1 | 2026-10-14 ~12:25 | `QmXpGF99ioKJfDskdkcBg9gLL4iSaM6RAsCnqv9mwLBx87` | ≈ `2e4257a` (working tree) | First IPFS deploy. Template 2 "station dashboard". Node + Caddy + DNSLink live. |

## Deploy command

```bash
cd "/Users/merlin/Desktop/FLM KIMI" && ./ftb/ops/ipfs/ftb-push-ipfs.sh ftb-node
```

The script stages `ftb/web/` (resolving the `proofs.json` symlink), pins on the node,
republishes IPNS, and appends a row to this log.
| auto | 2026-10-14 16:23 | `QmdKwwMEqAeWP8tokfDmiEAxqDYDF6DeAuuA6cDWktRPaw` | `43b25c9` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-15 00:51 | `QmXthByaKnkg2kTnua2nxHKTFq8keBuo9w8ZuexsgJcavc` | `b90f218` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-16 11:06 | `QmebKAeP32MAbAUWk1jwbM1pgQLXkKbuuUU34aUG6j6Mbo` | `a4d63cd` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-16 11:37 | `Qmb2nnw4AunGXbmuMszRhnCT11pnJYvXwkmXxPiZCs9hJ1` | `97f6f3c` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-16 16:48 | `Qmdb87SbKZcXzXVUwRxSTk29mh4aCvKiUEvZLg8yw9Aedm` | `ea8a58e` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-17 11:31 | `QmYLBXgyeRJ3uju429jr7Rro6SfzwZFf1NAzuVQjcTYh4U` | `ef4a679` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 10:31 | `QmYX9u5AZYNCpmMDvDcMXXrMtjcPGyKjsB13oricSSR6z5` | `8ce4b6c` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 10:41 | `QmdQrfAYu1unELGR8VvwLmcfPpMxKn2orjGJrcGzSAVwnd` | `854a30b` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 11:09 | `QmPpKirwJAdzXB3KzPpoSqfEWfDNzPzZQhKxq1QZhRraJw` | `879d410` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 11:57 | `Qma6s1p28JM2R6YTomPEWiseoiD9q14JrSud9sobeCkXjy` | `80c6576` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 12:46 | `QmVWvsu3b7zHnh2NjpUfrCqnXaW5zrSgLb19mVHbgNo7jp` | `d573462` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 16:52 | `QmTXnQB6MzqZzpCcULxMf5rezLEq9fE6qQhmuoZACZk96k` | `9276dfd` | Deployed via ftb-push-ipfs.sh |
