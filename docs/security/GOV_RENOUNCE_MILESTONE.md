# FTB - The Governance Renounce Milestone

**Status:** plan, 7 November 2026. This is the close-the-keyless-loop step of the FTB expansion sequence (Phase 0).
**Governing law:** the DEX Constitution. Keyless is unfixable, so this milestone is a point of no return: the audit must be real and the soak must be boring.
**Reads with:** [`KEYLESS_ATTESTATION.md`](KEYLESS_ATTESTATION.md) (the current owner-held state this milestone closes), the repo decision log [`DECISIONS.md`](../../DECISIONS.md), and the frozen NEF set [`NEF_RELEASE.sha256`](../../NEF_RELEASE.sha256).

## What renounces, and what it unlocks

Two contracts hold the only owner keys left in the FTB stack: **FtbDividends** (a self-timelocked upgrade key) and **FtbGov** (a one-time bind key). Renouncing both completes the keylessness of the whole governance and earn stack, because **FtbTreasury** and **FtbBondVault** already hold no owner key at all: their authority is a FtbGov vote, so the moment FtbGov is renounced and immutable, their trust root is fixed and they are fully keyless.

This is not only housekeeping. A renounced FtbGov is the precondition for the Monte Carlo's rank-1 product, the governance-resolved prediction market (Phase 2a, 31.7%): a market that resolves by governance needs the resolver itself to be ownerless. So this milestone is the unlock for the single most probable future product, not a side quest.

Out of scope, and why: the price-oracle problem that the de-keying design flagged as the hard part does not apply here. The deployed lending market is **FtbLoanBook**, the oracleless term-loan model (the willing lender prices the collateral), which replaced the price-dependent `lend`. There is no oracle anywhere in this milestone's scope.

## The gates (all must pass before any renounce)

1. **Build gate.** `make conformance` green on the Mac, end to end: build, `go test ./...`, `-race`, govulncheck (0 called), NEF determinism (byte-identical across two compiles), nefhash matches the pinned release, staticcheck / gosec / deadcode, Discord module gate. Authoritative.
2. **External audit** (dev not equal to auditor, from the vetted-auditor set). Scope is the new security-critical code: the FtbGov executor and the de-keyed treasury, bondvault, and dividends. Focus points:
   - FtbGov vote math: turnout at least 1/6 of the staked snapshot taken at propose, YES at least 2/3 of weight cast, one vote per address, correct `close` / `passed` / `subject` behaviour, and impostor-proofing (every executor binds the single FtbGov address).
   - The authorization-source move: on every de-keyed action, `passed(govID)` AND `subject(govID)` equals the recomputed subject AND a `consumed` anti-replay guard.
   - The byte-mask fix: `& 0xFF` on every packed byte in `escKey`, `monthKey`, and the FtbGov `idKey`, with the `TestPassedUnknownLargeId` regression; confirm no other unmasked byte packer survives (ballot option and commandments number are safely bounded).
   - Dividends: the 5760-block stake lock (the D-2 JIT-stake fix), the integrity of the self-timelocked upgrade path before renounce drops it, and the accounting invariants (`accPerShare`, `monthTotal`, `pendingOf`).
   - The audited hardening set unchanged: dev not equal to auditor, vetted-auditor set, soak floor, bound NEF hash, fixed slash sink and refund target.
3. **TestNet soak.** Deploy the full de-keyed stack to testnet and run governance end to end:
   - A real proposal lifecycle: `propose(subject, votingPeriod)`, stake FTB, `vote(id, voter, support)` one per address, window closes, pass at turnout at least 1/6 and YES at least 2/3, then ANYONE executes the bound treasury or bondvault action via `passed(govID)`.
   - Boundary cases: govIDs past 255 and escrow or month number 256 (the byte-mask boundary).
   - Failure paths that must reject cleanly: quorum miss, supermajority miss, replay attempt (consumed guard), impostor executor (wrong gov address), subject mismatch.
   - Soak floor (choose and record it): a fixed window with zero unexplained faults, recommend at least two weeks of continuous testnet operation and at least one successful run of every proposal type plus every failure path above. A single unexplained fault resets the clock.
4. **NEF freeze.** The audited NEF set is byte-pinned in `NEF_RELEASE.sha256` and the mainnet bytecode matches hash for hash (nefhash). Renounce only the exact audited bytes.

## The renounce sequence (irreversible; order and pre-checks matter)

Renounce locks every bind and every byte forever, so verify first, renounce second.

1. **Verify all configures are complete and correct on mainnet.** `ftb_gov.Configure(dividends)` done; `dividends.Configure(ftb)` done; `treasury.Configure(ftb, gov, registry)` done; `bondvault.Configure(ftb, gov)` done. Confirm each bound address is the intended mainnet address. A wrong bind becomes permanent.
2. **`dividends.renounceOwnership`.** Drops the self-timelocked upgrade key, leaving pure immutable accounting. Verify: `proposeUpgrade` and `applyUpgrade` now fault, a second renounce faults, and the RenounceOwnership event is emitted.
3. **`ftb_gov.renounceOwnership`.** Drops the bind key. Verify: `configure` now faults and a second renounce faults.
4. **treasury and bondvault: no call needed.** They have no owner. Confirm their only authority path is `passed(govID)` into the now-renounced, immutable FtbGov.
5. **Registry: decide and record.** Keep the 2-day timelock seam (a timelock, not a key, and the likely choice) or seal it only if its bootstrap setters are confirmed closed.

## After renounce

- Re-run the attestation [`KEYLESS_ATTESTATION.md`](KEYLESS_ATTESTATION.md) in full: every gov-stack contract now shows no reachable owner. Publish the updated card showing the full stack keyless.
- Light the governance and earn tabs: fill gov, treasury, bondvault, dividends hashes in `web/config.mainnet.js` (deployed hashes are already preserved in comments), then re-publish via the deploy-from-git flow.
- Update the DEX Safety card: S4 (access control) and S9 (upgrade/governance) move toward 10 for the whole stack rather than just the core, and S6 rises as audit, soak, and production time accrue.

## Honest caveats

- Keyless is unfixable. If the audit misses a bug, it is permanent. This is why the gates are heavy and the soak is deliberately dull.
- The order is one-way. Once FtbGov renounces, treasury and bondvault can never be re-bound, so the bind verification in step 1 is the single most important check in the milestone.
- Sequencing: this milestone has no deadline pressure from the core (the core is already live and keyless) and should not be rushed to light a few tabs. It is gated by the audit and the soak, full stop. Phase 2a waits on it by design.

---
*Governance renounce milestone v1.0. Governed by the DEX Constitution. The core is keyless now; this is how the governance and earn stack joins it, and how the prediction market's ownerless resolver comes into being.*
