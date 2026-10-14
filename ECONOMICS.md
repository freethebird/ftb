# FTB economics: rates, fees, APY

## Fee split (the constitution of the flows)

Trader-facing price: **0.30% per swap — Flamingo parity.** With Flamingo gone, its pool
fee is the rate the community already knows; we match it exactly and differ only in where
the money goes.

Every fee the system earns splits the same way:

- **2/3 → the liquidity providers** — swap LPs and lend suppliers. It accrues into the
  pool itself (share value rises; no mint, no transfers). Untouchable: LP yield is the
  liquidity moat.
- **2/9 → FTB holders** — the dividend. It accrues to a reserve and is swept to the
  dividends module; FTB is dividend-bearing by construction.
- **1/9 → the treasury** — the development fund. Of the holders' old third, one share in
  three now funds the proposals the holders themselves vote for: they reinvest part of
  their own dividend into the code they approved.

In basis points of swap volume at the 0.30% fee: LPs 20, holders ≈6.7, treasury ≈3.3.
On lend interest the same ratio falls out of the sweep: the reserve (reserveBps = 3333,
the holders' third) pays 1/3 to the treasury and 2/3 to dividends at sweep time. The split
is compiled in as the default and governable only through the proposal lifecycle, capped
at 50% so holders can never be voted into funding the whole pool alone — and providers can
never quietly zero the dividend, because the change needs a passed ballot.

## The treasury is a contract, not a wallet

A wallet means a person holds keys; that is a single point of failure and the first thing
a regulator or an adversary looks for. The treasury (`contracts/treasury`) holds the money
in chain state and pays out only through the pipeline:

1. The proposal ballot passes 2/3 with a budget, a dev team, and a named auditor.
2. The operator committee escrows the budget to the proposal id — funds stay in the
   contract until earned.
3. The auditor attests on-chain, binding the **exact audited NEF hash** (the bytes recorded
   in `build/NEF_HASHES.sha256`): payment is tied to the reviewed code, not to a promise.
4. The dev team self-serves: tranche A (default 30%) on attestation, tranche B after the
   testnet soak elapses. The committee cannot delay, reroute, or withhold.
5. A failed audit or a pre-attestation cancel returns the budget to the free balance.
   There is no clawback after attestation: reviewed work is earned.

The accrual cap is the `returnExcess` lever: surplus above what live proposals need can
only go back to the registry-bound dividends module — fail-closed, never to an arbitrary
address. Dev team and auditor must be different addresses by construction.


## Lend rates: classic utilization economics

The borrow rate follows utilization along a kinked curve (the Compound shape):

```
u = totalBorrows / (availableCash + totalBorrows)          utilization
borrowRate = base + u·slope1                    below the kink
           = base + kink·slope1 + (u−kink)·slope2   above it
supplyRate = borrowRate · u · (1 − reserveBps)    the suppliers' 2/3
```

Below the kink the pool prices for liquidity; above it the price rises steeply to pull
repayments and new supply in. All rates are per-block, 1e18 fixed-point.

## APY

Linear annualization at the 15-second target block time (`blocksPerYear = 2,102,400`),
exposed as safe views in basis points:

```
borrowApyBps = borrowRatePerBlock · blocksPerYear · 10^4 / 1e18
supplyApyBps = supplyRatePerBlock · blocksPerYear · 10^4 / 1e18
```

Daily activity drives the APY through utilization itself: more borrowing raises `u`,
which raises both rates; the dividend reserve grows with `interest · reserveBps` per
accrual, so holder APY is read from reserve growth over a window — the front end samples
`reserveAccrued` daily and annualizes (same pattern as the ratio ticker).

## Collateral

NEO and GAS only — the two native assets. NEO is indivisible (0 decimals): collateral
math prices it per whole NEO, and liquidation seizes floor-divided whole NEO (any dust
value stays with the borrower). No wrapped assets, no external dependency.
