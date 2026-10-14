# FTB first audit report

Date: 9 October 2026. Scope: the `ftb/` repo (token, migrator, registry, oracle, lend, the
`pkg/merkle` builder, and the `cmd/` tools). The reused Flamingo swap is external and out of
scope. Method: two independent passes, one by the author and one by a separate reviewer that
read the code cold, then reconciled. This is a first internal audit. (Note, 14 October 2026:
per DECISIONS.md Decision 7 the external-audit gate this line referenced is withdrawn; this
internal lineage is now the audit posture, recorded in the open.)

## Verdict

The one invariant holds in code: FTB is created in exactly one place, `MintForMigration`, behind
three gates (caller is the migrator via `GetCallingScriptHash`, migration still open, under the
cap), with no external call inside the mint, an immutable migrator, and one claim per address.
Both passes traced every writer of `keySupply` and found no unauthorized or reentrant path that
mints outside that function, past `CloseMigration`, or above the cap. Checks-effects-interactions
is followed everywhere, and the `contract.ReadOnly` flag on price and registry reads genuinely
blocks reentrant writes from a called module.

The honest caveat that frames everything below: the invariant is unconditional only *after*
`CloseMigration` and `RenounceOwnership` have both run. Until then the owner multisig is a trusted
mint authority bounded only by the cap. So the cap value and the close-then-renounce sequence are
the two things the whole design rests on, and both now have code support they did not before.

Fourteen findings: 1 High, 6 Medium, 4 Low, 3 Info. Eight are now fixed in code, two are partially
addressed (the cap FTB-01 and the mint-authority window FTB-02), and the rest are owner or deployment
decisions captured in `DECISIONS.md`. The fixes are guards, events, bounds, and Merkle domain
separation; the architecture is unchanged.

## Findings

| ID | Sev | Area | Title | Status |
|----|-----|------|-------|--------|
| FTB-01 | High | token | `maxSupply` is a placeholder not bound to the snapshot total | Mitigated, human action required |
| FTB-02 | Med | token+migrator | Mint authority and root mutable during the open window | Partially fixed (root now write-once) |
| FTB-03 | Med | token | `RenounceOwnership` had no "close first" interlock | Fixed |
| FTB-04 | Med | cmd/seed | Seeds with `min=0` without checking the pool is empty | Fixed |
| FTB-05 | Med | cmd/merkleroot | No dedup of accounts, no reject of negative balances | Fixed |
| FTB-06 | Med | oracle (L2) | Oracle admin not timelocked; `minReports` default 1 | Recommended |
| FTB-07 | Med | lend (L2) | No liquidation close-factor; open-ended params | Fixed |
| FTB-08 | Low | lend (L2) | `supplyIn` lacked `shares>0` / `pre>0` guards | Fixed |
| FTB-09 | Low | lend (L2) | `Configure` / `SetParams` emitted no events | Fixed |
| FTB-10 | Low | lend (L2) | Oracle pause trapped debt-free users' collateral | Fixed |
| FTB-11 | Low | migrator+merkle | Merkle leaves and nodes not domain-separated | Fixed |
| FTB-12 | Low | cmd/deploy+seed | Plaintext password in config; `Accounts[0]` assumed | Recommended |
| FTB-13 | Info | lend/registry/oracle | Accrual direction, rounding dust, sentinels, orphan keys | Noted |

### FTB-01 (High) maxSupply placeholder
`ftb_token.go` ships `maxSupply = 500_000_000 * one` with a note to set it from the snapshot. It is
a compile-time constant, so the deploy tool cannot set it, and nothing cross-checked it against the
sum of entitlements. If the real total is below it, the trusted window has mint headroom; if above,
honest claims hit `panic("exceeds cap")`. Done this pass: added a `MaxSupply()` view so the cap is
verifiable on-chain, and `cmd/merkleroot` now prints the exact total to set with an FTB-01 note.
Still required by a human: set the constant to that exact total before compiling for launch, and
publish both numbers next to `proofs.json`. Consider a `make` check that the constant equals the
merkleroot total.

