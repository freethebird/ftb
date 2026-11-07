# FTB - DEX Safety Self-Assessment

**What this is.** Free the Bird scored against the public DEX Safety rubric (Model 2, dimensions S1 to S10) of the Decentralization and Safety Benchmark. It is a structured self-assessment against a published, reproducible methodology, scored by the same rubric applied to every peer. It is not a third-party audit and does not replace one. It measures design and verification posture, which is not the same as proven resilience.

**Read this first - what a score does NOT mean.** A high score is a judgment about design and verification, not a promise that funds cannot be lost, and not a track record. The rubric's own rule: an unlaunched or un-battle-tested protocol can score well on design yet have no history. Free the Bird is live on Neo N3 mainnet as of 6 November 2026, with real migration and trading, but it is days young, has no external audit, and has no long production record. The score reflects that honestly. See S6.

**The rubric.** Ten dimensions, each scored 0 to 10, weighted to a composite out of 100 (weights sum to 100). Every score rests on public evidence: on-chain data, source, or the keyless attestation (`KEYLESS_ATTESTATION.md`).

| # | Dimension | Wt | Score | Weighted | Basis |
|---|-----------|--:|:--:|:--:|-------|
| S1 | Fund safety (no path steals or loses funds) | 18 | 9 | 16.2 | Every fund path bounded and publicly verifiable; the core is renounced. Held at 9, not 10, for want of external adversarial review. |
| S2 | Oracle and price integrity | 12 | 10 | 12.0 | The lending market is oracleless: the willing lender prices the collateral, so there is no price feed to manipulate, which is the single most-exploited surface in DeFi. The swap is caller min-out protected. |
| S3 | Economic-invariant coverage | 12 | 9 | 10.8 | Term-loan model: no LTV, no liquidation price, no bad-debt cascade. Invariants pinned by firing tests; fuzzing is the path to 10. |
| S4 | Access-control and authority minimization | 12 | 9 | 10.8 | The loan book is keyless and immutable by construction; the core is renounced on chain. 9 until the governance stack renounces at its milestone. |
| S5 | Blast-radius containment | 10 | 8 | 8.0 | Loans are isolated; one default never touches another; the lending market is self-contained. The router remains a swap boundary. |
| S6 | Verification depth | 10 | 5 | 5.0 | The honest anchor. Full public source and byte-reproducible builds, yes; but no formal proofs, no external audit, and little production time. This is the one being earned, not claimed. |
| S7 | Feature-surface discipline | 8 | 8 | 6.4 | Whole subsystems removed rather than guarded: the oracle and its feeder key, the risk-tuning keys, the upgrade timelocks. Small, on-the-record surface. |
| S8 | Signing and front-end integrity | 8 | 9 | 7.2 | Keyless front end served from IPFS with no backend to seize or change; witness-scoped calls, pinned recipients, no egress. |
| S9 | Upgrade and governance safety | 6 | 8 | 4.8 | The core's upgrade authority is retired (renounced). 8 until the governance and earn modules retire theirs at the renounce milestone. |
| S10 | Residual-risk transparency | 4 | 8 | 3.2 | Every residual named, bounded, and on record: the migrator's seal-only grief key, the not-yet-renounced governance stack. Live monitoring is the next step. |
| | **Total** | **100** | | **84.4** | |

**Calibration.** The same rubric puts Uniswap, the immutable-core benchmark the industry already trusts, at 83.4. Free the Bird scores 84.4, essentially level, not above it. That is the test of an evenhanded rubric: it does not flatter the project that publishes it.

**The weak spot, stated plainly.** S6 at 5 is the lowest axis, and it is deliberate. There is no external audit yet and no long track record. The design and the keyless posture are strong and verifiable on chain today; the audit, the formal work, and the production time are what the project has yet to earn. The governance and earn modules are not renounced precisely because they have not earned it.

**Verify, do not trust.** The keyless claims behind S4, S8, S9, and S10 are checkable on chain in minutes; `KEYLESS_ATTESTATION.md` in this folder gives the exact RPC calls. The full comparative benchmark across other chains and DEXs is published separately.

---
*Self-assessment against the public DEX Safety rubric (Model 2, v0.2), 7 November 2026. Scored from public evidence, by the same rubric applied to every peer. Not a third-party audit.*
