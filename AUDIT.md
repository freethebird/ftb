# FTB auditor brief

One page for a reviewer. It says what to verify first, where to look, and what we already
know is risky. Line numbers drift; names do not, so each item names the file and function.

Scope: `contracts/ftb_token`, `contracts/ftb_migrator`, `contracts/registry`,
`contracts/oracle`, `contracts/lend`, and the off-chain tools in `cmd/` plus `pkg/merkle`.
Toolchain: neo-go v0.123.0, Go 1.26. NeoVM integers are arbitrary-precision (256-bit bound),
so arithmetic faults rather than wraps; still confirm the large products below stay in range.
Launch 1 is token, migrator, registry, and swap wiring. `oracle` and `lend` are Launch 2,
behind their own separate audit, and are not reached by real value in the first launch.

## The one invariant, verify this before anything else

FTB is created in exactly one place: `ftb_token.MintForMigration`. Confirm there is no other
write that raises `keySupply`, and that all three gates hold there:

1. caller is the registered migrator: `GetCallingScriptHash() == keyMigrator`. The migrator
   calls it directly (`ftb_migrator.Claim` does `contract.Call(ftb, "mintForMigration", ...)`),
   so the calling hash is the migrator's. Confirm nothing else is ever set as migrator except
   the owner via `SetMigrator`, and that `SetMigrator` becomes unreachable after
   `RenounceOwnership`.
2. migration is open: `keyOpen != 0`. `CloseMigration` sets it to 0 permanently.
3. under the cap: `supply + amount <= maxSupply`.

`lend` must never mint. Confirm the comment in `lend` holds in code: supply comes only from
deposited FTB, `Borrow` sends from `cash()` (the contract's own balance), and `accrue()`
raises the borrow index and `totBorrow` without any call to `mintForMigration`.

## The one-way doors (irreversible by design, confirm each is truly one-way)

- `ftb_token.CloseMigration` sets `keyOpen = 0`. No path sets it back to 1.
- `ftb_token.RenounceOwnership` sets `keyOwner` empty, after which `requireOwner` always
  panics, so `SetMigrator`, `Update`, and `CloseMigration` are all dead.
- `registry.FinishBootstrap` flips `keyFinished` to 1 and requires `keyDelay > 0` first, so a
  zero timelock can never be locked in. After it, only `Propose` then `Apply` works.
- `registry.IncreaseDelay` rejects any value not strictly greater than the current delay.

## The seams (upgrade path, confirm the token is NOT upgradeable this way)

`registry` maps a name to a module hash. After bootstrap, a change is `Propose` (owner), wait
`Delay()` blocks in the open, then `Apply` (anyone). `lend` resolves the oracle through
`registry.get("oracle")` on every price read, so swapping the oracle is itself timelocked.
Confirm the registry can never change the token's code or mint path; the token seals itself and
is meant to be immutable after `RenounceOwnership`. The token's `Update` (via
`management.Update`) exists only for pre-launch fixes and must be renounced before value flows;
confirm on-chain that `keyOwner` is empty after launch.

## Access control, confirm the witness checks

- token: owner multisig does `SetMigrator`, `Update`, `CloseMigration`, `RenounceOwnership`;
  migrator contract does `MintForMigration`; `Transfer` needs `CheckWitness(from)`.
- migrator: owner does `Configure`, `Close`; `Claim` needs `CheckWitness(account)` and a valid
  proof, so a caller can only claim their own entitlement, once (`prefixDone`). Note both close
  paths need the token owner's witness on the transaction: `migrator.Close` calls the token's
  `closeMigration`, which runs `requireOwner` against the token owner, so the owner must co-sign.
- registry: owner does `Bootstrap`, `SetInitialDelay`, `FinishBootstrap`, `Propose`, `Cancel`,
  `IncreaseDelay`; anyone does `Apply` after the timelock.
