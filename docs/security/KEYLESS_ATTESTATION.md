# FTB - On-chain No-Keys Attestation (Neo N3 MainNet)

**As of:** 7 November 2026, read live from `https://mainnet1.neo.coz.io:443`.
**Purpose:** the honest, replayable proof of which FTB contracts hold keys and which do not. This is S6 (verification depth) and S10 (transparency) turning from design claim into on-chain evidence. Every line below is checkable by anyone with an RPC endpoint; the exact calls are in the last section.
**Reads with:** [`GOV_RENOUNCE_MILESTONE.md`](GOV_RENOUNCE_MILESTONE.md) (the plan to close the governance stack), the repo decision log [`DECISIONS.md`](../../DECISIONS.md), and the frozen NEF set [`NEF_RELEASE.sha256`](../../NEF_RELEASE.sha256).

## Headline

The public surface is keyless or seal-only today. The claims path, the swap, the loanbook, and the registry hold no reachable admin key. The one residual key on the public surface is the migrator's `close`, which is grief-only and cannot mint or move value. The governance and earn stack is deliberately still owner-held and is dark on the site until its own audit and soak (see [`GOV_RENOUNCE_MILESTONE.md`](GOV_RENOUNCE_MILESTONE.md)).

Two facts a skeptic must check, both proven below: the FTB token's ABI still declares an `update` method, and it is dead because the owner is renounced; and the dev deploy wallet holds zero FTB, so there is no admin key and no privileged economic stake.

## Live evidence base (this read)

- Token renounce tx `0xf5ef992146c102f2276f1a3c41eb46716e252baa3e3387aab08ac1a3336f284a`: vmstate **HALT**, event **OwnershipRenounced** emitted from the token contract.
- Every FTB contract: **updatecounter = 0** (never upgraded since deploy).
- Economic seals, live: migrator ratioDen 57,745,832,920,225,836, ratioNum 10,000,000,000,000,000; token maxSupply 10,000,000,000,000,000 (100M); deadline 1,809,756,704,750 ms = **2027-05-08T06:11:44Z**; migrationOpen true; totalSupply **49,981,979.83 FTB** (49.98% of cap); registry finished true, delay 11,520 blocks (about 2 days).
- Dev deploy wallet `NbqbDM5mc3geMfmuq4ygbQjZHqbNKoiLRe` (`0xdfce1c9c64c4bfd1aae44235804bdb60c501aeae`): **FTB balance 0**.

## Per-contract attestation

### Keyless core (the public surface)

**FTB token** `0xf85f93d672cbf79f7749fb50a2d46a48421d7e71` - RENOUNCED / KEYLESS.
updatecounter 0. The ABI declares `update`, `setMigrator`, `setClaimWindow`, `closeMigration`, and the CM permission allows `update`. All of these are owner-gated, and the owner is gone: the renounce tx above HALTed and emitted OwnershipRenounced, the owner slot is empty, and a second renounceOwnership faults. Every owner-gated path therefore faults for want of an owner to authorise it. Minting runs only through the migrator (`mintForMigration`, permissioned to the migrator) and self-seals at the deadline (2027-05-08). The presence of `update` in the ABI is a false positive for upgradeable: it is dead code gated on an empty owner, and the renounce tx is the proof.

**FtbMigratorR6** `0x51a82390734d9d38cc89fed6bab26e72419c7ad2` - IMMUTABLE / SEAL-ONLY.
updatecounter 0, no update or destroy method, and its CM permission only calls into the token (`mintForMigration`, `closeMigration`, `claimed`). It cannot upgrade itself. `configure` is write-once and spent (ratioDen and ratioNum are live and non-zero). The one residual key is `close`: grief-only, held by NbqbDM5, able to halt migration early but unable to mint, move, or redirect value. This is the documented seal-only residual, and it is value-safe by construction.

**FtbRegistry** `0xb796337672703c7d111364f3acf8dea54f7a714e` - IMMUTABLE / TIMELOCK SEAM, no owner.
updatecounter 0, no update method, CM permission only `transfer`. `finished` is true and `delay` is 11,520 blocks, so the bootstrap-phase setters are closed and the registry operates permissionlessly via propose and apply under a 2-day timelock. It has no owner and no renounce method; the timelock is a structural seam, not a key.

**FtbLoanBook** `0x19414f2c1b04bd2fd705d065887ec7cbdc244aaf` - FULLY KEYLESS / IMMUTABLE.
updatecounter 0, no update, no owner, no configure, no admin method of any kind. FTB was injected at deploy. `close` here is a per-position action, not an admin control. Nothing to renounce because nothing was ever keyed. The strongest card in the set.

### Governance and earn stack (deliberately owner-held, dark on the site)

