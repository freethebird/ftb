# FTB second audit pass

Date: 10 October 2026. Scope: the full stack — `contracts/ftb_token`, `contracts/ftb_migrator`,
`contracts/registry`, `contracts/oracle`, `contracts/lend`, `pkg/merkle`, `cmd/merkleroot`,
`cmd/deploy`, `cmd/seed`, `web/ftb-dapi.js`, the manifests (`*.yml`), `Makefile`, and CI. This
pass re-verified the first audit's claims line by line and then looked for what it missed.
Method: cold read of every file above; cross-check of contract behavior against manifest
permissions and `safemethods`; trace of every storage writer and every external call.

Environment note: no Go toolchain on this machine, so nothing was compiled or executed here.
Every statement below is from source reading. Run `make` (or the free GitHub Actions path in
`ops/VALIDATE_WITH_CI.md`) before relying on any fix.

## Re-verified: the first audit's claims hold in code

- Single mint path: the only writer that raises `keySupply` is `ftb_token.MintForMigration`,
  behind all three gates (calling hash == migrator, `keyOpen != 0`, `supply+amount <= maxSupply`),
  with no external call inside the mint. Confirmed no other writer exists.
- Close-then-renounce interlock (FTB-03): `RenounceOwnership` reverts unless `keyOpen == 0`.
- Write-once migrator root (FTB-02): second `Configure` reverts.
- Merkle domain separation (FTB-11): leaves tagged `0x00`, nodes `0x01`, identical construction
  in `pkg/merkle` and on-chain `Claim`; sorted-pair order matches (`lessBytes` == `bytes.Compare`).
- Lend fixes FTB-07/08/09/10 all present: close-factor half (full when underwater), bonus cap
  30%, rate ceiling `maxRate`, `pre > 0` / `shares > 0` guards, events on `Configure`/`SetParams`,
  debt-free collateral withdrawal without the oracle.
- Checks-effects-interactions holds in every lend mutation; the `done` marker in `Claim` is
  written before the mint call; all cross-contract reads (`balanceOf`, registry `get`, oracle
  `price`) use `contract.ReadOnly`, and each target method is correctly listed in the callee
  manifest's `safemethods`, so the calls resolve.
- Manifests are minimal: oracle and registry declare no permissions (they call nothing); the
  token's wildcard is scoped to `onNEP17Payment` only, which NEP-17 requires.
- `repayIn`/`liquidateIn` bnorm subtraction cannot underflow: `pay <= floor(bnorm*bi/scale)`
  implies `floor(pay*scale/bi) <= bnorm`.
- `poolValue` cannot be manipulated inside a transaction: borrow/repay move value between
  `cash` and `totBorrow` one-for-one, donations are blocked at the callback, and `accrue`
  runs first on every entry point.
- Test suites exist as claimed (8 token, 2 migrator, 10 lend, 3 registry, 2 oracle, 3 merkle),
  including the guard tests the first pass added.

## New findings

| ID | Sev | Area | Title |
|----|-----|------|-------|
| FTB-14 | Medium | deploy + all contracts | No ownership-transfer path; docs instruct an impossible step | **Fixed 10 Oct:** `cmd/deploy` now decouples owner from signer (`owner_multisig` pubkeys → computed hash, or `owner_scripthash`), deploys with the multisig as owner from the first block, prints the exact multisig wiring calls, and warns loudly on single-key fallback. Password can come from `password_env` (FTB-12). Ops flow in `ops/DECENTRALIZED_OPS.md`. |
| FTB-15 | Low | lend | `send()` ignores the NEP-17 boolean result; bNEO is an external contract | **Fixed 10 Oct:** `send()` panics unless the transfer returns true; lend suite green. |
| FTB-16 | Low | cmd/merkleroot | Skipped snapshot rows only print to stderr; root and cap stay self-consistent but wrong | **Fixed 10 Oct:** hard-fails on any skipped row unless `-allow-skip`; verified: real snapshot reproduces the identical root, a bad row exits 1. |
| FTB-17 | Medium | workspace hygiene | Two divergent contract trees; the C# tree documents a different Merkle leaf format | **Mitigated 10 Oct:** superseded banners on `contracts/README.md` and `UnifiedToken_FTB.cs`; deletion still recommended before publishing the repo. |
| FTB-18 | Info | web/ftb-dapi.js | Base58 decode skips checksum and version-byte validation |
| FTB-19 | Info | CI | No committed `go.sum`; builds resolve dependencies fresh every run | **Fixed 10 Oct:** `go.sum` committed; CI uses `go mod download` instead of `tidy`. |
| FTB-20 | Info | lend | Liquidator not protected in underwater seizes (self-harm only) |

### FTB-14 (Medium) no way to move owner to the multisig