### FTB-02 (Medium) mutable mint authority in the open window
Pre-renounce, the owner could re-point the token's migrator (`SetMigrator`) or overwrite the
migrator's root (`Configure`) and mint up to the cap. Done this pass: `ftb_migrator.Configure` is
now write-once (a second call reverts), so the published root cannot be silently swapped; to change
it you redeploy and re-point before finishing. Residual: `SetMigrator` is still re-pointable until
renounce, bounded by the cap (FTB-01) and by closing and renouncing promptly. Minimize that window
operationally and treat it as the documented trust period.

### FTB-03 (Medium) renounce-before-close
`RenounceOwnership` emptied the owner unconditionally. Renouncing before closing would make
`CloseMigration` permanently unreachable with minting still open, unrecoverable. Fixed: it now
reverts unless `keyOpen == 0`, enforcing close-then-renounce as a real ordered one-way door.

### FTB-04 (Medium, Launch 1) seed front-run
`cmd/seed` added liquidity with `min=0`, valid only on an empty pool, but never checked. A
front-runner could pre-seed a skewed ratio and skim the protocol's deposit. Fixed: seed now reads
`getReserves` first and refuses a non-empty pool unless `-force` is passed, with a loud reason.

### FTB-05 (Medium) snapshot builder validation
`cmd/merkleroot` appended one leaf per CSV row with no dedup and no rejection of negatives, so a
repeated or re-encoded address made a dead second leaf and overstated the cap total, and a negative
balance could distort an entitlement. Fixed: balances are now aggregated per account (so duplicates
collapse into one leaf) and negative balances are rejected with a warning.

### FTB-06 (Medium, Launch 2) oracle governance
The registry timelock protects swapping the oracle contract, but the oracle's own `SetReporter` and
`SetParams` take effect immediately, and `minReports` defaults to 1, so a fresh or compromised
oracle is single-source, the exact risk it exists to remove. Recommended (needs a governance call,
and would touch tests, so not auto-applied): make the oracle owner the same timelocked multisig as
the registry, enforce a `minReports` floor of 3 or more, and ship with a real quorum.

### FTB-07 (Medium, Launch 2) liquidation and parameter bounds
A liquidator could repay the full debt in one call (no close factor), and `SetParams` bounded
ordering but not the bonus or the rate. Fixed this pass: `SetParams` caps the liquidation bonus at
30 percent and the per-block rate at `maxRate` (1 percent), and `liquidateIn` applies a close-factor
of half the debt unless the position is underwater (collateral worth less than the debt), where full
repayment is allowed to clear bad debt. The close-factor percentage and the ceilings are tunable
constants, and tests cover the bonus bound and the close-factor.

### FTB-08 to FTB-10 (Low, Launch 2) fixed this pass
FTB-08: `supplyIn` now requires `pre > 0` and `shares > 0`, closing a divide-by-zero in an
insolvency state and a silent zero-share loss for a tiny supplier (the classic donation inflation is
already blocked because `OnNEP17Payment` rejects unrecognized FTB inflows). FTB-09: `Configure` and
`SetParams` now emit events, matching the "every power emits an event" rule the other modules keep.
FTB-10: `WithdrawCollateral` now skips the oracle-dependent health check when the user has no debt,
so a paused oracle cannot trap a debt-free user's collateral.

### FTB-11 (Low) Merkle domain separation
Leaves are `sha256(acct || ':' || dec(amount))` and internal nodes are `sha256(32||32)` with no
leaf-versus-node tag, the textbook second-preimage shape. Not exploitable here, because a claimant
must pass `CheckWitness(account)` and the leaf is built by the contract from the caller's own account
and a positive amount. Fixed this pass in lockstep: leaves are now tagged `0x00` and internal nodes
`0x01` on both `pkg/merkle` and the on-chain `Claim`. The scheme was checked consistent for odd and
even trees, and the real-VM round-trip test is the safety net that keeps the two sides in agreement.

