# LP Salvage - FLM locked in Flamingo pools at the snapshot

Date: 2026-10-16 · Method: state-root walk (same technique as `snapshot_state.py`) plus
transfer-log reconstruction. Snapshot block 12,782,704 (ts 1,788,022,068,695 ms).

## Bottom line

**164,197,790.77 FLM** sat inside 22 Flamingo swap-pair contracts at the snapshot.
It is not "contract-owned" - it belongs pro-rata to the liquidity providers.
**164,197,790.59 FLM (99.9999%) is attributable to 1,351 real wallets** and is
salvageable. Only 0.18 FLM maps to the null address (burned minimum-liquidity).

**Owner decision (2026-10-16):** the top staker, `Nd6YqmPtddyieyyaMx3xGi2Cugn3ZiMRcN`
(38,193,988.59 FLM), is moved to **dev-wallet custody on the same terms as #15** -
claimable by anyone who proves ownership, otherwise to the Governance proposal
wallet. Claim-eligible salvage is therefore **126,003,802.00 FLM across 1,350
wallets**.

- Final list: `lp_salvage_final.csv` (1,350 wallets, custody wallet removed, sorted by share)
- Custody entry: 38,193,988.59 FLM (evidence stays in `lp_salvage_l2.csv`)
- Evidence: `lp_salvage_detail.csv` (layer 1, per pair), `lp_salvage_l2.csv` (layer 2, per stake)
- Code: `lp_salvage.py`, `lp_salvage_l2.py` (fully rerunnable)

## How it was computed

**Layer 1 - direct LP tokens.** Every FlamingoSwapPair is itself an NEP-17 LP token.
Its balance storage at the snapshot state root uses keys `asset<20-byte scripthash>`
(discovered by probing; verified against the null-address minimum-liquidity entry).
Each holder's FLM share = `lp_balance / lp_total × pair FLM balance`. Result:
818 wallets, 467,601.18 FLM. But 99.7% of pool FLM (163,730,189.41) was held by
just four contracts: **Staking** (147.74M), **fusd-lp-staking** (15.73M), and two
**SpecialStaking** pools (0.26M).

**Layer 2 - unstaking the staking contracts.** Staking positions were reconstructed
from NEP-17 transfer logs: LP tokens in minus LP tokens out per user, per pool, up to
the snapshot block, converted at the same per-pool ratios. Result: 1,063 stakers,
163,730,189.41 FLM - an **exact match** with the contracts' LP holdings from layer 1
(to the last unit), which cross-validates both layers.

**Filters applied (owner rule):** the 16 flagged wallets, the attacker wallets, and
the #15 custody wallet are excluded from salvage. None of them held any LP position -
flagged/excluded LP total: 0.18 FLM (null address dust). Contract detection over all
1,351 final wallets: **zero contracts** - every recipient is a real wallet.

## The pairs (FLM at snapshot)

| Pair | FLM locked |
|------|-----------:|
| FLM-bNEO | 118,238,797.53 |
| FLM-FUSD | 16,250,147.82 |
| FLM-GAS | 7,112,355.22 |
| FLM-fWBTC | 5,991,167.71 |
| FLM-WBTC | 5,603,192.50 |
| FLM-fWETH | 5,178,853.38 |
| FLM-fUSDT | 4,115,353.42 |
| 15 smaller pairs | 1,707,923.19 |

## Top stakers (attestation candidates, like #2)

| Wallet | FLM share |
|--------|----------:|
| Nd6YqmPtddyieyyaMx3xGi2Cugn3ZiMRcN | 38,193,988.59 |
| NWz12JwiyatnMNWZwojjE4ADCAdePdp5LK | 13,996,324.96 |
| NP1f9cPmUdFH2x4BNtdhTgwV7bKS6KzfaG | 8,571,968.46 |
| NXKQ2ku8c4QN9hTJq5AcnNo65fpqQ1nZNt | 7,455,193.08 |
| NcdEfm31z42Bbgw8aqAdcTQ8AwBnhZ335b | 6,597,440.59 |

