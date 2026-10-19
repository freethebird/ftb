# Free the Bird (FTB): one-page overview

For a newcomer or an auditor. This ties the code in this repo to why it exists and how the
community decides to use it. Deeper documents are named at the end.

## The problem

On 29 August 2026, an attacker exploited a reward-calculation flaw in Flamingo's
lending-to-staking path and minted about 35.49 trillion FLM in a single transaction
(`0x5e310782...`, block 12,782,705), inflating FLM supply from roughly 570 million to about
37.7 trillion. The pools were drained, FLM was rendered worthless, and weeks later the
issuer had published no post-mortem and no plan. The lesson, in one line: the protocol died
because reward logic could mint a token without a cap. Everything here is built so that can
never happen again.

## The proof it is worth rebuilding

Measured on-chain, Flamingo was not a side app. Over its life it carried roughly one in four
of every Neo N3 transaction, nearly half at its 2022 peak, and about one in five of every
address that has ever existed on Neo has used it. That community is the asset that cannot be
recruited twice. The fair way to carry it forward is a snapshot taken one block before the
exploit, which excludes the attacker's minted supply by construction.

## The decision

What to do is a community choice, not a given. The vote is weighted by the pre-exploit
snapshot (so the attacker's 37.7 trillion minted FLM is worth zero), with a dual quorum of
token-weight and headcount, time-boxed and publicly tallied. See the game-theory piece and
the vote kit.

## The plan

Merge FLM and FLOCKS into one capped token, FTB, by a fair snapshot claim. Reuse Flamingo's
audited swap for trading. Seed liquidity from a community treasury so fees flow to the people
who provide it. Add lending later, on its own, behind its own audit. The bird is reborn, with
the keys held in the open.

## The build (this repo)

One language end to end, Go via neo-go. The audited swap is reused as deployed bytecode and
called by hash, so it never enters the repo. Modules, each its own contract and its own seam:

- `ftb_token` ... capped NEP-17. The only mint path is the Migrator, then sealed forever.
- `ftb_migrator` ... snapshot Merkle claim. One claim per address, attacker excluded.
- `registry` ... the seam board. Swap a peripheral module behind a timelock, no rewrites.
- `oracle` ... reporter-median price feed, reverts safely when fresh prices are too few.
- `lend` ... no-mint, bank-style supply and borrow. Launch 2, behind its own audit.

Off-chain Go: `cmd/merkleroot` (snapshot to root and proofs), `cmd/deploy` (deploy and
wire), `cmd/seed` (seed liquidity). Every contract has a neotest suite; `make` runs the lot.

## The one rule

No reward or interest path mints FTB. The only mint is the migration, capped and then sealed.
Rewards come from providing liquidity, never from printing. This is enforced in code and is
the single most important property an auditor should confirm.

## Safety posture

Simple designs, explicit powers that emit events, one-way doors for the irreversible acts,
keys behind a published multisig with the token's upgrade renounced after launch, and a
timelock on any module swap. Nothing reaches value until two independent audits of the launch
core, a separate audit of oracle plus lend, and a public TestNet with a bug bounty have
passed. Lending is deliberately not in the first launch.

## Status and how to run

```
make tools   # installs the neo-go compiler, SDK, and node
make         # gofmt, vet, compile every contract, run every test
```

On a fresh server, `ops/ftb-env-setup.sh` does all of the above from scratch (Go 1.26, neo-go,
compile, test); `ops/1984_SETUP.md` is the 1984 Hosting (Iceland) VPS runbook.

Launch core (token, migration, registry, swap wiring, tooling) and the Launch 2 modules
(oracle, lend) are written with tests. The remaining work is review, audits, TestNet, then a
guarded MainNet. See `README.md` for the deploy and seed runbooks and `ARCHITECTURE.md` for
the module and seam design.

## Companion documents (outside this repo)

- `FTB_Migration_Map.md` / `.png` ... the whole plan as a map.
- `FTB_Tokenomics.md`, `FTB_token_mechanics.png` ... how FTB works.
- `SNAPSHOT_README.md`, `snapshot_tool.py` ... the reproducible pre-exploit snapshot.
- `The_Game_We_Are_In.md`, `FTB_Vote.md` ... the game theory and the vote mechanism.
- `Flamingo_share_of_NEO_transactions.xlsx` / `.png`, `Flamingo_community_size.png` ... the
  on-chain evidence that the community is the asset.
- `NEO_DEX_Drained_Two_Men_High_Castle.pdf` ... the forensic account of the exploit.
- `docs/STACK_MAP.png` / `docs/STACK_MAP.md` - the stack from wallet to chain, with the NeoLine layer.
- `web/` ... the NeoLine wallet integration: adapter, wallet-neutral spec, reference demo.
- `AUDIT.md`, `AUDIT_REPORT.md`, `DECISIONS.md` ... the auditor brief, the first audit (findings +
  fixes), and the two decisions left to the owner.
