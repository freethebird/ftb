# FTB governance: proposal → vote → code

The lifecycle, and the layer that enforces each step. Three labels:

- **[chain]** enforced by contract — no human can skip it
- **[policy]** enforced by published rule + human gate — the deliberate circuit breaker
- **[ops]** enforced by the operator runbooks and CI

## The eight steps

**1. Proposals cost money. [chain, v2]**
A proposal requires a bond in FTB, burned or escrowed when the ballot is
registered in the registry contract. Spam becomes expensive; serious proposals
are cheap. (Today: ballot deployment already costs the proposer real GAS; the
FTB bond is the v2 registry change — see "Contract deltas" below.)

**2. Many proposals live at once. [chain + UI]**
The registry keeps the board of live ballots (hash, question, options, close
block). The front end renders it; any node can serve the read. No one hosts
"the vote page" — the chain is the page.

**3. Weight = holdings, sybil-proof by construction. [chain — already built]**
The ballot contract is snapshot-weighted: a voter presents the same Merkle
proof they use to claim FTB, and their weight is their snapshot entitlement.
One vote per address, ever. Generating ten thousand fresh wallets yields
ten thousand votes of weight zero — weight lives in the snapshot, not in the
wallet. Splitting real holdings across wallets changes nothing either: weight
is conserved, not multiplied.

**4. Pass = 2/3 supermajority. [chain to read, policy to execute]**
`Tally(option)` is public; anyone can compute the ratio from any node. The
2/3 rule is applied at the execution gate (step 4e). A later ballot version
can expose `Passed()` on-chain for convenience, but execution stays human —
see "why" in DECISIONS.md.

**4a. First dev team to present the upgrade. [policy]**
Proposals carry a spec and a bounty. Implementation is competitive: the first
team to deliver a complete, reviewable branch is the candidate. Their commit
hash goes into the record — what gets voted on downstream is *that exact
hash*, not a description of it.

**4b. Audit before anything ships. [policy]**
The candidate code goes to audit. The report is committed to the repo
(`AUDIT*.md` — same as this project's existing audits). Fail = back to 4a,
no payment, no merge.

**4c. Audit passes → bounty paid. [ops → chain, v3]**
Initially: a 3-of-5 operator multisig pays the bounty on presentation of the
passing report and the ratified vote. Target: a treasury contract that pays
only against a passed ballot result, removing the multisig from the loop.

**4d. TestNet first. [ops]**
The ratified build deploys to TestNet via `scripts/deploy_testnet.sh`. Soak
period, public endpoints, anyone can hammer it.

**4e. Then, and only then, git. [policy]**
Merge requires three artifacts in the PR: the passed ballot (contract hash +
tally), the audit report, the TestNet deployment hashes. Merge creates tag
`gov-N`. This is the human circuit breaker — the chain decides, the repo
implements, the nodes enforce; no layer acts alone.

**4f. Front end follows the vote. [ops]**
`web/config.js` changes only to point at voted contract hashes. Static
redeploy; IPFS pin when available. The UI the community sees always matches
what governance ratified.

## Contract deltas this implies

| change | where | status |
|---|---|---|
| snapshot-weighted vote, one per address | ballot | built, tested |
| public tally, anyone reads | ballot | built |
| proposal bond in FTB | registry | v2, not started |
| live-proposals board | registry | v2, not started |
| bounty treasury keyed to ballot results | new contract | v3, not started |

## What this is not

A passed vote never auto-executes code. The gap between "vote passed" and
"code runs" — audit, TestNet, human merge — is intentional. It is how the
system survives a hostile or compromised proposal, and it is the clean
regulator story: transparent decision, independent implementation,
distributed enforcement.