### FTB-12 (Low) operator key handling
`cmd/deploy` and `cmd/seed` read the wallet password from plaintext `config.json` and use
`Accounts[0]`. `.gitignore` correctly excludes the filled configs and wallets, so this is operator
hygiene, not a repo leak. Recommended: read the password from an environment variable or a prompt,
and let the operator pin the account label.

### FTB-13 (Info)
The `AUDIT.md` brief has been corrected: linear `rate * blocks` accrual *understates* compound
interest across long idle gaps (it approximates compounding when accrue runs most blocks), it does
not overstate it. Minor rounding dust in borrow and repay favors the pool or is sub-unit. The
registry `Apply` uses `eta == 0` as "nothing pending", safe only because `FinishBootstrap` requires
a non-zero delay. Oracle `SetReporter(false)` leaves orphaned price and time entries (harmless, see
weight below).

## Attack-vector catalog

| # | Vector | Target | Status |
|---|--------|--------|--------|
| 1 | Unauthenticated or reentrant mint | token | Blocked: calling-hash gate, no external call in mint |
| 2 | Mint after close | token | Blocked: `keyOpen` gate, one-way close |
| 3 | Mint above cap | token | Blocked in code; cap value must be set (FTB-01) |
| 4 | Owner re-points minter or root within the cap | token+migrator | Narrowed: root write-once; `SetMigrator` window remains (FTB-02) |
| 5 | Renounce before close bricks the seal | token | Fixed: interlock (FTB-03) |
| 6 | Double claim, claim another's, claim an unlisted amount | migrator | Blocked: witness + proof + one-shot marker |
| 7 | Merkle proof forgery or second-preimage | migrator+merkle | Not exploitable (witness-bound); domain tags added (FTB-11) |
| 8 | On-chain vs off-chain leaf drift (BE/LE) | migrator+merkle | Blocked: locked by a real-VM round-trip test |
| 9 | NEP-17 reentrancy on transfer | token, lend | Safe: checks-effects-interactions, witness-constrained |
| 10 | Donation / first-depositor share inflation | lend | Donation blocked at the callback; rounding edge guarded (FTB-08) |
| 11 | Reentrancy via price read into lend writes | lend to oracle | Blocked: `ReadOnly` call flag forbids callee writes |
| 12 | Fake-token deposit spoofing `onNEP17Payment` | lend | Blocked: calling-hash must equal the real token |
| 13 | Single-reporter or stale price manipulation | oracle | Mitigated by median+quorum+staleness; harden defaults (FTB-06) |
| 14 | Initial-seed front-running | cmd/seed | Fixed: empty-pool guard (FTB-04) |
| 15 | Snapshot CSV poisoning (dupes, negatives) | cmd/merkleroot | Fixed: aggregate + reject (FTB-05) |
| 16 | Registry timelock bypass or zero-delay lock-in | registry | Blocked: non-zero delay required, monotonic increase |
| 17 | Storage prefix collision or `append` aliasing | all | None found: prefixes distinct, fresh buffers |

## Optimize: security

The posture is sound and is summarized in `AUDIT.md` for the external reviewer. The strongest
properties are the single sealed mint path, the no-donation callback, the `ReadOnly` reentrancy
guard, and the oracle that pauses (reverts) rather than trading on too few prices. This pass added
guards, events, and input validation without touching the architecture. The remaining security work
is governance (FTB-06), the liquidation close-factor (FTB-07), and setting the cap (FTB-01), all
gated to Launch 2 or to a human step before launch.

## Optimize: weight (storage and bytecode)

Already tight: single-byte storage prefixes, balance keys deleted on zero, a one-byte claim marker,
no oversized constants. One cleanup worth doing in the Launch 2 oracle pass: delete the orphaned
`priceKey` and `timeKey` entries when a reporter is disabled in `SetReporter(false)`, so a rotating
committee does not leave unbounded dead entries. Everything else is minimal.

