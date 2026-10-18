# FTB web — module canon

**Status:** law for the `web/` layer, under MO V8, the Canonical Bible (Go edition), and
the QRN threat model. Ratified 18 October 2026 alongside `WEB_AUDIT.md`.
**The rule that creates this file:** every module is independent. One concern, one file,
one DOM subtree, explicit seams only. The page may ship any subset of modules — each must
run correctly with its own panel present and every other panel absent.

## The cross-cutting rules (bind every module)

R1. **One file per module, one root per module.** A module owns exactly one DOM subtree
   (its card or strip) and exactly one JS file. It never reaches into another module's
   ids. Cross-module CSS is forbidden; the shell owns the theme tokens.
R2. **Self-guard.** Every module file opens by locating its root element; if absent, it
   installs nothing and returns. No module may assume another module exists.
R3. **No direct `fetch` outside the seams.** All chain reads go through `ftb-rpc`; all
   writes are pure plans built by `ftb-dapi` builders, reviewed in the shell's one modal,
   and signed only by the wallet. A module that calls `fetch` itself is a defect.
R4. **No keys, ever.** No module stores, requests, derives, or displays secret material.
   No module requests a witness scope beyond `CalledByEntry` — the single documented
   exception is the swap module's `CustomContracts` signer, and it is built only by
   `ftb-dapi.signerFor()`.
R5. **Numbers are earned or absent.** Every displayed number comes from a chain read, a
   published artifact (`proofs.json`, `beat.json`), or `config.js`. A module that cannot
   read shows its named standby state — never a fabricated, cached-as-fresh, or
   hard-coded figure. Constants may live only in `config.js`.
R6. **One review modal.** The shell owns `#ov`. A write is: module builds a plan →
   `showPlan(title, warn, rows, plan)` → the user reviews → the shell executes. No module
   opens its own signing UI, and no module calls `execute()` except through the modal
   path. The modal always shows `plan.summary` verbatim.
R7. **Shared state crosses one seam.** The only cross-module state is the explicit state
   seam (`ftb-state`: current account, current RPC, loaded proofs). No globals, no DOM
   scraping, no `window.*` message-passing between modules. (Retires the
   `window.FTB_ACCOUNT` pattern, WEB-05.)
R8. **Degrade in the open.** If a module's contract hash is a placeholder (`0x0000…`) or
   its reads fail, it shows the standby text that names exactly what is missing. A
   degraded module never blocks, blanks, or crashes the others.
R9. **localStorage is browser-local and named.** Keys are prefixed `ftb_`, hold no
   secrets, and their loss changes nothing but convenience (theme, own node, ratio
   history, APY sample).
R10. **The invariants of `UI_RATIONALE.md` bind every module:** no dependencies, no build
    step, strict grayscale with the phosphor key reserved for clickables only, no fiat
    prices, no external APIs, no analytics, motion is the blinking cursor.

## Target layout

```
web/
  index.html            # shell markup: one <section>/<div> root per module, nothing else
  ftb-dapi.js           # module 11 (the adapter) — unchanged role
  js/ftb-rpc.js         # module 10 — the one read seam
  js/ftb-state.js       # the state seam (R7)
  js/shell.js           # module 12
  js/mod-token.js       # module 2
  js/mod-swap.js        # module 1
  js/mod-earn.js        # module 3
  js/mod-lend.js        # module 4
  js/mod-gov.js         # module 5
  js/mod-board.js       # module 6
  js/mod-claim.js       # module 7
  js/mod-verify.js      # module 8
  js/mod-wallet.js      # module 9
  config.js             # module 13 (deployment constants, git-ignored)
  proofs.json, beat.json
```

Load order: `config.js`, `ftb-dapi.js`, `ftb-rpc.js`, `ftb-state.js`, `shell.js`, then the
modules in any order (R2 makes order irrelevant among them).

## The modules

### 1. Swap / Pools — `mod-swap.js` — owns `#trade`, `#market`, `#ticker`, and the tile `#tTvl`

- **Reads:** `router.getReserves(tokenA, tokenB)` for each configured pair; nothing else.
- **Writes:** `dapi.swap(sender, amtIn, minOut, path, deadlineMs)` — the only module that
  may attach a `CustomContracts` signer, via `dapi.signerFor()` and naming the router, the
  path tokens, and the pair contracts.