- oracle: owner does `SetReporter`, `SetParams`; a whitelisted reporter does `Report`.
- lend: owner does `Configure`, `SetParams`; users act through `OnNEP17Payment`
  (supply, repay, liq) and the direct `Borrow`, `WithdrawSupply`, `WithdrawCollateral`.

## The migration Merkle check (the classic place this breaks)

The leaf must be byte-identical on both sides: `ftb_migrator.Claim` builds
`sha256(account ++ ":" ++ std.Itoa(amount, 10))`, and `pkg/merkle` plus `cmd/merkleroot`
must build the same. The known failure is account byte order: `pkg/merkle.accountBytes` uses
`BytesBE` with a note to switch to `BytesLE` if the cross-check test reports a bad proof.
Confirm `ftb_migrator_test.go` (which imports `pkg/merkle` on purpose) locks this agreement and
passes. Confirm sorted-pair hashing matches: on-chain `lessBytes` is unsigned big-endian, which
must equal the off-chain `bytes.Compare` order. Confirm the attacker is absent from the snapshot
by construction (no leaf, so no valid proof), and that `cmd/merkleroot` computes each
entitlement as `floor(flm*r1 + flocks*r2)`.

## Set these before launch (they ship as deliberate placeholders)

- `ftb_token.maxSupply` is `500_000_000 * one` with a `>>>` note. Set it to the exact sum of all
  snapshot leaves so every honest claim fits and no mint headroom exists beyond the entitlements.
- `oracle` `keyMinReports` defaults to 1 with a `>>>` note. Set to a real quorum (3 or more) or a
  single reporter can move the price, which is the exact failure the oracle exists to prevent.
- `lend` `keyRate` defaults to 0 (no interest) and `keyLTV`/`keyLiq`/`keyBonus` are reference
  values; set real ones. `lend` is Launch 2 regardless.

## Known risks to harden (we are flagging these, not hiding them)

- lend first-depositor / share rounding: `supplyIn` prices shares as `amount * ts / pre`. A naked
  FTB donation is blocked because the token's `Transfer` calls `OnNEP17Payment`, which panics on
  an unrecognized action, so funds cannot enter outside the share accounting. The residual edge
  is a small supplier rounding to zero shares when the share price is already very high, and a
  divide-by-zero in an insolvency state (shares outstanding, pool value zero). Both are now
  guarded in `supplyIn` (require `pre > 0` and `shares > 0`, audit item FTB-08); confirm the
  guards and consider seeding dead shares at deploy.
- lend liquidation has no close-factor cap: a liquidator may repay up to the full debt in one
  call. Confirm this is intended rather than the usual partial cap.
- lend depends on a live oracle: `price` reverts when fresh reports are too few, which pauses
  borrowing, borrowing-affected withdrawals, and liquidations. A prolonged oracle outage can let
  bad debt sit. Confirm this tradeoff is acceptable and the pause is the intended safe mode.
- reentrancy: every lend mutation updates storage before `send()` (checks-effects-interactions),
  and `send` to a user contract can call back through that token's `onNEP17Payment`. Confirm no
  cross-function reentrancy profits from the already-committed state.
- interest model: `accrue` uses linear `rate * blocks`, which understates compound interest across
  long idle gaps (it approximates compounding when accrue runs most blocks). Confirm the
  intermediate products (`bi*factor`, `amount*scale`, `tb*factor`) stay within the 256-bit bound
  for the chosen `rate` and realistic block gaps.

## How to run

```
make tools   # neo-go compiler, SDK, node
make         # gofmt, vet, compile every contract, run every neotest suite
```

Each contract has a neotest suite; `cmd/merkleroot` has the leaf cross-check. Nothing should
reach real value until two independent audits of the Launch 1 core, a separate audit of oracle
plus lend, and a public TestNet with a bug bounty have passed. See `ARCHITECTURE.md` for the
module and seam design and `README.md` for the deploy and seed runbooks.
