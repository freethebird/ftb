# pkg/evm - notes and the dependency decision

## What this package is

The minimal Ethereum wire primitives the Neo X (EVM) HTLC adapter needs:
Keccak-256, RLP, the static ABI subset for the FtbHtlc surface, EIP-1559
transaction signing, and a JSON-RPC client. Pure Go, standard library only,
plus the secp256k1 already compiled inside the one pinned neo-go dependency's
closure. No cgo, no further module.

## Dependency decision (Go Bible Book III: Article 2, Article 12)

The off-chain solver needs two crypto primitives the standard library does not
provide: Ethereum's legacy Keccak-256 (stdlib crypto/sha3 is NIST-padded SHA3,
a different function) and secp256k1 ECDSA with a recoverable signature.

1. secp256k1 signing uses github.com/decred/dcrd/dcrec/secp256k1/v4, which is
   ALREADY in the neo-go module closure (`go list -deps` of the N3 client shows
   it; go.sum already carried its hashes). The only go.mod change is moving it
   from the indirect block to a direct require - zero new modules enter the
   trust boundary; it is code already compiled into every neo-go binary. It
   gives RFC6979-deterministic, low-S, recoverable compact signatures - exactly
   the EIP-1559 (yParity, r, s) shape, built for this purpose. Hand-rolling
   secp256k1 was rejected: self-reviewed, non-constant-time ECC on a
   value-bearing key is the one place the Bible's caution bites.

2. Keccak-256 is hand-rolled in pure Go (keccak.go) - the lowest-risk hand-roll:
   a public hash, no secret-dependent branch, pinned to the canonical KATs that
   the in-house C accel-rail's Keccak also satisfies. The two agree
   byte-for-byte, the accel-rail stays the canonical reference, and nothing
   crosses a language boundary (the Go Bible bans cgo in the portable core, so
   the C rail cannot be linked here; a subprocess seam would add a C-toolchain
   dependency for a path that does a handful of hashes per swap).

3. RLP (iterative - Article 9), the ABI subset, and the JSON-RPC client are
   hand-rolled in pure stdlib.

Net: the solver is stdlib + the one pinned neo-go dependency, plus one direct
go.mod line for a module already inside that closure.

## PROVE coverage

- Keccak: canonical KATs + block-boundary guard.
- RLP: canonical vectors (empty, "dog", single byte, two-item list, long string).
- ABI: published ERC-20 selectors anchor keccak-over-signature + extraction;
  HTLC selectors pinned; full-calldata KAT.
- tx: canonical key=1 address; sign -> recover -> address; RFC6979 determinism.
- Read path bound LIVE to the deployed contracts: Neo X swaps()/Claimed decode
  (pkg/chain/neox, tag neoxlive) and N3 Swap()/Claimed decode (pkg/chain/neon3,
  tag n3live) match the chain byte-for-byte. The EIP-1559 sighash field order is
  bound by a live send (operator step, funded key).

## Resolved: amount is *big.Int end to end (was an Article 6 limit)

fasp.HTLC and the adapters carry a swap amount as *big.Int, so a swap spans the
full uint256 the on-chain contract supports: no uint64 cap, and decodeSwap no
longer rejects large values. (History: amount was uint64 through 2026-11-12,
which capped a Neo X swap at ~18 GAS and failed closed; elevated to *big.Int per
the Module 1 compliance audit, finding F-1.)