- **Rules.** The quote is computed from live reserves at 0.3% fee, constant product; min
  received is 0.5% under the quote; price impact past 5% draws the warning style. The
  market board and ticker show asset-against-asset ratios only — no fiat, no oracle, both
  directions, drift history in `localStorage.ftb_ratios` (display-only, never trusted as
  fresh). TVL (tile `tTvl`) sums FTB-side reserves of configured pairs, in FTB terms;
  non-FTB pairs are excluded by design (no price path without an oracle); on a network
  with no router it reads `0 FTB` with the reason in the tooltip. All three surfaces
  (board, ticker, ticket) are fed by the same reserves reads — never a second data path.
- **Standby:** router hash placeholder → "loads when the router hash is configured", TVL
  `0 FTB` with the TestNet explanation.
- **Anchors:** MO Articles 4, 6; Annex B 11; QRN F (app-layer honesty).

### 2. Token on FTB — `mod-token.js` — owns the stat tiles `#tSupply #tCap #tBlock` and the wallet strip `#balStrip`

- **Reads:** `ftb.totalSupply`, `ftb.maxSupply`, `getblockcount`, `ftb.balanceOf(acct)`,
  `neo.balanceOf` (whole units, never scaled), `gas.balanceOf`.
- **Writes:** none. The token module never builds a transfer; transfers happen inside
  module flows (stake, supply, claim) by those modules' own builders.
- **Rules.** Supply and cap always read the chain, never config. NEO displays whole
  units; FTB and GAS display base-units-formatted. The strip appears only when an account
  is connected. The seal state (`migrationOpen`) is rendered as `migration open` /
  `mint sealed` — the two honest states, nothing between — by the claim module (`#migState`)
  and the verify module (`#vSealed`), each from its own read.
- **Standby:** ftb hash placeholder → tiles show `—`.
- **Anchors:** MO 5, 6; the token is the supply invariant, and its panel is read-only by
  construction.

### 3. FTB dividends — `mod-earn.js` — owns `#earn`

- **Reads:** `dividends.totalStaked`, `dividends.accPerShare`, `dividends.stakedOf(acct)`,
  `dividends.pendingOf(acct)`.
- **Writes:** `dapi.stakeFtb` (tagged FTB transfer, `["stake"]`), `dapi.claimDividends`,
  `dapi.unstakeFtb`.
- **Rules.** The lede states the mechanism plainly: the holders' 2/9 of every fee
  (Decision 6). The APY is **sampled, never asserted**: it appears only after two
  `accPerShare` samples at least 20 h apart (`localStorage.ftb_div_acc`); before that the
  panel says "sampling — check back tomorrow". Dividends accrue per block, so no "next
  payout" stat may ever be shown — that number would be a lie. Stake/unstake/claim all go
  through the one modal with the plan summary verbatim.
- **Standby:** dividends hash placeholder → `#earnOff` names it.
- **Anchors:** MO 6 (a number you cannot yet measure is not shown); Annex B 11.

### 4. LEND — `mod-lend.js` — owns `#lend`

- **Reads:** `lend.utilizationBps`, `lend.borrowApyBps`, `lend.supplyApyBps`,
  `lend.totalBorrows`/`reserveAccrued`, and per account `debtOf`, `collateralOf` (NEO and
  GAS), `collateralValue`, `maxBorrow`, `isUnhealthy`, `sharesOf`.
- **Writes:** none in the current launch phase. When Launch 2 opens them, only these
  builders, all through the modal: `supply`, `repay`, `borrow`, `withdrawSupply`,
  `withdrawCollateral`, `addCollateralNeo` (whole units), `addCollateralGas`, `liquidate`.
- **Rules.** Lend is the highest-risk module (it is the class of bug that destroyed
  Flamingo), so its panel is read-first: position and health are visible before any
  action exists. The health tag shows `isUnhealthy` verbatim — never a softened
  paraphrase. Collateral display separates NEO (whole) from GAS (base units). Oracle
  dependence is stated, not hidden: if the oracle is paused, borrow-side figures read as
  unavailable, and the panel says why.
- **Standby:** lend hash placeholder → `#lendOff` names it; Launch 2 gate text stays
  until its own dedicated audit pass lands.
- **Anchors:** MO 5, 6; QRN F; the launch-gating discipline of `ARCHITECTURE.md`.

### 5. Governance — `mod-gov.js` — owns `#vote` and the tile `#tVoted`

- **Reads:** `ballot.tally(option)` per configured option (1-based on-chain), per account
  `ballot.voted(acct)`; turnout denominator from `config.governance.electorate`.
- **Writes:** `dapi.ballotVote(account, amount, option, proof)` — the voter's migration
  proof as weight, one vote per snapshot address.
