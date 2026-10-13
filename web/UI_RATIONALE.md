# UI rationale: where each decision comes from

`index.html` is a typewriter/terminal page that borrows battle-tested DEX patterns without
borrowing their chrome. This file maps every design decision to its source, so future
contributors extend the page without breaking the discipline. The rule of the house: strict
grayscale, one column, no dependencies, no build step, no backend, nothing leaves the
browser except signed transactions and public RPC reads.

## The sources

| source | what it does well | what we took |
|--------|-------------------|--------------|
| Hyperliquid | data density without clutter; the numbers never hide; every trade is previewed before signing | the ticker, the wallet strip, the review-before-sign modal discipline |
| THORSwap | swap reduced to its skeleton: pick assets, enter amount, read the quote, review, send | the swap ticket: two rows, live quote, rate + min received + price impact |
| Terra Station | governance legibility: status, turnout, quorum awareness at a glance | turnout vs the full snapshot supply on every ballot; share-of-vote bars per option |
| NEAR | plain words, no chrome, generous whitespace | the five-word anchor nav; one column; nothing decorative that is not structural |

## Decision map

### Command bar (`header.bar`)
- **Source:** NEAR's plain-word navigation + Hyperliquid's always-on status.
- Five anchors — `market · claim · swap · vote · verify` — collapse on mobile
  (`@media max-width:460px`). The wallet state and connect button live here, not in a hero,
  because connection state is chrome, not content.
- **Do not** add icons, dropdowns, or a hamburger. If a sixth destination is ever needed,
  the information architecture is wrong — fold it into an existing section.

### Ticker (`#ticker`)
- **Source:** Hyperliquid, where price is never more than zero glances away.
- One line: each pair's ratio with a ▲▼= drift mark, plus the live block height.
  Monospace, `overflow-x:auto`, no animation — a ticker that scrolls itself is a distraction,
  and motion is reserved for the blinking cursor.
- Fed by the same `getReserves` reads as the market board; never a separate data path.

### Market board (`#market`, `.rcard`)
- **Source:** Hyperliquid's market list, flattened into cards.
- One card per pair. Big display-font number: `1 bNEO = 12.3457 FTB`, both directions shown
  so nobody divides in their head. Sparkline + drift + sample count, history in
  `localStorage` only.
- Ratio only, asset against asset. **No fiat conversion ever** — a fiat price needs an
  oracle, and an oracle is a trust assumption this page does not make.

### Wallet strip (`#balStrip`)
- **Source:** Hyperliquid's funds panel, reduced to one line.
- Appears only after connect: FTB, bNEO, GAS balances and the claimable amount (with
  `claimed ✓` state). Hidden until then — a disconnected visitor has no wallet section.

### Swap ticket (`#swap`, `.ticket`)
- **Source:** THORSwap's select → quote → review flow; Hyperliquid's preview box.
- Two rows (you pay / you receive), asset pill selectors that auto-flip when both sides
  would collide, debounced live quote from pool reserves (0.3% fee, constant product),
  min received at 0.5% slippage, and a price-impact figure that draws a dashed warning box
  past 5%.
- Nothing signs here. `review & sign` opens the modal with the plan in plain language; the
  wallet signs. This mirrors the contract boundary: the page builds, the wallet signs.

### Governance (`#vote`, `.gopt`)
- **Source:** Terra Station's proposal lists — the insight that **turnout is the security
  metric**. A low-turnout quorum capture only works when nobody is watching participation.
- Every option is a card: name, tally in FTB, a solid-ink share-of-vote bar, its own vote
  button. The hint line reports total weight voted **and turnout as % of the full snapshot
  supply**, so a quiet capture attempt is visible at a glance.
- Votes reuse the migration Merkle proof as weight; tallies are chain state, so every node
  counts identically. The UI must never imply a server is counting.

### Verify panel (`#verify`, `.prov`)
- **Source:** our own threat model, not a reference site — THORSwap's 2023 phishing wave is
  the cautionary tale: a hostile mirror of this page is the realistic attack.
- Every contract hash, the Merkle root, the cap, and the mint-sealed state in one
  provenance grid, with the instruction to check them against the repo before signing.

### Review modal (`#ov`)
- **Source:** Hyperliquid's order preview: the last screen before money moves.
- Every write — claim, swap, vote — renders the same modal: what you do, the numbers, and
  `what you sign` verbatim from the plan summary. One modal for everything keeps the
  signing ceremony consistent; consistency is what trains users to actually read it.

## Invariants (do not break)

1. No dependencies, no build step, one HTML file plus `config.js` / `ftb-dapi.js`.
2. Strict grayscale, both themes; accent through weight and borders, never color.
3. No fiat prices, no external APIs, no analytics, no fonts beyond the inlined VT323.
4. The page never holds keys and never implies custody. Builds and reads here; signs in
   the wallet.
5. Motion is the blinking cursor. Everything else is still.

## Design lab outcome: Terra Station is canonical (12 October 2026)

Three complete variants were built and screened — Hyperliquid (dense two-column terminal with
inverted ink bands), THORSwap (centered swap-hero), and Terra Station (governance front page).
**Terra Station won and is now the canonical `index.html`; the lab is retired.** What carried
it: this project's defining act is the community vote and the claim, not day trading — the
proposal leads in display type inside a framed hero panel, turnout is the headline number, and
trading sits beneath as a utility. The loser's lessons survive in the parts that were already
canonical before the lab: the ticker and wallet strip (Hyperliquid) and the ticket's
select → quote → review flow (THORSwap).

## Template 2: station dashboard (12 October 2026)

Template 1 (`template1.html`, frozen) kept the template 1 single-column arc even after the Terra
skin promotion — at narrow widths and standby states the variants read as "the same page".
Template 2 (`template2.html`) is designed from scratch instead of re-skinned: a left rail
with plain-word vertical nav (NEAR) that collapses to a top strip on narrow panes, an
inverted governance hero panel where the proposal is the headline (Terra), four stat tiles
(supply / cap / weight voted / block) and the ticker for always-on numbers (Hyperliquid),
and a two-column working area with the market board left and the wallet strip + swap ticket
right (THORSwap). Same ids, same signing flows, same invariants; `build_template2.py`
assembles it from template 1's font, component CSS, modals, and script, so the two never
drift in behavior — only in layout.
