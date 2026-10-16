# Deep Dive — Wallet #15: NUN32id3LKcSTEcfyiGCBKg6puCijwe4ZZ

Date: 2026-10-15 · Scope: full NEP-17 transfer history (961 legs), FlamingoBroker cycles, NEO
flows, monthly aggregation. Method: `getnep17transfers` RPC against
`mainnet1.neo.coz.io:443` (archive + StateService), paginated ≤1000 records per call,
cross-checked against the snapshot state root at block 12,782,704.

Snapshot balances: **6,989,116.71 FLM + 5,030,925.75 FLOCKS**.
Current balances (2026-10-15): **identical** — the wallet has not moved a single token
since the snapshot. NEO balance: 1.

---

## 1. Origin (July 2025)

The wallet was born **2025-07-06**, seeded not with FLM but with
**3,237,662.79 FLOCKS from NT6Zw2** — a wallet that later appears as one of its four
OTC counterparties (the "ring"). From day one it received weekly FLM yield drips
(~5k FLM) from the FLOCKS contract NSuUJy. Early on, the yield was forwarded to
`Na7Th1bprF5FEwh8…` — that address returns **zero legs on current RPC**, so its role
is unresolved (an early forwarder that RPC history no longer surfaces; dead end via
RPC, noted honestly rather than guessed).

## 2. Timeline phases

| Phase | Months | Behavior |
|---|---|---|
| Warm-up | Jul–Sep 2025 | Small balanced relay; yield in, small FLM/NEO out |
| Scale-up | Oct 2025 | 3.88M FLM through the wallet, 23.4k NEO moved |
| **Pass-through peak** | **Nov 2025** | **33.48M FLM in == out exactly** (net zero); 142.8k NEO in/out; +1.79M FLOCKS from Broker |
| Accumulation starts | Dec 2025 | 4.99M FLM in, only 162k out |
| Quiet build | Jan 2026 | +1.9M FLM net |
| **NEO-hub phase** | **Feb 2026** | 133.9k NEO in from the ring (NT6Zw2 50k, NiF2ukj 30.2k, NQSbtd 20k, Nft4JE 13k, NYQVQar 20k); 83k out, incl. 20k each to fresh wallets NgHY57 and NTrujvx |
| Wind-down | Mar 2026 | −50,866 NEO to Nft4JE |
| Dormant | Apr 2026 – snapshot | No meaningful movement; last activity 2026-10-10 |

Two distinct roles are visible: an **FLM pass-through/seller** (Nov 2025) and a
**NEO redistribution hub** (Feb 2026). The same four wallets appear on both sides.

## 3. The OTC ring — payment first, goods days later

FLM deliveries vs. NEO paid to #15, per counterparty:

| Buyer (all active since Jul 2022) | NEO paid to #15 | FLM received | Implied FLM/NEO |
|---|---|---|---|
| NQSbtd (#12) | 45,589 | 5,000,000 | 109.7 |
| NiF2ukj (just below top-20) | 77,661 | 5,000,000 | 64.4 |
| Nft4JE | 30,300 | 3,000,000 | 99.0 |
| NT6Zw2 (the seed wallet) | 95,934 | 4,000,000 | 41.7 |

**Caveat, stated plainly:** the implied prices span 41.7–109.7 FLM/NEO, and there are
reverse NEO flows too (Nft4JE received a net 87,237 NEO back from #15). So this is
not a clean single-price order book. What *is* consistent is the shape: **NEO arrives
first, round FLM amounts leave days later** — the signature of negotiated OTC trades
(or of internal rebalancing dressed as trades). The "OTC sale" classification rests
on the payment-first sequencing, not on price coherence.

## 4. Where the FLM came from

- **Desk NXF8xB (#11, flagged):** 124 legs — the primary FLM source. #15 accumulated
  from the same exchange/MM desk network already flagged elsewhere in the top-20.
- **FlamingoBroker cycles:** 119 legs total. FLM in 33.52M (unlocks/yield), FLM out
  20.48M (locks), FLOCKS in 1.79M. Net effect: the wallet repeatedly locked FLM into
  Broker for FLOCKS yield, then unwound — a yield-farmer loop, not a treasury drip.
- **FLOCKS contract NSuUJy:** weekly yield drips (~5k FLM) over the wallet's life.

## 5. Where the NEO went — the collector

Nearly every NEO received was forwarded **within 1–3 minutes** to
**NPmdLGJN47EddqYcxixdGMhtkr7Z5w4Aos** — a fan-in collection address with **33
distinct NEO senders**. This is the exchange-deposit / cash-out pattern. The same
collector appears twice elsewhere in this investigation:

- the pre-exploit attacker wallet **NWuHQdx** sent NEO there **28 times**;
- **NUWq8bh6** (the trading wallet that funded flagged #8 NWu2ng) cashed 4,993 NEO there.

So #15, the attacker, and the desk network around #8 all share one cash-out endpoint.
That is a venue-level fact (many unrelated users deposit to the same exchange
address), **not** proof of common ownership — but it makes the collector the single
most interesting address on the board for anyone with exchange-side KYC access.

## 6. Feb 2026 NEO redistribution

In Feb 2026 the ring reversed direction: 133.9k NEO flowed **into** #15 from the four
ring wallets plus NYQVQar, and 83k flowed back out — including exactly 20k NEO each
to two **fresh** wallets, NgHY57 and NTrujvx, created that month. Whatever the Nov
2025 trades were, by Feb 2026 #15 was operating as the ring's NEO hub, funding new
wallets from pooled ring NEO. This is the strongest "coordinated group" signal in the
whole trace.

## 7. Posture at snapshot

After Mar 2026 the wallet went quiet. At the snapshot block it held
6,989,116.71 FLM + 5,030,925.75 FLOCKS and it holds exactly that today. The FLOCKS
position (5.03M) makes it the **largest non-contract FLOCKS holder after #2 NTRGiig**.

## 8. What this means for the flag decision

Two readings, both supported:

1. **Sophisticated OTC trader / yield farmer.** Real NEO changed hands first; FLM
   followed. Buyers and seller are all 2022-vintage wallets. Under this reading #15
   (and its buyers) are ordinary, if large, market participants → **unflag**.
2. **Ring treasury with internal bookkeeping.** The seed from NT6Zw2, the inconsistent
   implied prices, the reverse NEO flows, the shared 1-minute cash-out collector, and
   the Feb 2026 hub behavior all fit a coordinated group settling internally →
   **stay flagged**.

Currently flagged (owner's top-12 rule), with the unflag variant costed in
`FLM_Top20_Ownership.md` ("Pending owner decision: the #15 OTC ring"):

- Keep flagged: claim base **190,637,032.24 FLM + 113,932,226.80 FLOCKS** = 304,569,259.04 combined.
- Unflag the ring (#15 + #12 + NiF2ukj): **210,626,148.95 FLM + 118,963,152.55 FLOCKS** = 329,589,301.50 combined.

Same offer as #2: if the owner of #15 comes forward and attests, reclassify on their
word plus this evidence trail.

## Open threads

- **Na7Th1bprF5FEwh8** (early yield forwarder): zero RPC history; identity unresolved.
- **NgHY57 / NTrujvx** (Feb 2026, 20k NEO each from #15): fresh wallets funded by the
  ring hub — worth a first-activity check before launch disclosure.
- **NPmdLG collector:** shared by #15, the attacker, and the #8 funding wallet; only
  resolvable with venue-side data.

---

*Evidence: `snap_state/` (wallet_evidence.json, top20_probe.json, flag_top12.json),
`snapshot_full_state.csv`, RPC `getnep17transfers` pulls taken 2026-10-14/15.
Cross-referenced from `FLM_Top20_Ownership.md`.*