- **Rules.** Turnout is the security metric (Terra Station rule): the hint line always
  reports total weight voted and turnout as % of the snapshot electorate — the genesis
  vote is denominated in FLM (1 FLM = 1 vote, Decision 10); post-migration votes in FTB
  (1 FTB = 1 vote), and the label must say which. Tallies are chain state: the UI must
  never imply a server is counting. An address that has voted sees that state; an address
  outside the snapshot sees "no vote weight" with the reason. Options render in on-chain
  order (1-based — option 0 faults on-chain, FTB-31).
- **Standby:** no live ballot (`question` empty) → "no live ballot" + how a ballot works.
- **Anchors:** MO 6, 8; QRN C (counting is replicated state, not a service).

### 6. Proposals — `mod-board.js` — owns `#board`

- **Reads:** `registry.proposalBond`, `registry.proposalCount`, `registry.proposal(id)`
  (newest first), `registry.escrow`.
- **Writes:** `dapi.submitProposal(account, kind, ref, meta, bond)` — a tagged FTB
  transfer of exactly the bond to the registry, kinds `ballot` | `listing`.
- **Rules.** The bond rule is printed next to the form at all times: exact, escrowed,
  refunded on a legitimate close, kept against spam. `meta` is validated (non-empty,
  length-capped by the input) and `ref` must be a `0x` 20-byte hash before a plan is
  built. The board is replicated chain state — rendered newest-first, never reordered,
  never editorialized. A bond read failure blocks submission with the reason, not a
  guessed amount.
- **Standby:** registry placeholder → the standby entry naming it.
- **Anchors:** MO 5, 6; QRN A (bonded admission is the anti-gatekeeper: spam pays).

### 7. Claim — `mod-claim.js` — owns `#claim`

- **Reads:** `ftb.migrationOpen`, `ftb.totalSupply`, `ftb.maxSupply`,
  `migrator.claimed(acct)`; the artifact `proofs.json` (lazy, once).
- **Writes:** `dapi.claim(account, amount, proof)`.
- **Rules.** Entitlement, root, and holder count come from `proofs.json` itself — never
  hard-coded (WEB-03). The claimable preview uses the on-chain conversion: read
  `migrator.quote(amount)` (the canonical implementation); the JS mirror `ftbOf` is a
  documented offline fallback only, pinned by a KAT, and the preview always labels that
  the chain converts authoritatively at mint time (WEB-02). One claim per address, ever;
  "not in snapshot" and "already claimed" are first-class states shown with reasons. The
  proofs lookup index is built once at load (no repeated O(n) scans, WEB-05).
- **Standby:** proofs unreachable → "proofs unavailable" naming the URL.
- **Anchors:** MO 5, 6; Annex B 7 (the mirror must be pinned); QRN E (the exit artifact
  is published and reproducible).

### 8. Verify — `mod-verify.js` — owns `#verify` and the mirrors tile `#tMirrors`

- **Reads:** every hash from `config.js` (displayed verbatim), `ftb.maxSupply`,
  `ftb.migrationOpen`; `beat.json` from self and every configured mirror.
- **Writes:** none.
- **Rules.** The provenance grid shows token, migrator, registry, ballot, merkle root,
  cap, and seal state, with the standing instruction to check them against the repo
  before signing — the page assumes a hostile mirror is the realistic attack (THORSwap
  2023). A mirror counts as live only if its `beat.json` is reachable AND carries this
  exact commit; reachable-but-older is stale, and the hover breakdown shows all three
  states per mirror. Nothing in this module may be interpolated, shortened without an
  ellipsis marker, or colored by opinion.
- **Standby:** beat.json absent → mirrors tile `—`.
- **Anchors:** MO 6; QRN D (reproducibility) and B (mirrors are censorship defense).

### 9. Wallet connect — `mod-wallet.js` — owns `#connectBtn`, `#idstate`, and the account half of the state seam

- **Reads:** `dapi.init()` detection, `dapi.connect()` on click, `accountChanged` /
  `networkChanged` events.
- **Writes:** none of its own — it signs nothing; it supplies the account every other
  module's plans charge.
- **Rules.** Connection state is chrome, not content: it lives in the header, never in a
  hero. The state line is always truthful: `[ no wallet ]`, the short address with the
  network, `[ connect first ]`, `[ install NeoLine or a dApi wallet ]` — no other strings.
  Other modules nudge toward connecting only through the wallet module's
  `FtbWallet.needConnect()` seam; they never write `#idstate` directly (R1).
  The wallet strip (balances) appears only while connected. Account changes propagate
  through the state seam (R7) and every module re-reads for the new account; no module
  may cache account-scoped data across a change.
