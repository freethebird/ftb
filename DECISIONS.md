# FTB decisions: settled, and what is yours to call

After the first audit, most of the hardening is now in code. Two items genuinely need an owner
decision rather than a code change, and this brief gives a recommendation for each. The rest is
done and tested.

## Done in code (this pass)

The liquidation close-factor, bonus cap, and rate ceiling (FTB-07) are implemented: a single
liquidation repays at most half the debt unless the position is underwater, the liquidation bonus
is capped at 30 percent, and the per-block interest rate is capped at 1 percent (`maxRate`), which
is far above any real rate and only bounds misconfiguration. The 50 percent close-factor and the
ceilings are plain constants you can tune before launch. Merkle domain separation (FTB-11) is in:
leaves are hashed with a `0x00` tag and internal nodes with `0x01`, on both the off-chain builder
and the on-chain claim, verified consistent for odd and even trees and covered by the existing
round-trip test. These join the earlier fixes from the audit pass (renounce-before-close interlock,
write-once snapshot root, supply guards, debt-free collateral withdrawal, lend events, the
`MaxSupply` view, snapshot aggregation, and the seed empty-pool guard), each with a test.

## Decision 1: the cap number (FTB-01) is yours

**Status 16 October 2026: number COMMITTED by the owner — 373,721,196.93 FTB** (claim
base 186,427,071.14 FLM + 113,932,226.80 FLOCKS at r1 = r2 = 1, with 16 flagged
wallets excluded — the top-12 flags plus 6 inert desk sub-storage wallets from the
#11–#20 sweep; 7 active desk-funded wallets were tested against the CEX-withdrawal
hypothesis and judged legitimate — plus the dev-team wallet of 68,330,973.24
FLM-equivalent + 5,030,925.75 FLOCKS; see `FLM_Top20_Ownership.md`). Supersedes the
same-day 368,005,397.74. The dev-wallet FLM leg is still accumulating, so the
final `maxSupply` is set at launch: re-run `merkleroot` on the updated claim list
(`ftb_claim_list.csv`, 4,830 wallets) with `-r1 1 -r2 1`, add the
final dev-wallet allocation, and compile that exact sum in. The compiled-in value
below is superseded.

**Superseded status, 10 October 2026:** computed and compiled in, pending the vote on
ratios. Ran
`cmd/merkleroot -snapshot snapshot.csv -r1 1 -r2 4` against the final snapshot (4,847 holders):

- Merkle root: `0x639aaa7801f41c75d15c75aaad31aa6d6ede2b8bb81bd30f7ebf9560d50a8bba`
- Exact cap: `78326783050305942` base units (783,267,830.50305942 FTB), now the `maxSupply`
  constant in `ftb_token.go`
- Proofs: `proofs.json` (5.1 MB, 4,847 entries), every proof independently re-verified against
  the root in a separate Python implementation (4,847/4,847)
- Holder sums: 303.4M FLM + 120.0M FLOCKS claimed-eligible, vs 574.7M / 122.3M anchors — the
  difference is the excluded attacker and contract balances, by construction

If the vote picks different r1/r2, re-run merkleroot and reset the constant to the new printed
total before compiling for launch; the test constant in `ftb_token_test.go` must match.

(Original guidance follows.)

The token's `maxSupply` must equal the exact sum of all migration entitlements from the final
snapshot. The mechanism is in place: `cmd/merkleroot` prints that exact total, and the new
`MaxSupply()` view exposes the compiled value on-chain so anyone can check it. What remains is the
number itself, which only exists once the final snapshot and the community-agreed ratios are fixed.

Recommendation: run `merkleroot` on the final `snapshot.csv` with the agreed ratios, take the
printed total (`floor` of the sum over holders of `flm * r1 + flocks * r2`), set `maxSupply` to that
exact value, and publish both the number and `proofs.json` together. Add a `make` check that the
compiled constant equals the printed total so a mismatch cannot ship. This matters because a cap
below the total makes honest claims fail at the cap, and a cap above the total leaves mint headroom
in the pre-renounce window; exact equality removes both.

## Decision 2: oracle governance and quorum (FTB-06) is yours (deployment)

The oracle's own admin calls (`setReporter`, `setParams`) take effect immediately rather than
behind the registry timelock, and `minReports` ships at 1. In production that has to be a real
committee, or a single reporter can move the price, which is the exact risk the oracle exists to
remove.

Recommendation, which is a deployment choice rather than code: deploy the oracle (and lend) with the
owner set to the same timelocked multisig as the registry, call `setParams` with `minReports` of 3
or more, and enroll at least three independent reporters, each aggregating several venues off-chain.
I did not hard-code a floor of 3 because the test harness and TestNet legitimately run a single
reporter and a hard floor would break them; the floor belongs in the deployment checklist, where it
now sits in the pre-launch gate. If you would rather enforce it in code (timelock quorum reductions
and reporter additions inside the oracle), say so and I will add it.

## Still required regardless

Superseded by Decision 7 (14 October 2026): the external-audit requirement is withdrawn. The
public TestNet and the bug bounty still stand. The internal audit and these fixes are the
starting point the whole verification model now rests on. Lending stays out of the first
launch.

## Decision 3: TestNet deploy topology (11 October 2026)

