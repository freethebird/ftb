# FTB — Free the Bird

**A community relaunch of Flamingo-style DeFi on Neo N3.** Trade, earn, borrow, and
govern — the mechanism lives 100% on-chain, the front end is a static page served from
IPFS by community nodes, and after launch there are no admin keys at all.

Three promises, enforced in code and readable by anyone:

- **No mint beyond the hard cap.** The only mint is the migration claim, then it seals
  forever. FTB is dividend-bearing, never inflationary.
- **No fiat oracle.** Prices on the front end are ratios between assets — nothing to
  manipulate, nothing to trust.
- **No single point of control.** Governance, the treasury, and even the website run on
  contracts and community nodes. A gift to the Flamingo community.

One language end to end: Go, via [neo-go](https://github.com/nspcc-dev/neo-go) (contract
compiler, SDK, and node). The audited Flamingo swap is reused as a deployed NEF and called
by hash — battle-tested code, and no second language ever enters the repo.

> **Status: pre-TestNet.** Reference implementation — internal audit lineage only, no
> external audit was performed (Decision 7) — see [Safety](#safety) before trusting value.

## The documents

| you want… | read |
|---|---|
| the one-page "what is this" | `OVERVIEW.md` |
| the architecture and the seams | `ARCHITECTURE.md` + `docs/STACK_MAP.md` |
| fees, rates, APY, the 2/3 : 2/9 : 1/9 constitution | `ECONOMICS.md` |
| proposal → vote → code lifecycle | `GOVERNANCE.md` |
| every user action → the contract it invokes → who published it | `USECASES.md` |
| run one of the five community nodes | `ops/NODE_OPERATOR_RUNBOOK.md` |
| audit the code | `AUDIT.md`, then `AUDIT_REPORT.md` / `AUDIT_REPORT_PASS2.md` / `AUDIT_MOV8_QRN_V1.md` |
| build any client against the contracts | `web/invocations.md` |
| extend the front end without breaking the discipline | `web/UI_RATIONALE.md` |

## Build and test in one command

The whole audit runs with `make`:

```
make tools   # once: installs the neo-go compiler (also the SDK and node)
make         # gofmt check, go vet, compile every contract, run every test
```

`make conformance` runs the standing guarantee in one gate: all of the above plus the race
detector, `govulncheck` against the pinned module set, and the NEF-hash record/verify step
(`scripts/nef_hash.sh`; record a release with `cp build/NEF_HASHES.sha256 NEF_RELEASE.sha256`).

CI (`.github/workflows/ci.yml`) runs the exact same steps on every push and pull request,
so nothing merges unless it compiles and all tests pass.

## Status

- [x] Token: `ftb_token` — capped NEP-17, mint only by the Migrator, sealable,
      renounceable, full neotest suite.
- [x] Migration: `ftb_migrator` + `pkg/merkle` + `cmd/merkleroot`, with an on-chain claim
      test proving the off-chain and on-chain leaf formats agree byte for byte.
- [x] Registry seam (multisig + timelock) + the Go deploy/wire driver.
- [x] Liquidity seeding: `cmd/seed` driver + the swap/seed runbook.
- [x] Oracle (reporter-median, staleness-safe) + lend (no-mint supply/borrow, native
      NEO+GAS collateral, kinked utilization curve), oracle resolved via the registry seam.
- [x] Ballot: on-chain snapshot-weighted governance — one contract per proposal, the
      contract hash IS the proposal id, voters reuse their migration Merkle proof as
      weight, tallies in chain state so every node counts the same result.
- [x] Governance economy: `bondvault` (listing bonds locked while a project is listed),
      `treasury` (proposal escrow — auditor-attested NEF hash, tranche payouts, the fund is
      a contract, not a wallet), `dividends` (xSUSHI-style distributor paying FTB stakers
      the holders' 2/9 of every fee). Fee constitution: 2/3 LPs, 2/9 holders, 1/9 treasury.
- [x] Front end: static, dependency-free, ratio-only pricing (no fiat oracle), served from
      IPFS by community pinning nodes (`ops/NODE_OPERATOR_RUNBOOK.md`).
- [ ] Launch 1: token + migration + reused swap, protocol-owned liquidity (on TestNet, then
      MainNet).
- [ ] Launch 2: enable `oracle` + `lend` on MainNet, only after their own dedicated
      internal audit pass. Deliberately not part of Launch 1.

## Layout

```
ftb/
  ARCHITECTURE.md
  go.mod
  contracts/
    ftb_token/
      ftb_token.go          # the contract
      ftb_token.yml         # manifest config for neo-go
      ftb_token_test.go     # neotest suite (every invariant, every guarded path)
      testdata/caller/      # tiny helper contract to exercise the migrator-only mint
```

## Build a contract

```
# install the compiler (ships with neo-go)
go install github.com/nspcc-dev/neo-go/cli/neo-go@latest   # or download the release binary

# compile to NEF + manifest
neo-go contract compile -i contracts/ftb_token/ftb_token.go \
  -c contracts/ftb_token/ftb_token.yml \
  -o ftb_token.nef -m ftb_token.manifest.json
```

## Run the tests

```
go mod tidy
go test ./...
```

The suite spins up an in-memory chain with neotest and checks: metadata and zero initial
supply, owner-only admin, mint only by the Migrator, the hard cap, that CloseMigration
seals minting forever, that transfers need a witness and move balances correctly, and that
RenounceOwnership is final.

## Deploy and wire

Compile each contract into `build/`, then use the Go driver or the neo-go CLI.

```
mkdir -p build
for c in ftb_token ftb_migrator registry; do
  neo-go contract compile -i contracts/$c/$c.go -c contracts/$c/$c.yml \
    -o build/$c.nef -m build/$c.manifest.json
done
```

Build the migration root from the published snapshot:

```
go run ./cmd/merkleroot -snapshot snapshot.csv -r1 1 -r2 4 -out proofs.json
```

Option A, the Go driver (one language end to end):

```
cp cmd/deploy/config.example.json cmd/deploy/config.json   # fill rpc, wallet, root, timelock
go run ./cmd/deploy -config cmd/deploy/config.json
```

Option B, the neo-go CLI (stable, well-worn surface):

```
neo-go contract deploy -r $RPC -w wallet.json -in build/ftb_token.nef \
  -manifest build/ftb_token.manifest.json -- $OWNER
# repeat for ftb_migrator and registry, each with  -- $OWNER
neo-go contract invokefunction -r $RPC -w wallet.json <tokenHash>    setMigrator <migratorHash> -- $OWNER
neo-go contract invokefunction -r $RPC -w wallet.json <migratorHash> configure   <tokenHash> <rootBytes> -- $OWNER
# registry: setInitialDelay, bootstrap token/migrator, finishBootstrap
```

Order and gates are in `ARCHITECTURE.md`. The registry governs module swaps behind a
timelock; the token is sealed and immutable. Set `owner_multisig` in the deploy config so the
contracts are born with the multisig as owner (FTB-14), run the migration window, then
multisig-sign `migrator close` to seal the mint and `renounceOwnership` after it, in that
order. The full multi-operator flow, including the TestNet rehearsal, is in
`ops/DECENTRALIZED_OPS.md`.

## Launch 1: stand up the swap and seed liquidity

The swap is Flamingo's audited Factory, Pair, and Router, reused as compiled NEF. Deploy it
per Flamingo's repo, then create and whitelist the three pairs:

- deploy Factory, PairWhiteList, Router, and one Pair per pool;
- whitelist each Pair, then `Factory.createExchangePair(tokenA, tokenB, pairHash)`;
- point the Router at the Factory and whitelist.

NEO has 0 decimals, so wrap the NEO you intend to seed into bNEO (NeoBurger) first. Then
seed protocol-owned liquidity:

```
cp cmd/seed/config.example.json cmd/seed/config.json   # fill router, FTB hash, amounts
go run ./cmd/seed -config cmd/seed/config.json
```

It calls `Router.AddLiquidity` for bNEO/FTB, GAS/FTB, and bNEO/GAS with min amounts 0 (the
first LP sets the opening price), then reads `GetReserves` back to confirm each pool took
the funds. The neo-go CLI equivalent per pool:

```
neo-go contract invokefunction -r $RPC -w wallet.json <router> addLiquidity \
  <sender> <tokenA> <tokenB> <amtA> <amtB> 0 0 <deadlineMs> -- <sender>
```

After seeding, do a small manual swap to confirm pricing, run the full flow on TestNet with
community testers and a bug bounty, and only then consider MainNet.

## Lending and oracle (Launch 2, separately reviewed)

Written to the same standard, but deliberately not part of the first launch, because lending
is the highest-risk primitive and a lend-side bug is what destroyed Flamingo.

- `contracts/oracle` is a reporter-committee price feed: a whitelist of reporters each submit
  a price, and `Price` returns the median of the fresh ones, requiring a quorum. A single
  compromised or stale reporter cannot move it, and too few fresh prices makes `Price` revert,
  which safely pauses borrowing rather than trading on a bad number. It never reads a single
  spot AMM price. Reporters are a trust assumption; run several independent ones.
- `contracts/lend` is a no-mint, bank-style market: suppliers deposit FTB and earn interest,
  borrowers post bNEO/GAS collateral and borrow from what suppliers deposited, with an LTV
  cap, liquidation, and cToken-style supply shares. It resolves the oracle through the
  registry seam, so the oracle can be upgraded behind the timelock without touching lend.

These ship only after their own dedicated internal audit pass — a hardened multi-source
oracle and far more exhaustive tests (interest over time, rounding, partial liquidations) —
before any value.

## The one rule, enforced in code

No reward or interest path mints FTB. The only mint is `MintForMigration`, callable only by
the Migrator, only while open, only under the cap, then sealed. Rewards come from providing
liquidity in other modules, never from printing.

## Safety

Reference implementation. Internal audit lineage only - no external audit was performed
(DECISIONS.md, Decision 7); verify before trusting value, and use the public TestNet and bug
bounty. Lending ships later, on its own, because it is the highest-risk module.

## License

MIT (see `LICENSE`), copyright the Free the Bird community — matching the reused Flamingo
swap and neo-go, both MIT.
