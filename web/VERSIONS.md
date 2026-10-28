# FTB web - module version protocol

**Status:** law for the `web/` layer, alongside `MODULES.md` (module canon),
`CANONICAL_BIBLE_JS.md`, MO V8, and the QRN threat model. Ratified 20 October 2026.

## The rule

Every UI module carries a visible version tag next to its name. A version that
has been tested and accepted is **locked**: it is never edited in place.

1. **Lock.** When a module version is accepted, the exact files that produce it
   are copied untouched into `web/js/backup/vX.YY/`. That copy is the revert
   point. It is never modified, only replaced by a full-file restore.
2. **Change.** The next change to a locked module bumps the version (v0.10 ->
   v0.11), is developed on the live files, and is published only after the QA
   gate passes. The tag in the markup is updated in the same commit.
3. **Revert.** Restoring a locked version is `cp web/js/backup/vX.YY/<files>
   web/js/` (and the matching markup tag), then the normal publish flow. No
   partial restores: a module reverts as a whole file set.
4. **File-shared modules.** `mod-swap.js` produces three UI modules (swap
   ticket, tokens on ftb, swap liquidity). They share one file, so they share
   one version lineage: a change to any of the three bumps all three tags and
   re-locks together. Splitting the file is allowed only as a planned version
   bump of all three.
5. **Seams are versioned too.** `ftb-dapi.js`, `ftb-rpc.js`, `ftb-state.js`,
   and `shell.js` are locked and backed up under the same protocol; their tags
   live in this manifest only (no UI surface of their own, except `your node`).

## Version map (locked baseline: v0.13, 21 October 2026 - compact redesign)

| UI module | Tag location | File(s) | Version |
| --- | --- | --- | --- |
| header strip (supply+cap / block / mirrors / tvl / price / active ballots) | tiles (manifest only) | `js/mod-token.js` + `js/mod-gov.js` | v0.12 |
| swap | swap ticket tab | `js/mod-swap.js` | v0.12 |
| tokens on ftb | market card header | `js/mod-swap.js` | v0.12 |
| swap liquidity | LP section header | `js/mod-swap.js` | v0.12 |
| ftb dividends | earn card header | `js/mod-earn.js` | v0.11 |
| lend | lend card header | `js/mod-lend.js` | v0.11 |
| governance | governance card header | `js/mod-gov.js` | v0.11 |
| proposals | board card header | `js/mod-board.js` | v0.11 |
| claim | claim card header | `js/mod-claim.js` | v0.12 |
| verify | verify card header | `js/mod-verify.js` | v0.11 |
| wallet | menu (manifest only) | `js/mod-wallet.js` | v0.10 |
| your node | node panel header | `js/shell.js` | v0.10 |
| rpc seam | manifest only | `js/ftb-rpc.js` | v0.12 |
| state seam | manifest only | `js/ftb-state.js` | v0.11 |
| wallet adapter | manifest only | `ftb-dapi.js` | v0.12 |

Note: the compact redesign is a presentation change. mod-gov and mod-board were
rewritten to emit the new structures; mod-swap gained the disclosure pool/LP
menus + the user slippage control; mod-earn, mod-lend, mod-claim, mod-verify,
mod-token are byte-unchanged (markup-only in `index.html`) - their tag bump
records the new shell, not a JS change.

Locked backups: `web/js/backup/v0.13/` (all 13 files + `index.html`, byte-identical
to this baseline). Earlier revert points: `v0.10/`, `v0.11/`.

## History

| Version | Date | Commit | Change |
| --- | --- | --- | --- |
| v0.10 | 2026-10-20 | 708b4f5 | First locked baseline: R4 stack live, first-deposit LP entry, claim/vote/lend/dividends tested on TestNet |
| v0.11 | 2026-10-20 | (this commit) | Module audit pass (WEB_AUDIT.md addendum 20 Oct): RPC 10s timeout, swap/token read coalescing, claim pre-checks the token's claimed ledger, dead vote() builder removed. LOCKED 20 Oct - acceptance passed on-chain: GAS/FTB seeded, first swap landed (pool moved 40.00 -> 33.35 FTB/GAS, reserves and TVL painted live) |
| v0.12 | 2026-10-20 | (this commit) | Security + design pass. Seams: `ftb-dapi.js` binds the executed call to the inspected plan (args↔summary integrity) and validates every script-hash (0x+40hex); `ftb-rpc.js` labels the serving node attested (config set) vs unattested (own/public). Shell: CSP `<meta>` on `index.html`/`mainnet.html`; the mainnet countdown externalised to `mainnet.js` so script-src stays `'self'`. Design tighten: removed ~100 lines of dead CSS (the retired command bar, the Terra grid - nothing sets `data-skin`, and the arc/masthead leftovers), merged the duplicate `:root`/`body`. v0.11 shell backed up to `js/backup/v0.11/index.html`; render verified byte-stable. |
| v0.13 | 2026-10-21 | (this commit) | Compact redesign ported from `proposal1.html` into the live page, chain-wired (no fabricated figures, no hidden stubs - resolves the proposal1 MO-audit truth FAILs). Stat band: supply+cap merged, `weight voted` → live `active ballots` tile (`#tBallots`, written by mod-gov). Governance: serialized ballot disclosure menu (`#ballotMenu`) with per-ballot bars + turnout donut + vote options, from live tallies. Proposals: CSS-toggle ballot|listing create ticket, empty board, max-3 / queue-bond rule (inline-option ballots announced as pending the v2 factory). Swap: grid-area ticket + user `max slippage` control (feeds minOut, LP keeps fixed 0.5%). Tokens-on-FTB + Swap-liquidity: disclosure pool/LP menus (`#poolList`/`#lpList`). Earn/lend/claim/verify: compact menus/anchors, markup-only. CSS design-system folded into `index.html` (dead `.bar` dropped). QA: 14/14 `node --check`, profile pure, R3 clean (coingecko fiat + beat.json only), every module rendered live. |
| v0.14 | 2026-10-28 | (this commit) | Analytics node-status panel. New `js/mod-nodestatus.js` (analytics sub-page; R2/R3/R5/R8/R10 clean, `node --check` pass) renders health badges for the TestNet node, the pre-launch MainNet block-explorer node, and the self-healing watchdog, read from the node's own `/status.json` through a new `FtbState.status()` seam (live heartbeat, never cached — R3). `statusUrl` added to config.testnet/mainnet/example (R5). Dots grayscale + the negative key only — phosphor green stays clickables-only (R10); decorative dots `aria-hidden` (HTML bible 11). State seam bumped v0.10→v0.11. Node side (not in this repo): Caddy serves `/status.json` outside the IPFS bundle, rewritten each 60s by `ftb-status.timer`. Backup/lock of the v0.14 baseline is the release-time step. |
