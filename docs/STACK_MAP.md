# FTB stack map: companion notes

A reading of `FTB_Stack_Map.png`, top to bottom. The picture answers one question: how does a
person with a NeoLine wallet actually use FTB, and where is the line that keeps them safe.

## The one line that matters: the signing boundary

The dashed red line sits just under the wallets. Private keys live above it and never cross it.
Everything below, the client page, the public nodes, the contracts, only ever sees a signed
transaction or a read request. So a bug or a bad actor in the client layer cannot move anyone's
funds; the worst it can do is ask the wallet to sign something, and the wallet shows the user what
that something is first. This is why the integration can be "just a web page" without that being a
risk to custody.

## The layers

People and their wallets. NeoLine is the target, as the extension and on mobile. Because the
integration speaks the standard Neo dApi, the same thing works with other dApi wallets like O3 and
OneGate; no FTB-specific wallet is needed.

Client layer (this is the NeoLine integration). The adapter `ftb-dapi.js` connects to the wallet,
reads chain state, and builds a typed invocation for each FTB action. It holds no keys. The demo
page and any community UI sit on top of it, and a CLI using neo-go is the same operations without a
browser. The dApi verbs are small: `getAccount`, `invokeRead` for reads, `invoke` for writes, and
`send` for the dust-transfer vote, plus account and network change events.

Access layer. Public Neo N3 RPC nodes (community seed nodes and public RPC) are where the wallet
broadcasts a signed transaction and where the client reads. The NeoFura indexer is read-only and is
what the snapshot and the vote tally are built from.

FTB contracts (this repo). The capped token with its single sealed mint path, the migrator that
pays out the snapshot by Merkle claim, the registry that lets a peripheral module be swapped behind
a timelock, and the Launch 2 oracle and no-mint lend. The reused Flamingo swap sits alongside,
called by hash, never re-implemented, and bNEO wraps NEO to 8 decimals for collateral and trading.

Neo N3 base chain. The trust root no one in the project controls: NeoVM and dBFT consensus, the
native contracts (GAS, NEO, Policy, ContractManagement), and CryptoLib, whose sha256 is what the
on-chain Merkle claim uses.

Off-chain Go tools (the right column). Operator-only and pre-launch: `merkleroot` turns the snapshot
into a root and proofs, `deploy` stands up and wires the contracts and then renounces, and `seed`
puts community liquidity into the pools. The keys behind all of this are a published multisig, and
the token's upgrade is renounced once launched.

## The one rule the whole stack serves

No reward or interest path mints FTB. The only mint is the capped migration, and then it is sealed.
Read paths are green and change nothing; write paths are pink and are always signed by the user.
Everything above is in service of making that rule hold and making it checkable by anyone.

Status: the internal audit lineage is done (passes 1-2 plus the MO V8 / QRN pass, all gates
green); per DECISIONS.md Decision 7 no external audit will be performed - verify before value.
contract hashes configured per deployment, not hard-coded in the client. See `ftb/AUDIT_REPORT.md`.