The top wallet alone is 23% of all pool FLM. None of the top stakers is on the
flagged list; all are wallets (contract-checked).

### Profile: Nd6YqmPtddyieyyaMx3xGi2Cugn3ZiMRcN (#1 staker, verified 2026-10-17)

**Status: FLAGGED TO DEV-WALLET CUSTODY (owner decision 2026-10-16), same terms
as #15 - claimable on proven ownership, else to the Governance proposal wallet.**

- Single position: 483,312.03 LP in FLP-FLM-bNEO (32.3% of that pool), staked in
  the Staking contract → 38,193,988.59 FLM (23.3% of all pool FLM).
- 4,372 NEP-17 transfer legs; first activity 2022-01-02, last 2026-10-06 -
  active in 56 of 58 months since January 2022 (silent only 2025-06, 2026-09).
  ~3,168 legs are mint/reward events (null counterparty): years of FLM
  farming claims.
- 38 counterparties, almost all protocol contracts: Staking (184 legs),
  FlamingoBroker (89), FTokenVault (69), SwapAggregator (42), five
  FlamingoSwapPairs, one Proxy. Behavior: OG Flamingo LP farmer.
- Current liquid balances: 1,903,838.91 FLM, 17,318.41 bNEO, 5,942.46 FLUND,
  2,789.09 GAS, plus 341,345 FUSD, 335,845 pONTv2, 13,572 pWINGv2 (and the
  usual phishing dust: fake-GAS, ngasx.com, "GAS Airdrop Ticket").
- **Notable link:** 36 legs with NWz12JwiyatnMNWZwojjE4ADCAdePdp5LK - the #2
  staker - starting 2022-01-05, three days after this wallet's first activity.
  Includes FLM flows both ways (incl. 76,113 out / 31,103 in Jan 2022,
  ~540,000 FLM in 2023-24), 12,000 bNEO sent mid-2022, and a 2024-09-29
  exchange of 10,000 bNEO for 203,140 FLUND + 127,894.81 FLM + 137,500 FUSD,
  with 14,100.93 bNEO returned 2025-05-21 (reads as a loan or OTC between two
  whales). The chain cannot prove whether these are two people or one.
- Evidence: `verify_top_staker.py`, `snap_state/top_staker_legs.json`.

## OrderBook trading wallets (RESOLVED 2026-10-16)

The FlamingoSwapOrderBook contract (`0x999f51af4d4ecd263b0ed3b8bbfb1c31f6b1d8f8`,
`NibkMaEPY1agiUdbzdKpno2jfWWX9FfH9L`) held **79,674.99128989 FLM** at the snapshot.
A full event ledger - every one of the 1,168 pre-snapshot txs, attributing each
`OrderStatusChanged` and `Transfer` notification to its order ID - shows:

- **All of it is locked in open limit orders; zero "unclaimed" free FLM.**
  Reconstructed locked total = 79,674.99128986 FLM (3 raw units of dust) - an
  exact reconciliation with the contract balance.
- 32 live orders carry FLM: **31 sell orders (FLM→fUSDT) and 1 buy order
  (52.00 FLM, bNEO market)**. None was ever partially filled or cancelled
  after creation - every one sits at its original deposit.
- 13 maker wallets, none flagged, none contracts, no attacker/custody overlap.

