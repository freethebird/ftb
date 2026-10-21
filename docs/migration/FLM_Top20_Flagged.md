# FLM top 20 holders, flagged for FTB eligibility

Pre-exploit snapshot (block 12,782,704, one before the mint), pulled live from the chain. The
question this answers: of the biggest FLM holders, which are Flamingo infrastructure, exchanges or
desks, or team/treasury wallets that should not receive FTB as "community supporters," and which
look like real holders.

## Method

Top holders came from NeoTube's holder-rank API; pre-exploit balances came from the COZ archive RPC
(`invokefunctionhistoric balanceOf` at block 12,782,704); contract identity came from
`getcontractstate` manifest names; wallet character came from each address's asset mix and GAS/NEO
holdings. Pre-exploit FLM supply was 574,708,785 and FLOCKS 122,346,444, so the percentages below
are shares of the real, pre-exploit supply. The "$GAS AIRDROP" and "NGASX" entries that show up on
many wallets are spam tokens and were ignored.

One honest limit: the candidate set is the current top ~30 holders checked backward to the snapshot
block. That captures every stable large holder (contracts, exchanges, team), which is what matters
here, but a pre-exploit whale who fully sold after the exploit would not appear. The reproducible
`snapshot_tool.py` run is still the complete source of truth; this is the fast, targeted read.

## The result in one line

Of the top 20, eleven are Flamingo's own contracts (already auto-excluded by the snapshot tool), and
nine are wallets. Of those nine, the standout is wallet #2, which held 85.1M FLM (14.8% of supply)
plus 20.4M FLOCKS (16.6%) with almost no other activity, the signature of a team or treasury wallet.

## Top 20 by pre-exploit FLM

| # | Address | Pre FLM | % | Pre FLOCKS | What it is | Exclude |
|---|---------|--------:|--:|-----------:|------------|---------|
| 1 | NhVnp...RrNdaH | 118.2M | 20.6% | 0 | FlamingoSwapPair (pool) | yes, contract |
| 2 | NTRGiig...ZDwp8 | 85.1M | 14.8% | 20.4M | wallet, likely team/treasury | yes |
| 3 | NMQSkz...6U3nAt | 44.8M | 7.8% | 0 | FTokenVault (lending) | yes, contract |
| 4 | NSy3Gffa...Cg4KE | 35.4M | 6.2% | 2.4M | FlamingoBroker | yes, contract |
| 5 | NiU7ic...s2V3DS | 20.1M | 3.5% | 0 | wallet, dormant allocation (3 tx) | yes |
| 6 | NTWC7H...Q1Ej64 | 18.4M | 3.2% | 0 | wallet, likely CEX / market maker | yes |
| 7 | Nf7xBD...HWJ455 | 16.3M | 2.8% | 0 | FlamingoSwapPair (pool) | yes, contract |
| 8 | NWu2ng...Y3pnVM | 15.5M | 2.7% | 1.0M | wallet, likely team/allocation (round) | yes |
| 9 | NUfgAZ...Er7ifm | 13.5M | 2.4% | 0 | FTokenVault (lending) | yes, contract |
| 10 | NLdYPX...eMg93t | 13.0M | 2.3% | 0 | wallet, likely CEX / market maker (421k NEO) | yes |
| 11 | NXF8xB...X9mcVH | 10.4M | 1.8% | 0 | wallet, likely CEX / market maker | yes |
| 12 | NQSbtd...cP9JUC | 8.0M | 1.4% | 0 | wallet, allocation/OTC (2 tx) | yes |
| 13 | NSuUJy...Tu4KwC | 7.3M | 1.3% | 0 | FLOCKS (contract) | yes, contract |
| 14 | NLXTWh...QjwFjH | 7.1M | 1.2% | 0 | FlamingoSwapPair (pool) | yes, contract |
| 15 | NUN32i...we4ZZ | 7.0M | 1.2% | 5.0M | wallet, likely team/investor | yes |
| 16 | Nfps2r...Y4S4a | 6.3M | 1.1% | 0 | wallet, likely real DeFi user | review / keep |
| 17 | NRwXs5...pV7Mb | 6.0M | 1.0% | 0 | FlamingoSwapPair (pool) | yes, contract |
| 18 | NaHC6x...xyTztD | 5.2M | 0.9% | 0 | FlamingoSwapPair (pool) | yes, contract |
| 19 | NanYZR...rZqxdZ | 5.1M | 0.9% | 0 | FLUND (staking) | yes, contract |
| 20 | NRj5m5...QEGX4G | 4.0M | 0.7% | 0 | wallet, desk-like (46k NEO) | review |