No contract in the stack has a `SetOwner`/`TransferOwnership` method. Yet both
`ftb/README.md` (line 118) and `cmd/deploy/main.go`'s closing instructions say to "move owner
to the multisig" after the window — a step that cannot be executed as coded. Compounding it,
`cmd/deploy` derives the owner from `Accounts[0]` of a single-sig wallet and drives admin calls
with `actor.NewSimple`, so through the whole trusted window (`SetMigrator` re-pointing, `Update`,
`CloseMigration`, minting up to the cap — the FTB-02 residual) the admin authority is one hot
key on the operator's machine, not the published multisig the docs promise.

Fix (either): (a) decouple signer from owner in the deploy config — add `owner_scripthash`,
pass the multisig hash as `_deploy` data (it does not need to sign at deploy time), and issue
the wiring calls from a multisig actor; or (b) add a witnessed `SetOwner` usable pre-renounce.
Until one of these lands, treat the docs' "owner multisig" language as aspirational and FTB-02's
window as guarded by a single key.

### FTB-15 (Low) lend `send()` drops the transfer result

`send()` calls `transfer` and discards the returned bool. Every send is preceded by a balance or
collateral check, so with the FTB token this cannot fire today. But bNEO is an external contract
(NeoBurger): if its `transfer` ever returns `false` instead of faulting, lend has already debited
the user's collateral in storage and the tokens never move — accounting and custody diverge and
the funds strand. One-line hardening: panic unless the result is `true`. This also future-proofs
the FTB path against later edits to the token's false-return branches.

### FTB-16 (Low) merkleroot silently self-consistent on skipped rows

`cmd/merkleroot` prints a stderr line and continues on an unparseable address or a negative
balance. Because the cap is set from the printed total (FTB-01), a skipped row produces a root
and cap that agree with each other but quietly exclude a holder, who can then never claim. The
aggregation fix (FTB-05) made duplicates safe; this is the remaining silent-exclusion edge.
Fix: hard-fail on any skipped row (or require an explicit `-allow-skip` flag) and print a
skip-count summary next to the totals.

### FTB-17 (Medium) two divergent contract trees in one workspace

`contracts/FTB_Token.cs`, `FTB_Migrator.cs`, `FTB_Lend.cs` and the root-level
`UnifiedToken_FTB.cs` are a second, older implementation labeled "SKELETON TEMPLATES, NOT
AUDITED". Worse, `contracts/README.md` specifies the migration leaf as
`sha256(account ++ amount-32LE)` — a different format from the Go reference
(`sha256(0x00 || account || ":" || decimal(amount))`, domain-separated per FTB-11). Anyone who
builds proofs to the C# spec against the Go migrator gets 100% claim failure; anyone who deploys
the C# skeletons gets unaudited code that predates every fix in the first pass. Fix: delete the
C# tree, or move it under an `archive/` folder with a top-of-file "superseded by ftb/, do not
deploy" note, and correct or remove the stale leaf-format line.

### FTB-18 (Info) client base58 validation

`ftb-dapi.js` `base58Decode` checks decoded length but never verifies the 4-byte base58check
checksum or the N3 version byte. A mistyped vote or config address of valid length passes and
sends the dust vote (or reads) to the wrong hash. Client-side only, no custody risk; add the
checksum check when a sha256 helper is available.

### FTB-19 (Info) reproducible builds

CI runs `go mod tidy` fresh on every run and the repo ships no `go.sum` (the workflow comment
acknowledges this). Commit the `go.sum` after the first green run so contract bytecode is
reproducible and dependency drift cannot slip between audits.

### FTB-20 (Info) underwater liquidation sizes

In an underwater position the close-factor lifts to full repayment while the seize is capped at
available collateral, so a liquidator can repay more than the collateral is worth and eat the
difference. Self-harm only — the protocol and suppliers benefit — but liquidator bots should size
`repay` against `collateralValue(borrower) * (1 + bonus)`, not against the debt. Worth one line
in the operator docs.

## Attack-vector deltas (new this pass)

| # | Vector | Target | Status |
|---|--------|--------|--------|
| 18 | Admin key compromise in the trusted window | token+migrator | Open: no multisig path in tooling (FTB-14); bound by cap (FTB-01) |
| 19 | External-token false-return desync | lend | Guarded by pre-checks today; harden `send` (FTB-15) |
| 20 | Snapshot exclusion via skipped CSV row | cmd/merkleroot | Open: silent-skip (FTB-16) |
| 21 | Proof-format mismatch from stale docs | migrator+merkle | Open: C# tree documents an incompatible leaf (FTB-17) |
| 22 | bnorm underflow via repay/liquidate rounding | lend | Blocked: inequality verified this pass |
| 23 | Same-tx share-price manipulation | lend | Blocked: poolValue invariant under borrow/repay, no donation path |

## Bottom line

