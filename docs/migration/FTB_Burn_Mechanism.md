# FTB Deflation Design - Burn Mechanisms

Date: 2026-10-17 · Status: **B1 + B2 + B3 ADOPTED by the owner (2026-10-17)**;
B4 rejected, B5 not adopted. Numbers below use the committed supply model
(Decision 10, 18 Oct): **100,000,000 FTB hard cap**; entitlement base
526,782,179.10 FLM-equiv (415.2M community claim base + 111.6M dev allocation);
conversion r = 100,000,000 ÷ 526,782,179.10 = 0.189832, applied at mint time.

## Goal

Offset the migration's large initial float with credible, continuous deflation -
without punishing legitimate claimants or starving launch liquidity.

## The mechanisms

### B1 - Deadline burn (ADOPTED)
Every FTB claim expires at the claim deadline. **Unclaimed amounts are burned**:
under the 100M cap, an entitlement that goes unclaimed is simply never minted,
and when `CloseMigration` seals the mint, that headroom dies with it - it is
never recycled to treasury or dev.

- Why: the strongest possible "claim or lose it" signal; the community watched
  flagged wallets get excluded - burning leftovers is the same logic applied forward.
- Expected effect: airdrop/claim campaigns historically see 10–30% of small
  balances never claimed. Community claims total ~78.8M FTB at r, so every 10%
  non-claim ≈ ~7.9M FTB never minted (30% ≈ 23.6M).
- Cost: none. Risk: complaints after deadline - mitigate with reminders at
  T-30/T-7/T-1 days.

### B2 - Custody timeout burn (ADOPTED)
The custody amounts (#15: 6,989,116.71 FLM-eq + 5,030,925.75 FLOCKS-eq; Nd6YqmPt:
38,193,988.59 FLM-eq - **50,214,031.05 FLM-equiv total, ~9.53M FTB at r**) sit in
the dev wallet pending ownership proof. Previous rule: unclaimed → Governance
proposal wallet. Adopted rule: **if unclaimed after a fixed window (12 months
from launch), burn 50% and send 50% to governance** - up to ~4.8M FTB burned.
Keeps the governance incentive while capping the custody overhang.

### B3 - Per-transaction burn allocation (ADOPTED, ongoing)
A **fixed percentage of every swap transaction** is allocated to a **dedicated
burn accumulator** - its own contract/wallet, entirely separate from the dev
wallet, the treasury, and the FeeVault. The dev wallet seeds initial liquidity
but has **no role in the burn path**: it neither funds the burns nor holds the
accumulator's keys.

- The accumulator's balance market-buys FTB and burns it (or, for the slice
  collected directly in FTB, burns it immediately) - the same "aggressive
  burning" mechanic the community already knows from FLOCKS (>115M FLM burned).
- Deflation rate scales with real usage instead of a fixed emission schedule:
  every transaction burns, forever.
- Implementation: a public, permissionless `burn()` trigger anyone can call -
  the accumulator feeds itself, no admin keys on the burn path.
- The exact percentage and which fee slice funds it (protocol-fee share vs. a
  dedicated burn component on top of the 0.30% pool fee + 0.10% protocol fee)
  is set in the launch parameters. It is a per-swap allocation, **not** a tax
  on plain wallet-to-wallet transfers.

### B4 - Optional claim rake (REJECTED 2026-10-17)
A 0.5–1% burn taken from each claim. Simple and immediate, but it taxes the very
people the migration is meant to make whole, and invites "wait for a better fork"
behavior. Rejected.

### B5 - Governance burns (not adopted)
Allow governance proposals to burn treasury/proposal-wallet tranches with a
supermajority. Not adopted at this time; governance can add it later by proposal.

## What NOT to do

- No transfer tax on every FTB movement - kills DEX aggregation and CEX listings.
- No rebasing / supply-elastic tricks - breaks claim math and integrations.
- Don't burn the dev wallet itself - it's the launch's liquidity and credibility.

## The adopted combination

B1 + B2 + B3. One-time deflation from non-claims and custody timeouts, then
permanent usage-driven deflation from revenue. No claimant is taxed; every
burned unit is either unclaimed or bought on the open market.

| Lever | Type | Size (100M-cap model) | When |
|-------|------|----------------------|------|
| B1 deadline burn | one-time | ~7.9–23.6M FTB (10–30% non-claim) | at claim deadline |
| B2 custody timeout | one-time | up to ~4.8M FTB | 12 months post-launch |
| B3 per-transaction allocation | continuous | scales with volume | from launch |

## Implementation notes

- Burn = provably unspendable address (null-address convention) with a public
  event, or a native `burn()` that reduces totalSupply - pick per FTB chain
  architecture; both are auditable from the ledger.
- All burn calls permissionless; no admin key on any burn path.
- Publish a burn dashboard (cumulative burned, per-mechanism) from day one -
  deflation only builds trust if it's visible.

*Adopted by the owner 2026-10-17 (B1 + B2 + B3). Implementation parameters -
the exact B3 per-transaction percentage and the B2 timeout length - are set in the launch
parameters; the mechanisms themselves are decided.*
