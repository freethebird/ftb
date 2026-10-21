# FTB governance: proposal → vote → code

The lifecycle, and the layer that enforces each step. Three labels:

- **[chain]** enforced by contract - no human can skip it
- **[policy]** enforced by published rule + human gate - the deliberate circuit breaker
- **[ops]** enforced by the operator runbooks and CI

## The eight steps

**1. Proposals cost money. [chain, v2]**
A proposal requires a bond in FTB, burned or escrowed when the ballot is
registered in the registry contract. Spam becomes expensive; serious proposals
are cheap. (Today: ballot deployment already costs the proposer real GAS; the
FTB bond is the v2 registry change - see "Contract deltas" below.)

**2. Many proposals live at once. [chain + UI]**
The registry keeps the board of live ballots (hash, question, options, close
block). The front end renders it; any node can serve the read. No one hosts
"the vote page" - the chain is the page.

**3. Weight = holdings, sybil-proof by construction. [chain - already built]**
The ballot contract is snapshot-weighted: a voter presents the same Merkle
proof they use to claim FTB, and their weight is their snapshot entitlement.
One vote per address, ever. Generating ten thousand fresh wallets yields
ten thousand votes of weight zero - weight lives in the snapshot, not in the
wallet. Splitting real holdings across wallets changes nothing either: weight
is conserved, not multiplied.

**4. Pass = 2/3 supermajority. [chain to read, policy to execute]**
`Tally(option)` is public; anyone can compute the ratio from any node. The
2/3 rule is applied at the execution gate (step 4e). A later ballot version
can expose `Passed()` on-chain for convenience, but execution stays human -
see "why" in DECISIONS.md.

**4a. First dev team to present the upgrade. [policy]**
Proposals carry a spec and a bounty. Implementation is competitive: the first
team to deliver a complete, reviewable branch is the candidate. Their commit
hash goes into the record - what gets voted on downstream is *that exact
hash*, not a description of it.

**4b. Audit before anything ships. [policy]**
The candidate code goes to audit. The report is committed to the repo
(`AUDIT*.md` - same as this project's existing audits). Fail = back to 4a,
no payment, no merge.

**4c. Audit passes → bounty paid. [chain - built]**
The treasury contract holds the budget: the committee escrows it to the passed proposal id
(same trusted-window multisig as the registry - it turns the key, it never holds funds),
the auditor attests on-chain binding the exact audited NEF hash, and the dev team claims
tranche A on attestation and tranche B after the TestNet soak - self-service, no multisig
in the payment path. Failed audit or pre-attestation cancel returns the budget. Full
removal of the committee from escrow creation (ballot-executed funding) remains v3 work.

**4d. TestNet first. [ops]**
The ratified build deploys to TestNet via `scripts/deploy_testnet.sh`. Soak
period, public endpoints, anyone can hammer it.

**4e. Then, and only then, git. [policy]**
Merge requires three artifacts in the PR: the passed ballot (contract hash +
tally), the audit report, the TestNet deployment hashes. Merge creates tag
`gov-N`. This is the human circuit breaker - the chain decides, the repo
implements, the nodes enforce; no layer acts alone.

**4f. Front end follows the vote. [ops]**
`web/config.js` changes only to point at voted contract hashes. Static
redeploy; IPFS pin when available. The UI the community sees always matches
what governance ratified.

## Listing track: voting new tokens onto the DEX

A second proposal type, for admitting a project's token. Same ballot
machinery, different subject - and the project pays to play.

**L1. The project is the proposal. [chain, v2]**
A listing proposal names the token contract hash, the project, and - crucially -
the bond terms. The bond percentage is part of what gets voted on, not a
fixed constant: the community prices its trust per project. Ballot options
can even carry competing terms ("list at 1% bond / list at 2% / reject").

**L2. Pass → the bond comes due. [chain, v2]**
On a pass, the project must acquire the voted percentage of FTB supply - e.g.
1% - bought on the open market. That purchase is the listing fee in the only
currency that matters: demand for the community's token. No bond, no listing;
the front end never shows the pair until the lock is visible on-chain.

**L3. The bond splits 50/50. [chain, v2]**
- **50% → liquidity**: paired into the FTB/<token> pool, LP position locked.
  Day-one depth for the new market, and the project cannot pull it.
- **50% → rug-pull bond**: locked in a bond vault contract **for as long as
  the project is listed** - there is no vesting clock and no good-behavior
  unlock while the pair trades. If the project rugs - abandons, mints,
  drains - governance votes to slash: the locked FTB compensates harmed LPs
  or is burned. The bond is only ever released by a delisting vote (L4).

**L4. Delisting is the only key. [chain, v2]**
A delisting ballot ends the listing: the pair leaves the front end, the
liquidity half is unwound by the same vote's terms, and the vault half
returns to the project - unless a slash vote lands first. Listed = bonded;
delisted = released. Nothing in between.

**Why this shape works**
The project has capital at risk for the entire life of the listing - as long
as holders can buy the token, the project's bond answers for it. The
liquidity half makes the market real from block one; the vault half makes
"rug" an expensive word. And because the percentage itself is voted, a
blue-chip project and an anonymous launch get different prices of trust -
set by the community, per proposal, on the record.

## Contract deltas this implies

| change | where | status |
|---|---|---|
| snapshot-weighted vote, one per address | ballot | built, tested |
| public tally, anyone reads | ballot | built |
| proposal bond in FTB | registry | built, tested (exact bond, escrow, refund-or-keep) |
| live-proposals board | registry | built, tested (ballot + listing kinds) |
| listing proposal type (token hash + bond terms in the vote) | registry | built, tested |
| bond vault: 50/50 split, LP lock, slash-by-vote, unlock only on delisting | bondvault | built, tested |
| bounty treasury keyed to ballot results | new contract | v3, not started |

## What this is not

A passed vote never auto-executes code. The gap between "vote passed" and
"code runs" - audit, TestNet, human merge - is intentional. It is how the
system survives a hostile or compromised proposal, and it is the clean
regulator story: transparent decision, independent implementation,
distributed enforcement.
