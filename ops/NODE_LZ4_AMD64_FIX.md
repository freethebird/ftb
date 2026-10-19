# Node #1 sync stall - lz4 amd64 assembly bug (2026-10-16)

## Symptom

Node #1 (1984 / freethebird.co) froze at TestNet block **956,156** on 2026-10-15 08:42 UTC.
RPC refused to serve (`StartWhenSynchronized: true`), so Caddy returned 502 at
`freethebird.co/rpc` and the front end showed "community node unreachable or still syncing".

The log filled with ~2.3 errors/sec of:

```
peer disconnected {"error": "lz4: invalid source or destination buffer too short"}
```

206k+ occurrences, every NEO-GO peer affected, while block traffic from C# peers
arrived fine on the wire (verified by packet capture) yet nothing ever persisted.

## Root cause

**A data-dependent bug in the amd64 assembly decoder of `github.com/pierrec/lz4`
v2.6.1 (`decode_amd64.s`), used by neo-go v0.123.0** (latest release at the time).

A specific P2P message (compressed payload, 5392 bytes wire / 6981 bytes decoded)
fails decompression **only when decoded through the amd64 assembly path**:

| decoder | platform | result |
| --- | --- | --- |
| lz4 v2.6.1 pure-Go | any (e.g. darwin/arm64) | OK - md5 `f108be2ca7cb6f77877d3d68b940263f` |
| lz4 v2.6.1 asm | linux/amd64 (the VM) | `ErrInvalidSourceShortBuffer` |
| lz4 v2.6.1 `-tags noasm` | linux/amd64 (the VM) | OK - identical md5 |

Every peer sends this message right after handshake, so every connection died at
the first decode, and the node could never hold peers long enough to pull the one
batch it needed (blocks 956157–956656). Out-of-order batches filled the block
queue cache and were silently discarded - ~44 MB/25 s of wasted inbound traffic,
which also made C# peers RST us as abusive.

The stall started the moment the NeoFS BlockFetcher stopped carrying the sync
(NeoFS TestNet storage endpoints `st[1-4].storage.fs.neo.org:8080` unreachable
globally since 2026-10-15, from the VM and off-site alike) and P2P became the
only block source.

## Fix

Rebuilt **unmodified neo-go v0.123.0 source** with the pure-Go decoder:

```
GOOS=linux GOARCH=amd64 go build -tags noasm -o neo-go ./cli
```

(`decode_amd64.go` carries `+build !noasm`; the tag selects the audited pure-Go
path. No source changes.)

Deployed binary sha256:

```
b4515e878e2fd48f7b71540e9d84cb64acb88983126f54407175981028ea0b7c  /usr/local/bin/neo-go
```

Original official release binary kept on the VM as
`/usr/local/bin/neo-go-0.123.0-official-backup` (sha256 `71c0d482...`, byte-identical
to the GitHub release asset).

Result: zero lz4 errors after restart; sync resumed instantly
(956,156 → 1,000,000+ in ~2 minutes).

## Operator guidance

- Any amd64 neo-go build that links `pierrec/lz4` v2.x with the assembly decoder
  is exposed to this. Build with `-tags noasm`, or patch `pkg/network/compress.go`
  to `github.com/pierrec/lz4/v4` if a future neo-go release doesn't.
- `StartWhenSynchronized` is currently `false` on node #1 so `freethebird.co/rpc`
  stays answerable while the chain catches up; the front end reads and displays
  the height itself. Revisit once the node is at tip.
- NeoFS BlockFetcher stays enabled in config; it will resume automatically if the
  NeoFS TestNet endpoints come back, and P2P carries the sync meanwhile.

## Upstream

Worth filing: pierrec/lz4 (v2 branch, amd64 decode) and nspcc-dev/neo-go
(v0.123.0 ships the affected dependency; linux/amd64 release binaries are built
with the asm path). Payload sample that reproduces it is preserved in the
incident notes (5392-byte wire payload, `want=6981`).
