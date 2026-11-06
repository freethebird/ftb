# FtbLoanBook — state machine & invariants

Oracleless, keyless, immutable peer-to-peer term-loan book. No oracle, no LTV, no
liquidation price, no margin call, no owner, no upgrade path. The contract enforces
exactly one fact: **was the loan repaid in full before maturity?** The lender prices
the collateral when they post the offer; the willing lender is the oracle.

Amounts: FTB and GAS are base units (8 decimals); NEO is whole units (0 decimals).

## States  (storage prefix 0x19, per offer id)

| # | state | meaning | terminal |
|---|-------|---------|----------|
| 0 | None | id never created (zero value) | — |
| 1 | Open | principal escrowed, awaiting a borrower | no |
| 2 | Taken | collateral escrowed, principal released, clock running | no |
| 3 | Repaid | principal+interest paid before maturity; collateral returned | yes |
| 4 | Defaulted | maturity passed unrepaid; collateral credited to lender | yes |
| 5 | Cancelled | open offer withdrawn by lender; principal refunded | yes |

## Transitions

    None --postOffer(FTB in)--> Open --take(NEO/GAS in)--> Taken
                                 |                            |
                 CancelOffer     |            repay(FTB in)   |  t < maturity
               (funder, witness) v                            v
                             Cancelled                      Repaid

                                 Taken --Close(anyone)--> Defaulted
                                         t >= maturity

Guards — every illegal transition faults, which reverts the caller's own transfer, so
nothing is ever half-locked:

- take:        state==Open AND token==collateralToken AND amount==collateralAmount
- repay:       state==Taken AND GetTime() <  maturity AND amount >= principal+interest
- Close:       state==Taken AND GetTime() >= maturity
- CancelOffer: state==Open  AND CheckWitness(funder)
- Withdraw:    CheckWitness(funder) AND ledger[funder][token] > 0

The maturity boundary has no gap and no overlap: repay for `t < maturity`, Close for
`t >= maturity`. Maturity is wall-clock (`GetTime()+termMs`, set at take), immune to
Council block-time changes.

## Entry points & authority

| method | caller | effect |
|--------|--------|--------|
| OnNEP17Payment "offer" (FTB) | lender | open an offer, escrow principal |
| OnNEP17Payment "take" (NEO/GAS) | borrower | take an offer, escrow collateral, receive principal |
| OnNEP17Payment "repay" (FTB) | anyone | repay a loan (collateral always returns to the recorded borrower; overpay refunded) |
| Close(id) | anyone | settle a defaulted loan (collateral → lender's pull ledger; **no outbound call**) |
| CancelOffer(id) | funder (witness) | withdraw an un-taken offer, refund principal |
| Withdraw(funder, token) | funder (witness) | pull accumulated proceeds for one token |

There is **no owner, no admin method, and no update path**. `_deploy` stores only the FTB
token hash and refuses re-init. The contract is immutable from its first block — this is
why the LØ key panel reports it structurally **Keyless** (nothing to renounce).

## Invariants (enforced in code, pinned by tests)

1. **Conservation.** `book.FTB == Σ(open principals) + Σ(repaid proceeds not yet withdrawn)`;
   `book.NEO == Σ(taken-unrepaid NEO) + Σ(defaulted NEO not yet withdrawn)`. Nothing is
   minted or lost.  → TestConservationMultiLoan
2. **Checks-effects-interactions** on every path: state is written before any token leaves.
   Reentrancy into a settled id finds it terminal and faults.
3. **Pull, not push, for lenders.** Proceeds are credited to a ledger and withdrawn by the
   lender, so a contract-lender can never brick a borrower's repay or a defaulter's close.
   `Close` makes no outbound call — zero reentrancy surface.
4. **No double-settle.** Repaid/Defaulted/Cancelled are terminal; repay-twice, close-twice,
   close-after-repay, repay-after-default, take-twice, cancel-after-take and withdraw-twice
   all fault.  → TestStateMachineExhaustive
5. **Immutable collateral allowlist**: native NEO and GAS only; a later asset is a fresh
   deploy, never a key.
6. **Flat, integer-floored interest**: `repay = principal + principal*rateBps/10000`.
   → TestInterestRoundingAndZero

## By-design residuals (disclosed, not defects)

- **GAS yield on NEO collateral is forfeited.** NEO held as collateral generates GAS to the
  contract; that GAS is credited to no one and has no withdrawal path, so it is locked. Dust
  for short terms — disclose to borrowers posting NEO.
- **Hard maturity cliff.** No grace period: at `maturity` the borrower can no longer repay and
  anyone may Close. Intended (the lender's downside is bounded to "receive the collateral"),
  but borrowers must repay strictly before maturity.

## Test coverage

- Base (`ftb_loanbook_test.go`): happy path, default + permissionless close, overpay refund,
  cancel, validation bounds, okey byte-mask regression (ids ≥ 256).
- Invariant (`ftb_loanbook_invariants_test.go`): conservation across mixed outcomes,
  state-machine exhaustiveness, GAS collateral, interest rounding/zero.
- Recommended CI addition: a reentrancy harness (a malicious NEP-17 recipient that reenters on
  principal receipt and on collateral return) to turn the CEI proof into a running test.