## Optimize: speed and performance (compute and gas)

The hot path is `lend`, and it is where compute can be trimmed in the Launch 2 hardening pass (left
as recommendations now so this pass stays safety-only and low-risk):
reduce repeated cross-contract calls by reading `cash()` once into a local where a method uses it
more than once; resolve the oracle hash once per transaction instead of on every `price` read, since
each read currently goes through the registry and then the oracle; and read each needed price once in
liquidation rather than through `IsUnhealthy` and then again explicitly. Keep the oracle committee
small, since `Price` iterates all reporters with two storage reads each on every borrow-side call.
The launch core (token, migrator, registry) is already lean and has no comparable hot loop.

## Optimize: modularity

The registry seam is the right pattern and should stay: `lend` resolving the oracle by name behind a
timelock, and the token being deliberately reference-free and immutable. Three improvements, mostly
Launch 2: bind peripheral admin to the same timelocked multisig as the registry so each module's own
admin is not an un-timelocked side door (FTB-06); keep `pkg/merkle` as the single source of truth for
the leaf format (it already is, imported by both the CLI and the on-chain cross-check test) and add
the domain tag there so both sides move together (FTB-11); and keep risk parameters (`SetParams`)
separate from wiring (`Configure`), which they are, now that both emit events. The write-once root
(FTB-02) brings the migrator closer to the same seam discipline as everything else.

## What is solid

One sealed mint path with three gates and no external call inside it; an immutable migrator and
one-shot claims; checks-effects-interactions across the token and every lend mutation; the
`ReadOnly` flag preventing reentrant writes from a called module; the no-donation property of the
lend callback neutralizing the standard inflation attack at the root; median plus quorum plus
staleness-revert in the oracle with pause as the safe failure mode; the registry one-way doors;
byte-order agreement locked by a real-VM round-trip test rather than by assertion; no storage prefix
collisions and no `append`-aliasing foot-guns; internally consistent units across borrow,
collateral, seize, and health math with rounding that favors the pool on custody paths; and a
`.gitignore` that keeps wallets and filled configs out of the repo.

## Pre-launch gate

Set `maxSupply` to the exact merkleroot total and publish both (FTB-01). Run the migration window,
then close, then renounce, in that order (now enforced, FTB-03). Set a real oracle quorum before any
lend value touches the oracle (FTB-06). The liquidation close-factor, bonus cap, and rate ceiling
are set in code (FTB-07); see `DECISIONS.md` for the two owner decisions, the cap value and oracle
governance. Then the external gate: two independent audits of the Launch 1 core, a separate audit of
oracle plus lend, and a public TestNet with a bug bounty. Lending stays out of the first launch.

## Verifying this pass

These edits were checked for structural balance and consistency but not compiled here (no Go
toolchain in this environment). Before relying on them, run `make` (gofmt, vet, compile, and the
full neotest suite).

The guard tests are now written and ship with this pass: a `MaxSupply` view check and a
renounce-before-close revert (`contracts/ftb_token/ftb_token_test.go`), a second-`Configure` revert
(`contracts/ftb_migrator/ftb_migrator_test.go`), and a liquidation-bonus bound, a zero-share supply
revert, and a debt-free collateral withdrawal during an oracle pause (`contracts/lend/lend_test.go`).
The existing renounce test was updated to close first, since the interlock changed its behavior.
This pass also added lend tests for the close-factor, multiple suppliers, the utilization lock,
collateral health on withdraw, and interest accrual over blocks, and the Merkle domain-separation
change was checked for scheme consistency (odd and even trees) outside the VM and is covered by the
round-trip test. Run them with `go test ./...`. The off-chain `cmd/seed` empty-pool refusal and the `cmd/merkleroot`
aggregation and negative-rejection are logic inside `main()`; they are covered by review here and
are worth a small refactor into testable functions in a later pass.
