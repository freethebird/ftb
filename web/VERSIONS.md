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

## Version map (locked baseline: v0.10, 20 October 2026, commit 708b4f5)

| UI module | Tag location | File(s) | Version |
| --- | --- | --- | --- |
| header strip (supply / cap / voted / block / mirrors / tvl / price) | tiles (manifest only) | `js/mod-token.js` | v0.11 |
| swap | swap ticket tab | `js/mod-swap.js` | v0.11 |
| tokens on ftb | market card header | `js/mod-swap.js` | v0.11 |
| swap liquidity | LP section header | `js/mod-swap.js` | v0.11 |
| ftb dividends | earn card header | `js/mod-earn.js` | v0.10 |
| lend | lend card header | `js/mod-lend.js` | v0.10 |
| governance | governance card header | `js/mod-gov.js` | v0.10 |
| proposals | board card header | `js/mod-board.js` | v0.10 |
| claim | claim card header | `js/mod-claim.js` | v0.11 |
| verify | verify card header | `js/mod-verify.js` | v0.10 |
| wallet | menu (manifest only) | `js/mod-wallet.js` | v0.10 |
| your node | node panel header | `js/shell.js` | v0.10 |
| rpc seam | manifest only | `js/ftb-rpc.js` | v0.11 |
| state seam | manifest only | `js/ftb-state.js` | v0.10 |
| wallet adapter | manifest only | `ftb-dapi.js` | v0.11 |

Locked backups: `web/js/backup/v0.10/` (all 13 files, byte-identical to the
published commit above).

## History

| Version | Date | Commit | Change |
| --- | --- | --- | --- |
| v0.10 | 2026-10-20 | 708b4f5 | First locked baseline: R4 stack live, first-deposit LP entry, claim/vote/lend/dividends tested on TestNet |
| v0.11 | 2026-10-20 | (this commit) | Module audit pass (WEB_AUDIT.md addendum 20 Oct): RPC 10s timeout, swap/token read coalescing, claim pre-checks the token's claimed ledger, dead vote() builder removed. LOCKED 20 Oct - acceptance passed on-chain: GAS/FTB seeded, first swap landed (pool moved 40.00 -> 33.35 FTB/GAS, reserves and TVL painted live) |
