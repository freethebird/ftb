# FTB invocation spec

Every FTB action as a wallet invocation, so any front end (or CLI) can drive the contracts, not
just the demo. This is the contract between the client and the chain. The adapter `ftb-dapi.js`
builds exactly these; the CLI runbook in `../README.md` builds the same with `neo-go`.

## How the dApi expresses an argument

The NeoLine N3 dApi (and O3, OneGate) take each argument as `{ type, value }`. Types used here:

| type | value format | used for |
|------|--------------|----------|
| `Hash160` | `0x` + 40 hex | a contract or script hash |
| `Address` | `N...` base58 | a user account (what `getAccount` returns) |
| `Integer` | decimal string in base units | amounts, shares |
| `String` | text | the lend action tag |
| `ByteArray` | base64 | a Merkle proof element |
| `Array` | list of the above | proof list, transfer `data` |
| `Any` | `null` | the `data` slot when a contract ignores it |

Amounts are base units: FTB, bNEO, and GAS are 8 decimals, so 1 FTB is `100000000`.

## Signers and scope

Writes carry a signer: `{ account, scopes, allowedContracts }`. `account` is the little-endian
script hash of the signing address (the adapter's `signerFor()` produces it; most calls can omit
signers entirely and let the wallet sign as the connected account with `CalledByEntry`). Scope
values: `None 0`, `CalledByEntry 1`, `CustomContracts 16`, `CustomGroups 32`, `Global 128`.

Rule of thumb: every FTB-native call below is safe with `CalledByEntry` (1), the least-privilege
scope, which the wallet applies by default. Only the external swap, which hops through the router
into token and pair contracts, needs `CustomContracts` (16) listing those hashes. Never sign with
`Global` unless you have a specific reason.

Hashes shown as `<ftb>`, `<lend>`, etc. are per deployment; set them in `config.js`. `<gas>` is the
native `0xd2a4cff31913016155e38e474a2c06d08be276cf`.

## Token (`<ftb>`)

| call | kind | operation | args |
|------|------|-----------|------|
| balance | read | `balanceOf` | `Hash160 account` |
| total supply | read | `totalSupply` | none |
| cap | read | `maxSupply` | none |
| migration open? | read | `migrationOpen` | none |
| transfer | write | `transfer` | `Hash160 from, Hash160 to, Integer amount, Any data` |

## Migration (`<migrator>`)

| call | kind | operation | args |
|------|------|-----------|------|
| already claimed? | read | `claimed` | `Hash160 account` |
| claim | write | `claim` | `Hash160 account, Integer amount, Array[ByteArray] proof` |

Claim notes: the `amount` and the `proof` come from the published `proofs.json` (one entry per
address). Each proof element is a 32-byte hash, sent as base64. The caller must be `account` (the
wallet signs for it), and each address can claim once. The attacker is not in the tree, so it has
no proof.

## Lend (`<lend>`), Launch 2

Supply, repay, liquidate, and adding collateral are NOT direct lend calls. They are NEP-17
transfers into the lend contract, which routes on the `data` tag. Borrow and the withdrawals are
direct calls.

| action | kind | target | operation | args |
|--------|------|--------|-----------|------|
| supply FTB | write | `<ftb>` | `transfer` | `from, <lend>, amount, Array[String "supply"]` |
| repay FTB | write | `<ftb>` | `transfer` | `from, <lend>, amount, Array[String "repay"]` |
| liquidate | write | `<ftb>` | `transfer` | `liquidator, <lend>, repay, Array[String "liq", Hash160 borrower]` |
| add bNEO collateral | write | `<bneo>` | `transfer` | `from, <lend>, amount, Any null` |
| add GAS collateral | write | `<gas>` | `transfer` | `from, <lend>, amount, Any null` |
| borrow | write | `<lend>` | `borrow` | `Hash160 user, Integer amount` |
| withdraw supply | write | `<lend>` | `withdrawSupply` | `Hash160 user, Integer shares` |
| withdraw collateral | write | `<lend>` | `withdrawCollateral` | `Hash160 user, Integer bneo, Integer gas` |
| my debt | read | `<lend>` | `debtOf` | `Hash160 user` |
| my collateral value | read | `<lend>` | `collateralValue` | `Hash160 user` |
| my max borrow | read | `<lend>` | `maxBorrow` | `Hash160 user` |
| am I liquidatable? | read | `<lend>` | `isUnhealthy` | `Hash160 user` |
| my supply shares | read | `<lend>` | `sharesOf` | `Hash160 user` |