| Maker wallet | FLM locked |
|--------------|-----------:|
| NdsPeiZV2pSkfZj44vrGQU1wNe7sjH3Zwe | 21,610.63 |
| NPEodeusPVtm153e4iBLtXSUgpsHXVQPn1 | 18,000.00 |
| Ni17yzsxshLRGwJZMttxCQax77mgZQ4XUG | 11,650.00 |
| NYrNe5NSKg2q292hidVxeGLdB2n8qxfnEw | 10,000.00 |
| NZLBoeJ1jdBgbCZjKhPjaPeZqKMdJWZefU | 3,989.59 |
| NbK2gpYozaLeHzEm4DNiwoQo8swXMGUwUX | 3,500.00 |
| NWQS5aSC34UyJXNZ9A7FZ9iFHeVzdDReCu | 3,471.24 |
| Ni598hKKbnRGWsJkeUd1TmTVGEvhbkevTo | 2,900.00 |
| NY24YM8fRMeCV2Kkky7EpUX2KYoauciDVU | 2,108.95 |
| NfDwbCkK1J6uWZz27vEadUvcG4TWZT9mJw | 991.22 |
| NW2B8cr6SVQrDXqvTmicQ6ZCYi8qCbeaUu | 871.58 |
| NXgpQmkYui73ep7jNUJqQNWEk8iFhuUsu3 | 529.78 |
| NYpWtvWrTwrD2BvjSWJ7fMw6saTRFF2fWJ | 52.00 |

- Claim list: `orderbook_salvage.csv` (per-order) · Evidence: `snap_state/ob_order_ledger.json`,
  `snap_state/ob_txlogs/`, `snap_state/ob_all_legs.json` · Code: `orderbook_flows.py`,
  `orderbook_logs.py`, `orderbook_ledger.py`, `orderbook_probe.py`

## FTokenVault lending vaults (RESOLVED 2026-10-17)

The two FTokenVault contracts (FUSD collateral vaults) held **58,301,741.65 FLM**
at the snapshot, all of it user collateral backing FUSD loans. Unwrapped with the
contract's own `getAllVaults(FLM, FUSD)` view called historically at the snapshot
block (no storage archaeology needed):

- **vault1** (`0xe67e8975…`): a single position - `Ncq4LYyToAPXMr8SV2wXnCrHYi5EinAbcT`
  with **44,769,958.14 FLM** collateral and 813,520.40 FUSD debt. A leveraged-long
  whale: deposited 75.28M FLM May–Jun 2025, minted FUSD, bought more FLM (funding
  traced to FLUND redemptions, FLOCKS dividends, Broker trades, LP removals).
  Wallet last active 2026-10-06 - a live user, not abandoned.
- **vault2** (`0x799bbfcbc9…`): 15 positions with collateral, **13,531,783.51 FLM**,
  only 726.86 FUSD total debt. Top: `NW6rWCvsiqJaYEfqChRrTGxmrufJB2gzKa` 9.98M,
  `NU1GQvTGj6PzPYCYds7u37vmV8Din11Ec4` 2.30M - **both are funding sources of the
  vault1 whale** (sent it 15M and 5M FLM in May–Jun 2025): one whale cluster.

**FLAGGED 2026-10-17 (owner decision) - the whale cluster is excluded:**

- `Ncq4LYyToAPXMr8SV2wXnCrHYi5EinAbcT` - identified by the owner as the **NGD
  wallet**: the platform loan that was never liquidated and never repaid (the
  position documented in the Code Was Law articles, LTV ~2,150%). Flagged, no
  claim, no custody. This also removes its direct snapshot holdings from the
  claim base: 435,749.16 FLM + 12,483,575.73 FLOCKS.
- `NW6rWCvsiqJaYEfqChRrTGxmrufJB2gzKa` - feeder (sent the whale 15M FLM); holds
  15,918,923.73 FLOCKS directly + 119,718.74 FLM. Flagged as whale cluster.
  Owner suspects Flamingo dev-team ownership; on-chain it is a heavy
  FLOCKS/Broker/BurgerNEO user since 2023-09 - consistent but not proven.
- `NU1GQvTGj6PzPYCYds7u37vmV8Din11Ec4` - feeder (sent the whale 5M FLM);
  funded with GAS by the whale from its very first transaction (2024-01-01).
  Effectively whale-operated. Flagged. Direct holdings: 0 FLM + 6,916.72 FLOCKS.

