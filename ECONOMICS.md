# FTB economics: rates, fees, APY

## Fee split (the constitution of the flows)

Every fee the system earns splits the same way:

- **2/3 → the liquidity providers** — swap LPs and lend suppliers. It accrues into the
  pool itself (share value rises; no mint, no transfers).
- **1/3 → FTB holders** — the dividend. It accrues to a reserve and is swept to the
  dividends module; FTB is dividend-bearing by construction.

The split is compiled in as the default (`reserveBps = 3333`) and governable only through
the proposal lifecycle, capped at 50% so holders can never be voted into funding the
whole pool alone — and providers can never quietly zero the dividend, because the change
needs a passed ballot.

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