**FtbDividends** `0x5066582708c4d273b3a7b7eadf8dc94855e4c1a8` - OWNER-HELD (self-timelocked upgrade), not yet keyless.
updatecounter 0. No plain `update` method, but a live self-timelocked upgrade path exists: `proposeUpgrade` then `pendingUpgradeEta` then `applyUpgrade` (CM permission includes `update`), owner-gated. `renounceOwnership` is present and uncalled; when called it drops the upgrade key and leaves pure immutable accounting. Honestly upgradeable today.

**FtbGov** `0x277095a7bc8567b4040ecf7b6253767d1451192f` - OWNER-HELD (bind key), immutable code.
updatecounter 0, no self-update path (CM permission only reads `dividends.stakedOf` and `totalStaked`), so the code is immutable. The owner key is the one-time bind, `configure(dividends)`. `renounceOwnership` is present and uncalled; the owner binds dividends once and then renounces.

**FtbTreasury** `0x64f8fe20340211954ace5a99f63a05663c9a280c` - DE-KEYED to gov, immutable, no owner key.
updatecounter 0, no update, no owner, no renounce. The privileged actions (`createEscrow`, `setAuditor`, `cancel`, `returnExcess`) each require `passed(govID)` from FtbGov plus a recomputed-subject match and a consumed anti-replay guard; `configure(ftb, gov, registry)` is write-once. There is no owner key at all: authority is a FtbGov vote. Its keylessness completes only when its trust root, FtbGov, renounces.

**FtbBondVault** `0xc77d27432a2f82b4aeb30b81bbc5e012a04ddf1c` - DE-KEYED to gov, immutable, no owner key.
Same shape as treasury: gov-bound (`registerListing`, `slash`, `delist` via `passed(govID)`), `configure(ftb, gov)` write-once, no owner, no update, no renounce. Keyless pending the gov renounce, like treasury.

## Summary

| Contract | Upgradeable? | Owner key | Residual | State |
|---|---|---|---|---|
| FTB token | no (owner renounced; `update` dead) | none | none reachable | keyless |
| FtbMigratorR6 | no | none | `close` (grief-only, value-safe) | seal-only |
| FtbRegistry | no | none | 2-day timelock seam | keyless |
| FtbLoanBook | no | none | none | keyless |
| FtbDividends | self-timelock upgrade (live) | yes (uncalled renounce) | upgrade key | owner-held |
| FtbGov | no | yes (bind key, uncalled renounce) | bind key | owner-held |
| FtbTreasury | no | none (authority = gov vote) | inherits gov | de-keyed to gov |
| FtbBondVault | no | none (authority = gov vote) | inherits gov | de-keyed to gov |

Economic neutrality: the dev deploy wallet holds zero FTB and no LP position. The team has no admin key on the public surface and no privileged economic stake; community-seeded liquidity is the only path consistent with that.

## How to verify (replay this)

```
RPC=https://mainnet1.neo.coz.io:443
# 1. token basics + migrator wiring
curl -s $RPC -d '{"jsonrpc":"2.0","id":1,"method":"invokefunction","params":["0xf85f93d672cbf79f7749fb50a2d46a48421d7e71","getMigrator",[]]}'
# 2. the renounce event (expect vmstate HALT + OwnershipRenounced)
curl -s $RPC -d '{"jsonrpc":"2.0","id":1,"method":"getapplicationlog","params":["0xf5ef992146c102f2276f1a3c41eb46716e252baa3e3387aab08ac1a3336f284a"]}'
# 3. any contract's manifest + update counter (expect updatecounter 0; inspect abi.methods, permissions)
curl -s $RPC -d '{"jsonrpc":"2.0","id":1,"method":"getcontractstate","params":["0x51a82390734d9d38cc89fed6bab26e72419c7ad2"]}'
# 4. dev wallet FTB balance (expect 0)
curl -s $RPC -d '{"jsonrpc":"2.0","id":1,"method":"invokefunction","params":["0xf85f93d672cbf79f7749fb50a2d46a48421d7e71","balanceOf",[{"type":"Hash160","value":"0xdfce1c9c64c4bfd1aae44235804bdb60c501aeae"}]]}'
```

Every contract hash is in the summary table. Pull each one's `getcontractstate` and confirm updatecounter 0, no update entrypoint (except the token's dead owner-gated one), and no owner getter beyond the de-keyed stack's uncalled `renounceOwnership`.

## What this earns, and what it does not

It earns S10 (transparency, keys readable live) and begins to earn S6 (verification depth) for the core: real on-chain evidence, not a design claim. It does not yet earn the rest of S6: there are still no formal proofs, no external audit, and little production time. The sequence is how the remaining S6 is earned, and the gov-renounce milestone is where the governance and earn stack joins the keyless column.

---
*Attestation v1.0, read live 7 Nov 2026. Re-run after any deploy, and in full after the gov-renounce milestone, then publish the updated card.*
