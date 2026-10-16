# Pre-exploit snapshot: reproduction and evidence

Everything needed to independently reproduce the FTB migration claim list.

## What this is

The FTB migration claims are computed from the FLM + FLOCKS holder state at **block
12,782,704**, one block before the exploit mint (tx
`0x5e310782aac17676a80572e073ba0d70a7d533caff60b38a4e23740ddec22030`, block 12,782,705).

- Claim list (4,830 wallets): [`../ftb_claim_list.csv`](../ftb_claim_list.csv)
- Findings and classifications: [`../FTB_Snapshot_Findings.md`](../FTB_Snapshot_Findings.md)
- Full top-20 ownership analysis: [`../FLM_Top20_Ownership.md`](../FLM_Top20_Ownership.md)
- Wallet #15 deep dive: [`../FLM_15_NUN32i_DeepDive.md`](../FLM_15_NUN32i_DeepDive.md)

## Reproduce

```
python3 snapshot_state.py
```

The script walks the FLM and FLOCKS balance storage at the snapshot state root
(`0x86a3042d22bfc7d0d847a476cd145460c50d436881b53ea4037de6ac4a69baf7`) via `getstateroot` /
`findstates` against any Neo N3 archive node with StateService enabled (default:
`https://mainnet1.neo.coz.io:443`).

It **fails loudly** unless the reconstructed balances sum exactly to the on-chain
total-supply anchors:

| Token | Raw supply anchor | Human |
|-------|------------------:|------:|
| FLM | 57,470,878,533,054,587 | 574,708,785.33 |
| FLOCKS | 12,234,644,406,057,840 | 122,346,444.06 |

An exact match makes the holder list complete by construction: every token that existed
at the snapshot block is accounted for.

Storage layout (both contracts, values are signed little-endian integers, 8 decimals):

- FLM (id 26): balance keys `01 01 <20-byte big-endian scripthash>`
- FLOCKS (id 449): balance keys `09 <20-byte big-endian scripthash>`

## Evidence files (`evidence/`)

| File | Contents |
|------|----------|
| `flm_entries.json`, `flocks_entries.json` | every non-zero balance entry at the snapshot |
| `contract_flags.json` | which snapshot entries are contracts (auto-excluded) |
| `flag_top12.json` | evidence behind the original top-12 flags |
| `wallet_evidence.json`, `top20_probe.json` | per-wallet classification probes |
| `deep_11_full.json`, `deep_11_20_raw.json`, `deep_11_counterparties.json` | full transfer histories from the #11-#20 sweep (desk network) |
| `exact_funded_check.json` | the CEX-withdrawal test on the desk-funded wallets |

All transfer data was pulled from public RPC (`getnep17transfers`) on 2026-10-14/15/16.

*On-chain measurement, not legal or financial advice.*
