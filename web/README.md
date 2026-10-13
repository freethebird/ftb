# FTB web: wallet integration

The thin client layer that lets a NeoLine wallet (and other dApi wallets) talk to the FTB
contracts. It is the only part of the project that lives in the browser, and it holds no keys and
no logic the contracts do not already enforce. The signing boundary in `../../FTB_Stack_Map.png`
is exactly this folder: everything here builds and reads, the wallet signs.

## What is here

- `index.html` ... **the production page** (ticker, market board, wallet strip, swap ticket,
  claim, on-chain governance, verification panel), built on the template 1 black-and-white
  design system. One static file, no dependencies, no build step, no backend: reads go over
  raw JSON-RPC with failover across community nodes (plus a user-pastable "your own node"
  endpoint, stored only in the browser), and every write is a plan reviewed in words before
  the wallet signs. DEX patterns adapted from Hyperliquid (ticker, funds strip, preview
  before sign), THORSwap (quote → route → review flow), Terra Station (turnout vs eligible
  weight in governance), and NEAR (plain-word anchor nav) — all inside the typewriter skin.
  The market board sits at the top: one ratio card per pair, live from pool reserves — asset
  against asset, no fiat oracle — with both directions, a sparkline, and drift since you
  opened the page; history stays in the browser. The swap ticket quotes live from reserves
  with rate, min received at 0.5% slippage, and a price-impact warning. Governance renders
  each option as a card with its tally, a share-of-vote bar, turnout against the snapshot
  supply, and its own vote button, backed by a per-proposal ballot contract using the same
  Merkle proof as the claim. Ship `config.js` (hashes, community RPC list, merkle root,
  proofs URL, ballot question, pair list) and `proofs.json` next to it; pin the whole folder
  on IPFS.
  Local preview: `npm run dev` (a 30-line dependency-free static server, `serve.mjs`).
- `ftb-dapi.js` ... the adapter. Dependency-free, one file. Connects to NeoLine, reads state, and
  builds a typed invocation ("plan") for every FTB action. Plans are pure: nothing is signed until
  you pass one to `execute()`.
- `invocations.md` ... the per-method spec (script hash, operation, typed args, signer scope,
  read vs write). The contract between any client and the chain. Build your own UI against this.
- `UI_RATIONALE.md` ... every design decision mapped to its source (Hyperliquid, THORSwap,
  Terra Station, NEAR) plus the invariants future contributors must not break. Read it before
  touching `index.html`.
- `demo.html` ... a minimal reference page. Read-only by default; every write is build, review,
  then sign. Not a product, a worked example.
- `config.example.js` ... copy to `config.js` and set your contract hashes.

## Run the demo

NeoLine is a browser extension, so the demo needs a real browser with it installed, not a preview
pane. Install NeoLine, set it to TestNet, then open `demo.html` from this folder (double-click, or
serve the folder with any static server). Fill in the contract hashes (or prefill them by copying
`config.example.js` to `config.js`), connect, and try the reads first. Writes show you the exact
invocation and only send after you press Sign.

## Use the adapter in your own page

```html
<script src="ftb-dapi.js"></script>
<script>
  const ftb = new FtbDapi({
    network: 'TestNet',
    contracts: { ftb:'0x..', migrator:'0x..', lend:'0x..', bneo:'0x..', registry:'0x..', oracle:'0x..' },
    vote: { ftb:'N..', wait:'N..' }
  });
  await ftb.init();            // finds NeoLine
  const acct = await ftb.connect();

  // read (no signing)
  const bal = await ftb.balanceOf(acct.address);      // BigInt, base units
  console.log(FtbDapi.util.fromBaseUnits(bal.toString()) + ' FTB');

  // write: build a plan, show plan.summary to the user, then execute
  const plan = ftb.supply(acct.address, FtbDapi.util.toBaseUnits('10'));
  console.log(plan.summary);   // "Supply 10 FTB to the lend pool"
  const res = await ftb.execute(plan);   // the wallet prompts here
  console.log(res.txid);
</script>
```

Every action is a builder: `claim, supply, repay, borrow, withdrawSupply, withdrawCollateral,
addCollateralBneo, addCollateralGas, liquidate, vote, ballotVote, swap`. Reads: `balanceOf,
totalSupply, maxSupply, migrationOpen, claimed, debtOf, collateralValue, maxBorrow,
isUnhealthy, sharesOf, price, registryGet, ballotRoot, ballotNumOptions, ballotTally,
ballotVoted`. Subscribe to wallet changes with `ftb.on('accountChanged', fn)` and
`ftb.on('networkChanged', fn)`.

## Safety

- Read-only until you call a write builder and then `execute()`. The demo makes this a two-step
  review-then-sign on purpose.
- Least privilege: FTB-native writes sign with `CalledByEntry` (the connected account only). Only
  swap needs `CustomContracts`, and `signerFor()` builds that signer explicitly.
- Contract hashes are configured, never hard-coded, so the adapter cannot be pointed at a surprise
  contract by the code itself.
- TestNet first. Do not point at MainNet contracts holding value until the contracts are audited
  (see `../AUDIT_REPORT.md`) and these flows are tested.
- The swap path reuses the external Flamingo router; confirm its ABI against the deployed manifest
  before using the `swap` builder.

## Wallet compatibility

Built on the N3 dApi object (`window.NEOLineN3`), which NeoLine (extension and mobile) exposes, and
which O3 and OneGate implement too, so the same plans work across them. Newer NeoLine also exposes a
NEP-21 provider; `hasNep21()` detects it for information, and `invocations.md` lists the NEP-21
method names for anyone who wants to add that path. The signer account byte order (little-endian)
was verified against the NeoLine docs; if a specific wallet rejects a swap signer, flip it with
`signerFor(addr, allowed, { be:true })`.

## No-browser alternative

Everything here maps one-to-one to `neo-go contract invokefunction` (see the deploy and seed
runbooks in `../README.md`). The dApi is a convenience for users; it is not required to use FTB.

## Layout

The canonical layout is the Terra Station skin (chosen 12 October 2026 from a three-way
design lab against Hyperliquid and THORSwap variants, since retired): governance is the
front page — the proposal leads in display type in a framed hero panel — with wallet, claim,
market, and swap arranged in a two-column grid beneath it. Rationale and the lab's history
are in `UI_RATIONALE.md`.

## Templates

- `index.html` — the canonical page: **template 2, the station dashboard** (left-rail nav,
  bird mark, inverted governance hero, stat tiles, two-column market/swap). Chosen by the
  owner 13 October 2026.
- `template1.html` — the frozen first design (single-column arc), kept for reference.
- `template2.html` — same content as `index.html`; rebuild it with
  `python3 build_template2.py` after changing `template1.html` components, then copy over
  `index.html` (the builder reuses template 1's ids, modals, and signing script).
