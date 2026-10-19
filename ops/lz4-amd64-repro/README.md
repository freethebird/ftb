# lz4 v2.6.1 amd64 assembly decoder - data-dependent decode failure

Reproducer for the bug that froze FTB community node #1 on 2026-10-15/16
(full incident: ../NODE_LZ4_AMD64_FIX.md).

## Run it

```
go run main.go                 # linux/amd64: both payloads FAIL
go run main.go                 # darwin/arm64 (any non-amd64): both OK
go run -tags noasm main.go     # linux/amd64, pure-Go decoder: both OK
```

Observed on a linux/amd64 VM (QEMU vCPU, Go 1.27.x), and in production by every
neo-go v0.123.0 linux/amd64 node that receives this message (official release
binary, go1.27.0, sha256 71c0d482… - byte-identical to the GitHub asset).

## The payloads

Both files carry the same 6981-byte Neo N3 P2P message, compressed independently
by two different encoders - proof that the bug is in the amd64 *decoder*, not in
any encoder:

- `payload-neo-csharp.bin` (5392 bytes) - compressed by the C# node's K4os LZ4
- `payload-neo-go.bin` (5524 bytes) - compressed by lz4 v2.6.1's own `CompressBlock`

Format: 4-byte little-endian decompressed length, then the raw LZ4 *block* stream
(the N3 P2P `compress()` framing in neo-go `pkg/network/compress.go`).

Both decode identically everywhere except through v2.6.1's `decode_amd64.s`;
reference md5 of the decoded bytes: `f108be2ca7cb6f77877d3d68b940263f`
(independent pure-Python decoder agrees).

---

## Draft issue for pierrec/lz4

**Title:** v2.6.1: amd64 assembly decoder fails with ErrInvalidSourceShortBuffer on valid input (data-dependent)

**Body:**

`UncompressBlock` on v2.6.1+incompatible rejects a valid LZ4 block stream when
running the amd64 assembly path (`decode_amd64.s`); the pure-Go path
(`-tags noasm`) and all non-amd64 builds decode the same bytes correctly.

Reproducer: https://github.com/freethebird/ftb/tree/main/ops/lz4-amd64-repro
(two payloads - one produced by lz4 v2.6.1's own `CompressBlock`, one by
K4os.Compression.LZ4 - same 6981-byte message; `go run main.go`).

- linux/amd64 (Go 1.27.x): `lz4: invalid source or destination buffer too short`, n=0
- linux/amd64 with `-tags noasm`: OK
- darwin/arm64: OK

This is hit in production by neo-go v0.123.0 N3 nodes on amd64: a specific P2P
message that every peer sends after handshake fails to decode, the peer is
disconnected, and block sync stalls permanently. Impact is network-wide for
amd64 operators.

## Draft issue for nspcc-dev/neo-go

**Title:** v0.123.0 linux/amd64: block sync stalls - pierrec/lz4 v2.6.1 asm decoder fails on a valid peer message

**Body:**

On linux/amd64 (official release binary and source builds alike), every peer
connection dies with `lz4: invalid source or destination buffer too short` as
soon as the peer sends a particular compressed message (~7 KB, an early-post-
handshake state/inventory-type message). The queue fills with out-of-order
blocks but the one needed batch never survives, so a syncing node stalls
indefinitely with zero blocks persisted and multi-MB/s of discarded inbound
traffic.

Root cause is upstream: `pierrec/lz4` v2.6.1's amd64 assembly decoder is
data-dependently broken (reproducer + payloads:
https://github.com/freethebird/ftb/tree/main/ops/lz4-amd64-repro). C# nodes are
unaffected; amd64 neo-go nodes disconnect from every peer that sends the message.

Workaround that fixed our node: rebuild unmodified v0.123.0 with `-tags noasm`.

Suggested fixes: bump `pkg/network/compress.go` to `github.com/pierrec/lz4/v4`
(same `CompressBlock`/`UncompressBlock` API), or build release binaries with
`-tags noasm` until then.