## The groups

Flamingo internal (eleven of twenty): the swap pairs, the Broker, the two FTokenVaults, FLUND, and
the FLOCKS contract. These hold the bulk of FLM because that is where pooled and staked FLM lives.
They are contracts, so `snapshot_tool.py` already drops them (`EXCLUDE_CONTRACTS = True`). No action
needed beyond confirming the auto-exclusion ran.

Team / treasury / allocation wallets (the ones to decide on): #2 (85.1M FLM + 20.4M FLOCKS) is the
clearest, a treasury-sized position in both tokens with few counterparties and nothing else held.
#5 (20.1M, three lifetime transactions), #8 (round 15.5M + 1.0M), #12 (round 8.0M, two
transactions), and #15 (7.0M + 5.0M FLOCKS) all show the same pattern: large, round or dual-token,
nearly inactive. On-chain behavior cannot prove these are the team or NGD, but the pattern is
exactly what insider or foundation allocations look like, and the community should ask for a public
disclosure of these addresses before deciding.

Exchanges / market makers: #6, #10, and #11 hold large NEO and GAS alongside ecosystem tokens like
SWTH, TIPS, and MCC, with thousands of transactions. Those are liquidity desks or exchange wallets,
not individual supporters, so FTB sent there would land in a pooled account, not with people. #20 is
a smaller version of the same and is marked for review.

Looks like a real holder: #16 holds a spread of Flamingo wrapped assets (FWETH, FWBTC, FUSDT) with
low activity, which reads like a genuine early liquidity provider. Keep unless something else turns
up.

No wallet here is provably NGD from the chain alone. If NGD or the Foundation holds FLM, it is most
likely one of the dormant large wallets above; only a disclosure or a labeled address list can
confirm which.

## Recommendation

The contracts need no action (auto-excluded). For the wallets, the safe and transparent path is to
add the clear non-community ones to `EXCLUDE_ADDRESSES` in `snapshot_tool.py`, publish the list with
this reasoning, and invite challenge. Proposed additions, pending community agreement:

```
# exchanges / market makers (pooled user funds, not individual supporters)
NTWC7Hh5VYMQ5K8YJbyCLbmJ4RhfQ1Ej64
NLdYPXACYPKa6zTvoThPuJxd3uEmeMg93t
NXF8xBeBaXSrrvTAHWK6xuGTyutAX9mcVH
# large team/treasury/allocation pattern (ask for disclosure first)
NTRGiigCG7M3RsyBJRc1WoZDV2cfvZDwp8
NiU7ic68RHF4oo5QK8Li5JbXx22rs2V3DS
NWu2ng1hBxU6w6AUmGy4QDkscQTXY3pnVM
NQSbtd1YYCe1d6gHubwt275QzNTbcP9JUC
NUN32id3LKcSTEcfyiGCBKg6puCijwe4ZZ
# review before deciding
# NRj5m5jLDuJVsmGZwdZSeiFYzJfaQEGX4G   (desk-like, confirm)
# Nfps2rFwD9VZ8kd6GRkgWix8LH8A5Y4S4a   (looks like a real LP, likely keep)
```

Excluding the three exchange/desk wallets and the five team/allocation wallets removes roughly 173M
FLM and 26M FLOCKS of pre-exploit balance from the distribution, which is a large share and exactly
why it should be a published, challengeable decision rather than a silent one.

Full per-wallet data is in `FLM_top20_flagged.csv`.

## Sources

NeoTube holder-rank API (`api.neotube.io/v1/rank`), COZ archive RPC
(`mainnet1.neo.coz.io`, `invokefunctionhistoric`), contract names via `getcontractstate`. All
figures are at block 12,782,704 and verifiable by re-running `snapshot_tool.py`.