- **Standby:** no dApi wallet → the install hint; the page stays fully readable
  disconnected.
- **Anchors:** MO 8 (the signing boundary); QRN B (reads never require a wallet).

### 10. Network / RPC seam — `ftb-rpc.js` — owns the RPC list, failover, the own-node modal `#ovRpc`, and `#rpcNote`

- **Reads:** `getblockcount` (probes), `invokefunction` (reads it serves to modules).
- **Writes:** none.
- **Rules.** THE ONE read stack in the page (retires the duplicated stacks, WEB-01): one
  `rpcCall`, one tip-aware `pick` (`SYNC_SLACK` — a syncing node never serves reads), one
  `readFn`/`readInt`/`readBool`. Community nodes lead the list; public endpoints are
  appended as fallback and are named when they win ("public fallback — community node
  unreachable or still syncing"). A user-pastable node is validated (`getblockcount`
  answers) before use, stored only in `localStorage.ftb_rpc`, and labeled "(your node)".
  Failover affinity is a recorded decision (WEB-06): first at-tip endpoint serves reads.
- **Standby:** no healthy endpoint → `rpc: no healthy RPC node answered — paste your own
  node`, and every module degrades per R8.
- **Anchors:** MO 3, 10; QRN B (a user on their own node cannot be censored).

### 11. The adapter — `ftb-dapi.js` — owns the wallet dApi and every chain primitive

- **Role:** the Article 10 shared toolkit: one canonical implementation of every builder,
  reader, decoder, and byte/address primitive. Pages and modules consume it; none
  re-implement it. (The pure-JS SHA-256 + full base58check live here and only here.)
- **Rules.** Builders stay pure: build, expose `.summary`, sign nothing. Reads carry no
  signer. New chain capabilities enter as adapter builders first — a module may never
  hand-assemble an invocation. Every builder's summary is a plain sentence a user can
  check against the wallet's own display. The NEP-21 path stays informational until it is
  a module-level decision recorded in DECISIONS.md.
- **Anchors:** MO 2, 8, 10; `invocations.md` is its spec and must be updated in the same
  change as any builder.

### 12. Shell — `shell.js` — owns the header/nav, theme, hero, launchbar countdown, footer, the review modal `#ov`, and the page grid

- **Rules.** The shell owns the theme tokens and the phosphor key (only clickables are
  green — nothing else, ever). The countdown is a dated public commitment: one target
  (MainNet window, 7 Nov 2026 00:00 UTC), self-contained, no chain; at zero it flips to
  the window-open notice and stops — it never counts past. The footer carries the audit
  posture verbatim ("internal audit lineage only — no external audit was performed") and
  may never be softened. The one review modal (R6) renders any module's plan in the same
  ceremony; consistency is what trains users to read it. The nav is plain words, five to
  seven anchors; a sixth destination means the information architecture is wrong — fold,
  don't add.
- **Anchors:** MO 6; `UI_RATIONALE.md` invariants; Annex B 11.

### 13. Config — `config.js` + `beat.json` — owns every deployment constant

- **Rules.** Every contract hash, RPC endpoint, mirror, pair definition, electorate
  figure, ratio, and the ballot question enters through this one file — no constant may
  live inside a module (R5). Placeholders are `0x0000…` and every module treats them as
  standby (R8). `config.js` is git-ignored; `config.example.js` is the committed template
  and must stay field-identical in shape. `beat.json` is written per deploy by the push
  script and carries `{project, commit, published}` — it is how the verify module proves
  a mirror serves this exact build. A config value that contradicts a published artifact
  (root, electorate, count) is a defect at gate time, not a display bug (WEB-03's class).
- **Anchors:** MO 6, 8; QRN D.

## Definition of done (web)

The web layer is done when: every module above is its own file and passes R2 (runs with
its panel alone); no module fetches, signs, or hard-codes outside the rules; `invocations.md`
and this file agree with the adapter; and the whole folder pins to IPFS with a `beat.json`
that the verify module confirms live on at least two mirrors.

**Landed 18 October 2026:** the module split itself — WEB-01, WEB-02, WEB-03, and WEB-05
are closed in code (see `WEB_AUDIT.md` § Remediation). Remaining: WEB-04 (template
archive) and the two-mirror `beat.json` confirmation at the next publish.