## Oracle (`<oracle>`), Launch 2

| call | kind | operation | args |
|------|------|-----------|------|
| price of asset | read | `price` | `Hash160 asset` (bNEO or GAS) |
| is a reporter? | read | `enabled` | `Hash160 reporter` |
| report a price | write | `report` | `Hash160 reporter, Hash160 asset, Integer price` (reporter only) |

Price unit: FTB base units per 1 whole asset.

## Registry (`<registry>`)

| call | kind | operation | args |
|------|------|-----------|------|
| resolve a module | read | `get` | `String name` (for example `"oracle"`) |
| timelock delay | read | `delay` | none |
| apply a pending change | write | `apply` | `String name` (anyone, after the timelock elapses) |

## Vote (native `<gas>`)

The snapshot-weighted vote is cast as a dust GAS transfer to the published address for your
choice. Weight is your pre-exploit FLM + FLOCKS balance; the tally script reads these transfers.

| action | kind | target | operation | args |
|--------|------|--------|-----------|------|
| vote Free the Bird | write | `<gas>` | `transfer` | `voter, <VOTE-FTB>, Integer 1, Any null` |
| vote keep waiting | write | `<gas>` | `transfer` | `voter, <VOTE-WAIT>, Integer 1, Any null` |

`Integer 1` is one base unit, 0.00000001 GAS. Never send more than the dust to a vote address.

## Ballot (`<ballot>`) — on-chain weighted governance

The ballot contract is deployed once per proposal; its hash IS the proposal identity. It is
configured once at deploy time with the SAME Merkle root as the migration, so every snapshot
holder votes with the proof they already have in `proofs.json`. One vote per snapshot address,
weight = snapshot balance, tallies live in chain state so every node verifies the same count.

| call | kind | operation | args |
|------|------|-----------|------|
| bound root | read | `root` | none |
| number of options | read | `numOptions` | none |
| tally of an option | read | `tally` | `Integer option` (0-based) |
| has an address voted? | read | `voted` | `Hash160 account` |
| vote | write | `vote` | `Hash160 account, Integer amount, Integer option, Array[ByteArray] proof` |

Vote notes: `amount` is the snapshot balance in base units and `proof` the sibling hashes, both
straight from `proofs.json` — identical to a migration claim. The wallet must sign as `account`
(CheckWitness). Re-voting, voting with an inflated amount, and voting from an address that is
not in the snapshot all fail on-chain. Tally results are base units of snapshot weight.

## Swap (external Flamingo router `<router>`)

The swap reuses the audited Flamingo router and is the one place to confirm the exact ABI against
the deployed manifest (method name, argument order, and the pair hashes), because it is not part of
this repo. The shape documented here matches the community guide.

| call | kind | operation | args |
|------|------|-----------|------|
| quote | read | `getAmountsOut` | `Integer amountIn, Array[Hash160] path` |
| reserves | read | `getReserves` | `Hash160 tokenA, Hash160 tokenB` |
| swap | write | `swapTokenInForTokenOut` | `Hash160 sender, Integer amountIn, Integer amountOutMin, Array[Hash160] path, Integer deadline` |

Swap signer: multi-hop pulls the input token and touches pair contracts, so sign with
`CustomContracts` (16) listing the router, both path tokens, and the pair contracts, or the swap
will fail a witness check mid-route. `path` is the FLM-hub route, for example `[GAS, FLM, FLOCKS]`.
`deadline` is a timestamp in milliseconds.

## Errors

Legacy dApi rejects with `{ type, description, data }`: `NO_PROVIDER`, `CONNECTION_DENIED`,
`CHAIN_NOT_MATCH`, `CANCELED`, `RPC_ERROR`, `INSUFFICIENT_FUNDS`, `MALFORMED_INPUT`. The newer NEP-21
provider uses numeric codes (`10006 CANCELED`, `10004 FAILED`, and so on). Network magic numbers:
MainNet `860833102`, TestNet `894710606`.