- Claimable claim list: `vault_salvage_final.csv` - **13 wallets,
  1,248,400.67 FLM** (58,301,741.65 − 57,053,340.98 whale cluster).
  `vault_salvage.csv` (16 wallets, per-position FUSD debt) kept as the full
  record. No LP-token collateral in either vault at the snapshot (no
  double-count with LP salvage).

**POLICY DECISION NEEDED (debt):** mostly moot now - 99.9% of the 814,247.25
FUSD debt was the flagged whale's. The 13 claimable vault wallets carry only
726.86 FUSD of debt; gross crediting stands unless the owner says otherwise.

## What this does to the migration math (DECISION PENDING)

If LP + OrderBook + vault salvage are added to the claim base at r1 = 1, with
the custody wallet moved to the dev wallet and the whale cluster flagged
(2026-10-17):

| | FLM | FLOCKS | Combined |
|---|----:|-------:|---------:|
| Committed claim base | 186,427,071.14 | 113,932,226.80 | 300,359,297.94 |
| − whale cluster direct holdings | −555,467.90 | −28,409,416.18 | −28,964,884.08 |
| + unflagged #8 NWu2ng1h (claim resolved) | +15,500,000.00 | +1,000,000.00 | +16,500,000.00 |
| + LP salvage (1,350 wallets) | +126,003,802.00 | 0 | +126,003,802.00 |
| + OrderBook salvage (13 wallets) | +79,674.99 | 0 | +79,674.99 |
| + Vault salvage (13 wallets, gross) | +1,248,400.67 | 0 | +1,248,400.67 |
| **New claim base** | **328,703,480.90** | **86,522,810.62** | **415,226,291.52** |
| Dev wallet `NQzqB9ABJEQ5toYCBRc9kY99rFCTdqCwRD` (incl. #15 + Nd6YqmPt custody) | 106,524,961.83 | 5,030,925.75 | 111,555,887.58 |
| **New FTB total** | **435,228,442.73** | **91,553,736.37** | **526,782,179.10** |

Flag #8 `NWu2ng1hBxU6w6AUmGy4QDkscQTXY3pnVM` (15.5M FLM + 1M FLOCKS): **UNFLAGGED
2026-10-17**. Two independent grounds: (1) on-chain review contradicted the
original "allocation / holds nothing else" rationale - a 4-year active DeFi user
whose round-number funding traces to the flagged market-maker desks #6 and #11
(customer-side of desk flows, zero cluster overlap); (2) the claimant passed the
ownership proof (FTB_Unflag_Process.md) - TestNet tx
`0xcb423f7a878cefc711b0c3653cd8b2c07a33c5e08bcc81b22db74db025a2893c`, HALT,
1.0 GAS signed by and sent from the exact flagged address to the team TestNet
wallet. Key control proven; flag removed.

Note: the custody move does **not** change the FTB total - it only reclassifies
38,193,988.59 FLM from claim base to dev-wallet custody, exactly like #15.
OrderBook and vault salvage are additive (contract-attributed before).

**SUPERSEDED FRAMING (Decision 10, 2026-10-18):** the 526,782,179.10 figure above
was committed - but as the FLM-equivalent **entitlement base**, not the FTB
supply. FTB is a flat 100,000,000 hard cap; entitlements convert at
r = 0.189832 at mint time. The v2 claim tree (root `0x9a36b6a0…`, 5,194 wallets)
sums to exactly this base and is live on TestNet (Decision 11).

## Still behind (not user-attributable by this method)

The other contract FLM is a different question, layer by layer if the owner wants it:
FlamingoBroker (35.4M), the FLOCKS contract's own FLM (7.3M,
unpaid yield?), FLUND (5.1M), and dust. ~~OrderBook (0.08M)~~ and
~~FTokenVault (58.3M)~~ - resolved above.

*On-chain measurement, not legal or financial advice. Snapshot block 12,782,704.*