TestNet has no Flamingo AMM router and no bNEO — the Neo core team keeps an open issue about
essential contracts being absent on TestNet (neo-project/neo#3285). Consequences, decided:

- **TestNet deploys only what is ours**: token, migrator, registry, and the first governance
  ballot (the r1=1, r2=4 ratio vote, 2 options, same Merkle root as the migration). Oracle and
  lend stay out — they are Launch 2 and audit-gated.
- **Router stays a placeholder on TestNet.** The market board and swap ticket degrade to their
  standby states by design; they go live only where pools exist (MainNet, or a minimal
  community AMM stood up for TestNet later — a separate decision).
- **Owner on TestNet is the deployer's single key.** Acceptable for a valueless test network;
  the driver prints the FTB-14 warning and MainNet must use `owner_multisig`. The deploy wallet
  (`cmd/deploy/testnet.wallet.json`, password in `cmd/deploy/.pass`) is git-ignored; its only
  job is paying deploy gas.
- One command after funding: `sh scripts/deploy_testnet.sh` — compiles, deploys, wires, and
  fills `web/config.js` from the driver's machine-readable output. Hash hand-copying is
  eliminated as an error class.

## Decision 4: UI pattern adoption (11 October 2026)

The page adopts DEX patterns from Hyperliquid (ticker, funds strip, preview-before-sign),
THORSwap (select → quote → review), Terra Station (turnout as the governance security metric),
and NEAR (plain-word nav), inside the typewriter skin. Full mapping and the invariants future
contributors must not break live in `web/UI_RATIONALE.md`. Two invariants are worth repeating
here because they are protocol-level, not cosmetic: **no fiat prices anywhere** (a fiat price
needs an oracle, and an oracle is a trust assumption the UI does not add), and **the page never
holds keys** (it builds and reads; the wallet signs).

## Decision 5: dependency bump for the govulncheck gate (13 October 2026)

Adding `make conformance` (the Go canon Article 1 gate: fmt, vet, build, test, `-race`,
govulncheck, NEF-hash record) surfaced six reachable vulnerabilities in two indirect
modules of the operator tooling (`golang.org/x/net` v0.58.0, `google.golang.org/grpc`
v1.82.1 — HTTP/2 paths reached by the RPC client in `cmd/deploy`/`cmd/seed`). Decided and
done: bumped to `x/net` v0.60.0 and `grpc` v1.83.1 (the recorded minimum-fixed versions),
with `x/crypto`, `x/mod`, `x/sync`, `x/sys`, `x/term`, `x/text`, and genproto following as
indirects. The pinned platform dependency (neo-go v0.123.0) and the `go 1.26.0` directive
are unchanged. Proof the bump changed nothing on-chain: all six contract NEFs recompile
**byte-identical** across it (hashes compared before and after), and the full suite plus
`-race` passes. govulncheck is now green and on the gate path, so the next vulnerable
transitive dependency fails the build instead of shipping silently.

## Decision 6: fee parity with Flamingo, and the treasury is a contract (14 October 2026)

With Flamingo gone, its rates are the community's reference point. Decided: the DEX swap
fee is **0.30% of volume — exact Flamingo parity**. What changes is where the money goes:
the old 2/3 : 1/3 (providers : holders) becomes **2/3 providers, 2/9 holders, 1/9 treasury**
(≈20 / 6.7 / 3.3 bps of swap volume; on lend interest the sweep splits the holders' reserve
1/3 to treasury, 2/3 to dividends). The LP share is untouched — liquidity is the moat — and
the treasury slice comes out of the holders' own dividend, because holders are the ones
voting the proposals it funds.

And the fund is **a contract, not a wallet** (`contracts/treasury`): budgets escrow to a
passed proposal id, the named auditor attests on-chain binding the exact audited NEF hash,
and the dev team self-serves two tranches — A on attestation, B after the testnet soak. A
failed audit or pre-attestation cancel returns the budget; there is no clawback of attested
work; excess accrual can only return to the registry-bound dividends module. No person ever
holds treasury keys. Committee execution uses the same trusted-window multisig already
recorded for the registry and oracle (QRN class A); moving execution behind the ballot
itself is v3 work, recorded here rather than smoothed over.

## Decision 7: no external audit — the audit posture is internal, gated, and public (14 October 2026)

Decided by the owner: **FTB will not commission external audits.** The previous launch gate —
two independent external audits of the Launch 1 core, plus a separate audit of oracle and lend —
is withdrawn, not satisfied. Per Article 6 this is recorded in the open everywhere the audit
posture is stated, not smoothed over:

- **What stands in its place.** The internal audit lineage (passes 1, 2, and the MO V8 / QRN
  pass), the `make conformance` gate — fmt, vet, compile, the full suite, `-race`,
  govulncheck, the NEF-hash record, the compile-twice determinism check, and the deepcheck
  trio (staticcheck, gosec, deadcode) — the fuzz gates, the published artifacts (snapshot,
  ratios, `proofs.json`, reproducible root), a public TestNet, and a bug bounty. Anyone can
  re-run every gate and re-derive every number; that is the verification model now.
- **What is given up, honestly.** A second pair of professional eyes. The internal lineage is
  self-review: it can be wrong in ways it cannot see. The treasury contract still names an
  auditor per proposal — that role is unaffected; what changes is that no firm signs the
  whole codebase before launch.
- **The consequence for users.** The banners and docs now say "internal audit lineage only;
  no external audit was performed." Anyone who requires an external audit before trusting
  value should not trust value — that is the honest reading, and it is printed where they
  will see it.
