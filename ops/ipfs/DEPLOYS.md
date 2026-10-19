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
| auto | 2026-10-18 16:53 | `QmTYWL7DPzLoazCigMGhT6UGKggDZ25x3wz7EH7AjLsbDL` | `dc1ac41` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 17:24 | `QmWbSUFstRisbfHy7om8UPsSQRxocTwaUsprnNybza61P7` | `a10d1c2` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 18:23 | `QmR5eXhEeHH1toFwFvs2o6KP1JrVNFpbhzNmvPv4ey9Gaj` | `bac64b4` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 18:35 | `QmPQW4t3WrrtoRoMZYi8b7DErpYZcqaF7zzuRgj7KkywdC` | `e8a4240` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 19:02 | `QmY2tmLixxWMbS48fx6jMTKhNot1wRPfX9Jjdx4H1wWiBv` | `e116026` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 19:25 | `QmQPuP9hGU84FGw67XNWh9ofNwu4sWsBHJMDBAUgKNZFDU` | `f5bc16d` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 19:41 | `QmUZtBfNb1akjzXBC1cAZrrZrUzoVjupGnyc489rjNT1Uc` | `c9f8062` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 19:49 | `QmNLzyGBL5VpGyMrsLGZUEmghFV5MQpGT2HPjTgnj24gNC` | `8e76b8a` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 19:55 | `QmSRRnASXNJ377UfPKabM2XkUHJetwxwszjGa7FdXKXMHk` | `c8412d5` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 19:59 | `QmVHTFEkuCGVjPr3ncuE7jARPfPXGMyn5QKwjLqWEE1axK` | `5e0ece2` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 20:00 | `QmVrapjZcXPVA61WNWzoVpLaCVrpUSxtDo7HJawndMcbn9` | `ef6cf7f` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 20:05 | `QmVX3oi8rQGJEjPWoj2MMXK4QETsd7u2pXn7CC2y19ihrJ` | `38d8b63` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 20:15 | `QmREsBwLgQN7fGzFRNzf9Jbk2rSZmo2kfa5Gbcg5zL6iPz` | `04dcf00` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 20:38 | `QmfX74wQDWoRmH9YeQDDtxhEw3y5dm4RTSavGajaLq5QLd` | `cd56684` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 22:24 | `Qmb367uPT8dYWnTcoGzQ8xbwgMwPKp4HasNVrfYj62gY88` | `e103f6b` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 22:31 | `QmTpND6drr4U24VegdRxrmnZnGe8J2KHQhRSjAn4ATZyZE` | `4545e68` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 22:41 | `QmcRsz3TrVbzRVDdEzMic7KyZGBhLNNVJHjsdvm9cMMYVc` | `043deb8` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 22:54 | `QmdqcBHbQXVLefxTgexuhcAeNx1pbFVk5MfCfpzLAe6cSz` | `8b70cec` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 23:06 | `QmcCXSErkkdwPzoCRCaRq3wdLE8W8xyhEgFEWkbjuWfLPH` | `21cfb9d` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 23:12 | `QmYMa8WcpPEZFcLqpf9K7ojHGBoiiTDympWeVy9hB8QsPJ` | `7cea2f6` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 23:17 | `QmTZgxBpZRLXaRDQofyZ1aoJscZM1HE869cJi9GDXc8o2g` | `e770a77` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 23:34 | `QmQVVP8UcdJmKN57WsmEyBuXco9dbZt4NK95zmw7ng9JDd` | `c27a9dd` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-18 23:55 | `QmZBcm56RtNT93FSPRNc21r533NTsNKsnKqSGFy5La4d2B` | `048e574` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 00:04 | `QmUqzGnD6Qk6XJ69CCksnXqLyzD4D2UV8X3hdbHvb5ewrR` | `eb9357a` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 00:11 | `QmYy33D37hRvXrWZ54iwkRpaN7v3RDC4fYmc2GHEvkjZyV` | `ba9235c` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 09:37 | `QmcNEJSnLdGosWEEynuhcgC3VQtFPPe46nojd1aBWdGs3y` | `23ca421` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 09:45 | `Qmf42KmBh9ekJgDXvGKg655khnCwLL9YXwocM66fWrXPvS` | `50b4d89` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 10:16 | `QmWBX8FTPxh4Rgj2MTUQ7ydJTXhVTvvwCjsDQ28wb8d1NC` | `1ed3ba1` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 11:14 | `QmVo6TpwoPb51w51L8Z4qFU9HK4TLX892oSpGWCPbKmRfA` | `cfca105` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 11:29 | `QmVojWSQtup9haqSA2f6sWXTVDLfV4kwHpXaZAnCg9WBo1` | `90c9bf1` | Deployed via ftb-push-ipfs.sh |
| auto | 2026-10-19 11:35 | `QmNxaA5wvn3vUdZgxxX39fWkvshKemC1dijecUSb8LZVom` | `ca16723` | Deployed via ftb-push-ipfs.sh |