The Launch 1 core remains sound and the first pass's fixes are genuinely in the code, with tests
named for each. Nothing new threatens the sealed-mint invariant. The two items worth acting on
before launch are FTB-14 (make the multisig owner real, since the docs already promise it) and
FTB-17 (remove the divergent C# tree and its wrong leaf spec). FTB-15 and FTB-16 are one-line
hardenings. The pre-launch gate from the first report stands, minus its external-audit leg
(withdrawn per DECISIONS.md Decision 7, 14 October 2026): set the cap from the merkleroot total,
close then renounce, real oracle quorum before any lend value, public TestNet with a bounty.

## Addendum: first real build (10 October 2026, local toolchain)

A Go toolchain (go1.27.2, neo-go compiler v0.123.0) was installed and `make` was run for the
first time. Important discovery: **the repo had never compiled against its own pinned neo-go
v0.123.0.** The first audit's caveat ("checked for structural balance but not compiled") was
well-founded — the interop API had drifted from what the code was written against. Fixes,
all mechanical API alignment, no logic changes:

- `crypto.Sha256` moved to `native/crypto` (migrator import).
- Storage find flags renamed: `FindKeysOnly|FindRemovePrefix` → `KeysOnly|RemovePrefix` (oracle).
- `gas.Hash` is now an untyped string constant; cast to `interop.Hash160` at five sites (lend).
- `management.Update(nef, manifest, data)` split; the token now calls `UpdateWithData`.
- `actor.Wait` → `WaitSuccess(ctx, ...)` (deploy, seed).
- neotest: `*neotest.Account` → `neotest.Signer` in four test files.
- Token manifest: wildcard permission entry now lists `update` (a separate contract-specific
  entry duplicates the compiler-inferred one).
- Mint events: `runtime.Notify("Transfer", nil, ...)` infers `Any` and conflicts with the
  NEP-17 manifest; emitted as `interop.Hash160(nil)` (token and the lend mocktoken), which
  still serializes as null at runtime.
- Lend manifest gained the `Configured`/`ParamsSet` events (FTB-09) the code already emitted;
  mocktoken manifest gained the `safemethods` NEP-17 requires.
- Tests: Buffer vs ByteArray expectation (registry), `[][]byte` proof args converted to
  `[]any` (migrator), and the oracle staleness test now mines one block — neotest advances
  timestamps by 1ms per block, so `now-ts = 1ms` was still within the inclusive 1ms bound.

Result: gofmt clean, `go vet` clean, all five contracts compile to NEF + manifest, and the
full suite passes — 28 tests across 6 packages, including the Merkle round-trip that locks
the off-chain and on-chain leaf formats byte for byte. `go.sum` is committed (FTB-19) and CI
now installs the compiler from its new path (`cli`, not `cli/neo-go`).

## Addendum 2: hardening and hot-path performance pass (10 October 2026)

Compiled, vetted, and tested locally (28/28). FTB-15 and FTB-16 fixed as above. The lend hot
path optimization deferred from the first pass is now in: the oracle hash is resolved once per
transaction (`priceFrom`/`collateralValueFrom`) instead of once per price read, collateral and
cash are read into locals instead of repeated storage/cross-contract reads in
`WithdrawCollateral`, `liquidateIn`, and `WithdrawSupply`, and `CollateralValue` shares the
single-resolution path. No logic changed; the full lend suite (10 tests, including liquidation
math and the close-factor) passes unchanged, which pins the behavior.

## Addendum 3: ballot contract — on-chain governance (10 October 2026)

A sixth contract, `contracts/ballot`, is now in the tree and **must stay in scope for any
future review** (the external-audit gate itself is withdrawn per Decision 7). It is the governance mechanism: deployed once per proposal, its hash is the
proposal identity. Design decisions an auditor should confirm:

- It reuses the migration Merkle tree verbatim — the same domain-separated leaf format
  (`sha256(0x00 || account || ":" || dec(amount))`, `sha256(0x01 || min || max)` sorted-pair
  nodes) that the migrator and the Python verifier already pin byte-for-byte. Voters prove
  snapshot weight with the exact proof they claim with; no second snapshot to attack.
- One vote per snapshot address (a voted marker stores the chosen option), CheckWitness on the
  voter, write-once owner-only `Configure(root, numOptions)` with 2–16 options. Re-votes,
  inflated weights, and non-snapshot addresses all abort.
- Zero permissions in the manifest: the ballot calls no other contract. Tallies are chain
  state, so every full node counts the identical result — this is what makes the vote
  verifiable between nodes rather than by a server.
- The neotest suite covers the weighted flow end to end (proof built with `pkg/merkle`),
  double-vote rejection, inflated-weight rejection, out-of-range option, non-holder rejection,
  and the configure guard rails. Full repo: gofmt clean, vet clean, all 6 contracts compile,
  30 tests pass across 7 packages.
