# FTB use cases → contracts → provenance

Every user-facing action, the contract it invokes, and who published that contract.
Three provenance classes:

- **Neo native** — published by the Neo project, built into every N3 node, same hash on
  every network. Not ours, not upgradeable by us.
- **FTB (this repo)** — written here, MIT, in `contracts/`, compiled to `build/`.
- **External immutable** — the audited Flamingo AMM, called by hash. A protocol peer,
  not a vendor: no one can revoke or change it. Absent on TestNet (DECISIONS.md, #3).

| # | Use case | Invokes | Provenance |
|---|----------|---------|------------|
| 1 | Claim migration FTB (merkle proof) | `ftb_migrator.claim` → `ftb_token.mintForMigration` | FTB / FTB |
| 2 | Check claim status | `ftb_migrator.claimed`, `ftb_token.migrationOpen` | FTB |
| 3 | Vote on a proposal (snapshot-weighted) | `ballot.vote` | FTB |
| 4 | Read tallies / my vote | `ballot.tally`, `ballot.voted` | FTB |
| 5 | Submit a proposal (bonded) | `ftb_token.transfer` → `registry.onNEP17Payment` | FTB |
| 6 | Read the proposals board | `registry.proposalCount`, `registry.proposal` | FTB |
| 7 | Close a proposal (committee, refund or keep bond) | `registry.closeProposal` | FTB |
| 8 | Post / top up a listing bond | `ftb_token.transfer` → `bondvault.onNEP17Payment` | FTB |
| 9 | Slash a rug / release on delist (committee, after vote) | `bondvault.slash`, `bondvault.delist` | FTB |
| 10 | Swap assets | `router.swapTokenInForTokenOut` (Flamingo) | External immutable |
| 11 | Read asset ratios (prices between assets) | `router.getReserves` (Flamingo) | External immutable |
| 12 | Supply FTB liquidity to the lend pool | `ftb_token.transfer` → `lend.onNEP17Payment` ("supply") | FTB |
| 13 | Post NEO or GAS collateral | `neo.transfer` / `gas.transfer` → `lend.onNEP17Payment` | Neo native → FTB |
| 14 | Borrow FTB against collateral | `lend.borrow` | FTB |
| 15 | Repay debt | `ftb_token.transfer` → `lend.onNEP17Payment` ("repay") | FTB |
| 16 | Withdraw collateral (health-checked) | `lend.withdrawCollateral` | FTB |
| 17 | Liquidate an unhealthy position | `ftb_token.transfer` → `lend.onNEP17Payment` ("liq") | FTB |
| 18 | Sweep the 1/3 fee share to FTB holders | `lend.sweepDividends` → dividends module | FTB |
| 19 | Oracle price report / read (lend pricing) | `oracle.report`, `oracle.price` | FTB |
| 20 | Pay transaction fees | `gas.transfer` (system) | Neo native |
| 21 | Committee governance actions (timelocked module swaps) | `registry.propose/apply` | FTB |
| 22 | Deploy / update contracts | `ContractManagement` | Neo native |
| 23 | Hash proofs (claim/vote verification) | `CryptoLib.sha256` | Neo native |
| 24 | Block height (timelocks, accrual) | `Ledger.currentIndex` | Neo native |

**What is NOT used:** bNEO / NeoBurger (dropped — collateral is native NEO and GAS, one
less external dependency), any hosted API, any custodial service. The front end talks to
nodes over plain JSON-RPC; wallets sign via the N3 dApi.
