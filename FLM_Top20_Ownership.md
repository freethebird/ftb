# FLM top 20: person, exchange, or Flamingo itself?

Ownership classification of the top 20 FLM positions at the pre-exploit snapshot
(block 12,782,704), from the exact state enumeration in `snapshot_full_state.csv`.
Evidence: contract manifests (`getcontractstate`), historic NEO/GAS balances at the
snapshot block, current token portfolios, lifetime transfer counts, FLM/FLOCKS
acquisition tracing (`getnep17transfers`, `transferaddress` senders), and public
transfer-leaderboard data (neoanalytics.info). Raw evidence: `snap_state/wallet_evidence.json`,
`snap_state/top20_probe.json`. Supersedes the heuristic pass in `FLM_Top20_Flagged.md`.

## Verdict table

| # | Address | FLM at snap | FLOCKS | Class | Confidence | Flag? |
|---|---------|------------:|-------:|-------|-----------|-------|
| 1 | NhVnpBxS…RrNdaH | 118,238,798 | 0 | **DEX infra** - FlamingoSwapPair (contract) | certain (manifest) | auto-excluded |
| 2 | NTRGiigC…ZDwp8 | 85,103,198 | 20,355,588 | **Person - confirmed whale** (owner-attested 2026-10-10). On-chain pattern: funded by NUqLhf wallet; FLOCKS via Broker lock; ~zero NEO/GAS | confirmed | **keep** |
| 3 | NMQSkzqh…6U3nAt | 44,769,958 | 0 | **DEX infra** - FTokenVault | certain | auto-excluded |
| 4 | NSy3Gffa…Cg4KE | 35,352,487 | 2.4M | **DEX infra** - FlamingoBroker | certain | auto-excluded |
| 5 | NiU7ic68…s2V3DS | 20,083,299 | 0 | **Allocation** - 3 lifetime tx; 15.48M FLM in one transfer from NVUvokc (2026-04-29) + 4.6M from Nb1zQp; dormant | high | **flag** |
| 6 | NTWC7Hh5…Q1Ej64 | 18,431,767 | 0 | **Exchange/MM desk** - top chain-wide GAS mover (neoanalytics); 143k NEO, 134k GAS, SWTH 22.7M, TIPS; heavy two-way flow | high (unnamed venue) | **flag** |
| 7 | Nf7xBDaq…HWJ455 | 16,250,148 | 0 | **DEX infra** - FlamingoSwapPair | certain | auto-excluded |
| 8 | NWu2ng1h…Y3pnVM | 15,500,000 | 1,000,000 | **Allocation (curated)** - accumulated via pool buys + FLUND unstakes, swept to exact round 15.5M / 1.0M; nothing else held | medium-high | **flag** |
| 9 | NUfgAZ2k…Er7ifm | 13,531,784 | 0 | **DEX infra** - FTokenVault | certain | auto-excluded |
| 10 | NLdYPXAC…eMg93t | 12,961,268 | 0 | **Exchange/MM desk** - ~416k NEO (one of the largest NEO wallets), #1 chain-wide NEO mover (neoanalytics); 4.8k GAS | high (unnamed venue) | **flag** |
| 11 | NXF8xBeB…X9mcVH | 10,385,342 | 0 | **Exchange/MM desk** - 130k NEO, SWTH 7.5M, MCC, TIPS; heavy activity | high (unnamed venue) | **flag** |
| 12 | NQSbtd1Y…cP9JUC | 8,000,000 | 0 | **Person - OTC buyer** (rev. 2026-10-14) - paid NUN32i ~25.6k NEO in Nov 2025, received 5M FLM days later; 3M more from Nft4JE (same ring) Jul 2026; wallet active since Jul 2022 | high (flow-timed) | **revised: keep/review** |
| 13 | NSuUJyZr…Tu4KwC | 7,295,673 | 0 | **DEX infra** - the FLOCKS contract itself | certain | auto-excluded |
| 14 | NLXTWhgi…QjwFjH | 7,112,355 | 0 | **DEX infra** - FlamingoSwapPair | certain | auto-excluded |
| 15 | NUN32id3…we4ZZ | 6,989,117 | 5,030,926 | **OTC seller / trader** (rev. 2026-10-14) - accumulated FLM via desk NXF8xB (124 legs) and Broker lock cycles; Oct–Nov 2025 sold FLM for NEO to four long-standing wallets (see below); forwarded ~all NEO within minutes to NPmdLG, a 33-sender collection address also used by the attacker wallet | high (flow-timed) | **revised: keep/review** |
| 16 | Nfps2rFw…Y4S4a | 6,324,108 | 0 | **Person / real DeFi whale** - holds fUSDT 4.9M, fWETH 52, pONT, FTW; FLM built from two wallet transfers (14.35M, 6.49M) and LP activity | medium-high | **keep** |
| 17 | NRwXs5yZ…pV7Mb | 5,991,168 | 0 | **DEX infra** - FlamingoSwapPair | certain | auto-excluded |
| 18 | NRbVXs55…Rnzfv | 5,603,193 | 0 | **DEX infra** - Flamingo Swap-Pair | certain | auto-excluded |
| 19 | NaHC6xx4…xyTztD | 5,178,853 | 0 | **DEX infra** - FlamingoSwapPair | certain | auto-excluded |
| 20 | NanYZRm6…rZqxdZ | 5,128,677 | 0 | **DEX infra** - FLUND staking | certain | auto-excluded |

Just below the cut (for completeness): **NiF2ukjN…ifwA** - 5,000,000 FLM exactly from
#15 NUN32i (2025-11-13) after sending NUN32i ~22.5k NEO in the days before; wallet
active since Jul 2022 → **OTC buyer like #12, keep/review** (rev. 2026-10-14).
**NRfDCVn3…gfGR** - 4,836,421 FLM from a single 9.24M
transfer (2024-08-12, sender NVGFUq), only 11 lifetime transfers, no other assets →
quiet accumulator, person-or-allocation unclear, **review**.

## Correction 2026-10-14: the "#15 insider cluster" was an OTC ring

The first read saw round 5M FLM payouts from NUN32i to "fresh wallets" and called it
insider distribution. Tracing the NEO legs reverses it:

- The four receiving wallets (NQSbtd, NiF2ukj, Nft4JE, NT6Zw2) are **not fresh** -
  all have histories back to July 2022.
- In Oct–Nov 2025 they each sent NUN32i thousands of NEO (NQSbtd ~25.6k, NiF2ukj
  ~22.5k+ later legs, Nft4JE ~15.3k, NT6Zw2 ~89.9k), and the round FLM amounts
  (5M / 5M / 3M / 4M) arrived **days after the NEO**. Payment first, goods second =
  **OTC purchases**, not allocations.
- NUN32i forwarded nearly every NEO within ~1–3 minutes to **NPmdLG…4Aos**, a fan-in
  collection address with 33 distinct NEO senders (exchange-deposit pattern; notably
  the pre-exploit attacker wallet NWuHQdx also sent NEO there 28 times).
- NUN32i's own FLM came from desk NXF8xB (124 legs) and FlamingoBroker lock/unlock
  cycles - an accumulator-turned-seller, not a treasury.

Consequence: #12, #15 and NiF2ukj move from **flag** to **keep/review** (buyers paid
real NEO; the seller retained a normal stake). Same attestation offer as #2 applies:
if any of these owners come forward, reclassify on their word plus this evidence.

## What changed vs. the earlier heuristic pass

- `FLM_Top20_Flagged.md` suspected #2, #5, #8, #12, #15 were "team/treasury pattern."
  The acquisition trace first pointed at #15 as a distribution hub (round 5M payouts
  in Nov 2025), but tracing the NEO legs showed the opposite: those were **OTC
  sales** - the recipients paid NEO first, received FLM days later (see the
  correction section above). #12, #15, NiF2ukj reclassified accordingly.
- #2 (NTRGiig) showed the strongest "treasury" pattern on-chain - funded by a
  dedicated wallet (NUqLhf) with two ~5–14M FLM transfers, a dual FLM+FLOCKS
  position with ~zero NEO/GAS - but the owner **confirmed it is a whale, a real
  person** (2026-10-10). Reclassified to keep. This is the clearest case for why the
  flag list stays challengeable: behavior alone would have misflagged the largest
  community holder.
- The three desks (#6, #10, #11) are confirmed chain-scale liquidity hubs via public
  transfer leaderboards, but **no public label names the venue**. Treat as
  "exchange-grade desk, unnamed." If any of them is an exchange cold/hot wallet,
  FTB sent there lands in a pooled account, not with end users.

## Decided 2026-10-14: top-12 flag, NTRGiig at snapshot, dev wallet separate

Owner decision: **flag the top 12 wallets** (by snapshot FLM), **except #2 NTRGiig**,
a confirmed whale, which is valued **at the snapshot block** like everyone else.
NTRGiig's post-exploit accumulation (+28.7M FLM past its snapshot balance, and any
further buys) is **not** credited in the claim base - that amount, plus additional
funds, will be set aside as a **dev-team wallet for the FTB launch** (a separate,
explicit allocation outside the snapshot math).

- Top-12 wallets at snapshot: 197,881,386.47 FLM + 26,386,514.21 FLOCKS.
- Flagged (top 12 minus NTRGiig, minus #15 → dev wallet, per owner 2026-10-15;
  extended 2026-10-16 with the desk sub-storage wallets found in the #11–#20 sweep,
  then **narrowed same-day to the 6 inert ones** after the CEX-withdrawal test -
  the 7 active desk-funded wallets were judged legitimate by the owner and stay
  claimable): **109,999,032.43 FLM + 1,000,000.00 FLOCKS** removed. The 16 flagged:
  NiU7ic, NTWC7H, NWu2ng, NLdYPX, NXF8xB, NQSbtd, Nfps2r, NiF2ukj, NRfDCVn,
  NjEFZv4p3ascRwBcKcoTjbSAJtegiAupFm, plus inert desk-funded NNuxvz, Ndwkrm,
  NNkbhJ, NazUQh, NLPAk2, Ndx6j9.
  (Note: this flags #9 Nfps2r, which earlier analysis read as a real LP whale -
  owner call.)
- #15 NUN32i (6,989,116.71 FLM + 5,030,925.75 FLOCKS): **transferred to the dev-team
  wallet** (owner call, 2026-10-15) - not claimable, not burned.
- NTRGiig claim: **85,103,198.42 FLM + 20,355,588.46 FLOCKS** (snapshot rule, no
  special treatment). Its post-exploit accumulation goes to the dev wallet instead.
- Dev-team wallet funding source (owner note, 2026-10-14): the post-exploit FLM
  accumulation (+28.7M and counting) was bought with proceeds from **selling NUDES
  and other NEO-ecosystem assets tied to Flamingo** - i.e. the team converting its
  own ecosystem holdings into FLM at post-exploit prices to fund the relaunch.
  Accumulation is ongoing; the **dev wallet address will be confirmed and disclosed
  before launch**.

**Resulting migration claim base** (community list, contracts + attacker + flags removed,
everyone at snapshot):

| Token | Claim base | vs. pre-exploit supply |
|-------|-----------:|-----------------------:|
| FLM | **186,427,071.14** | 32.4% of 574,708,785.33 |
| FLOCKS | **113,932,226.80** | 93.1% of 122,346,444.06 |

Combined FLM + FLOCKS claim base: **300,359,297.94** (of 697,055,229.39 pre-exploit
combined). Set the FTB ratios against these two columns; the dev-team wallet is
allocated on top of FTB, not carved out of these.

**Final FTB-side supply (claim base + dev wallet, committed 2026-10-16):**

| Token | Claim base | Dev wallet (current) | FTB total | vs. pre-exploit supply |
|-------|-----------:|-----------:|----------:|-----------------------:|
| FLM | 186,427,071.14 | 68,330,973.24 | **254,758,044.38** | 44.3% of 574,708,785.33 |
| FLOCKS | 113,932,226.80 | 5,030,925.75 | **118,963,152.55** | 97.2% of 122,346,444.06 |

Combined FTB-side: **373,721,196.93** (53.6% of pre-exploit combined) - **COMMITTED by
the owner, 2026-10-16** (supersedes the same-day 368,005,397.74; the change is the
unflagging of the 7 active desk-funded wallets after the CEX-withdrawal test). This
implies r1 = r2 = 1 (one FLM or one
FLOCKS = one FTB) with the dev-wallet allocation minted inside the cap at deploy.
Current as of block 14,080,719; the dev-wallet FLM leg is still accumulating - re-read
it at launch and adjust the dev allocation within this committed total, or re-vote if
the total itself should move. The rest stays
behind: 38 contracts (270,708,623.39 FLM + 2,383,291.51 FLOCKS), the 16 flagged
wallets (109,999,032.43 FLM + 1,000,000.00 FLOCKS), and the attacker's pre-exploit
balance (584,941.65 FLM). Note the dev wallet's post-exploit FLM is not part of the
snapshot pie - it is claim-equivalent value donated by holder #2 from his own asset sales.

**RESOLVED (2026-10-15): the #15 OTC ring → dev wallet.** Owner decision: **all of
#15's FLM + FLOCKS (6,989,116.71 FLM + 5,030,925.75 FLOCKS) is transferred to the new
dev-team wallet**, together with **all FLM #2 NTRGiig acquired post-exploit
(+28,725,491.17 FLM and counting)** - earmarked for the dev team and potentially
initial liquidity. #15 therefore leaves the flagged set not via unflagging but via
**transfer**: its position never enters the claim base. #12 NQSbtd and NiF2ukj (the
OTC buyers) **stay flagged** - the owner's transfer call covered #15 only.

| Dev-wallet component | FLM | FLOCKS |
|----------------------|----:|-------:|
| #2 NTRGiig post-exploit accumulation (outside snapshot; as of block 14,080,719, 2026-10-15 - **still growing**) | 61,341,856.53 | 0 |
| #15 NUN32i full snapshot position | 6,989,116.71 | 5,030,925.75 |
| **Dev-team wallet total (current)** | **68,330,973.24** | **5,030,925.75** |

Combined dev wallet: **73,361,898.99** (and counting - accumulation ongoing).

**Custody rule for the #15 position (owner clarification, 2026-10-16):** the #15
position is **held in custody by the dev wallet - not owned by the team**. It is
claimable: anyone who proves ownership of #15 during the challenge window receives
the position and the dev wallet releases it. **If it is never claimed, the amount is
moved to the Governance contract proposal wallet** - community-controlled, not
team-controlled. Consequence for the supply math: the committed total
(373,721,196.93) is unchanged either way - custody keeps the position inside the
supply; only a final "really flagged" verdict would remove it (which would lower the
total by 12,020,042.46 to 361,701,154.47 - not the current decision).

#2 post-exploit trail, fully on-chain: 2026-10-04 sold 230T NUDES via FlamingoBroker
(NSy3Gffa) → 8,103,857.45 FLM; 2026-10-04/05 sold its **entire** 20,355,588.46 FLOCKS
(two Broker legs, 54,267.46 dust returned) → 20,621,633.72 FLM; 2026-10-15 a further
32,616,365.36 FLM settled from the Broker (block 14,080,719). Total 61,341,856.53 FLM;
current #2 balance 146,445,054.95 FLM, FLOCKS 0. Address to be confirmed and disclosed
before launch. Full evidence trail for #15: [FLM_15_NUN32i_DeepDive.md](FLM_15_NUN32i_DeepDive.md).

**#11–#20 sweep (2026-10-15)** - full histories pulled for the three non-contract
wallets in that range (raw: `snap_state/deep_11_20_raw.json`, `deep_11_full.json`,
`deep_11_counterparties.json`):

- **#11 NXF8xB (flagged desk):** 44,404 legs, 6,028 counterparties (2022-01 → today).
  Confirmed ties into the flagged network: NUWq8bh6 (#8's funder) 135 legs / 52.2M FLM;
  #15 NUN32i 124 legs / 8.39M FLM; NjEFZv 9 legs; ring NT6Zw2 1 leg. Also 39 direct
  legs / 904.7k FLM with #2 NTRGiig spanning 2022–2026 - small vs. #2's 85M position,
  consistent with order-book-era trading; the owner's no-relation attestation stands.
- **Desk sub-storage network found in the top-100.** 14 wallets received their *entire
  exact snapshot balance* directly from the desk with zero return flow and zero NEO
  paid - the NjEFZv signature at scale: #23 NjEFZv (4.27M, already flagged), #38 NNuxvz
  (1.32M), #42 NSCvRc (1.20M), #44 NLFsAv (1.11M), #53 NR9vB6 (1.00M), #55 Ndwkrm
  (0.91M), #58 NTTsRc (0.82M), #62 NQ6h93 (0.72M), #72 NNkbhJ (0.54M), #78 NazUQh
  (0.51M), #79 NNa7NT (0.50M), #81 NLPAk2 (0.49M), #87 Ndx6j9 (0.44M), #96 NN9VpH
  (0.36M). Total **14,192,626.20 FLM**, of which **9,925,760.29 FLM is newly
  identified** beyond the current flagged set. Seven more top-100 wallets show the
  partial pass-through pattern (#30 NbSMUD 11.5M through, #68 NMLtz balance exactly
  599,999.90 - the NjEFZv chunk size, #37, #39, #80, #90, #93; ~7.04M FLM) → review.
- **#12 NQSbtd (flagged, ring buyer):** counterparty set is almost entirely the ring -
  Nft4JE (12 legs), #15 (7), NiF2ukj (3), NT6Zw2 (2), NYQVQar (1) - plus three new
  ties: **Na7Th1b (10 legs)** - the unresolved early forwarder from the #15 deep dive
  is ring-connected after all; **NUqLhf settlement wallet (6 legs)**; and **NgHY57
  (1 leg)** - one of the fresh Feb-2026 wallets #15 seeded.
- **#16 Nfps2r (flagged by owner call):** zero connections to any flagged wallet, the
  desk, the collector, or the ring. Its funders (NNikg6 14.5M/84 legs, NUeY5i 14.35M,
  NbrACQj 6.49M) are also clean of the desk. On-chain evidence supports the original
  "real LP whale" reading - **flag rests on the owner call alone; candidate for
  re-review**.
- **#13, #14, #17, #18, #19, #20:** contract slots (FLOCKS contract, Flamingo swap
  pairs, FLUND staking) - classifications stand as verified in the snapshot pass;
  skipped deep pulls by design (protocol contracts touch everyone).

Net effect - **DECIDED (2026-10-16): 6 of the 13 exact-funded wallets are flagged;
the 7 active ones were unflagged by the owner the same day** after the
CEX-withdrawal test below ("Those look legitimate - let's include those"). Net claim
base change vs. the original 10-wallet set: −4,209,961.10 FLM. The seven
partial pass-through wallets (#30, #37, #39, #68, #80, #90, #93, ~7.04M FLM) remain
on the **review** list - not flagged.

**CEX-withdrawal caveat (tested 2026-10-16, owner asked directly):** the sub-storage
reading of the 13 is **not provable** - an unnamed-venue hot wallet paying customer
withdrawals produces the same one-way, no-on-chain-payment signature (payment happens
off-chain on the venue ledger). Desk-scale facts support the venue reading: 1,057
one-way FLM receivers (median ~5.9k), 1,684 depositors, 6,028 counterparties, no
label found anywhere. Split of the 13 by wallet behavior:

- **Inert (strong sub-storage signature, 6 wallets, 4,209,961.10 FLM):** NNuxvz,
  Ndwkrm, NNkbhJ, NazUQh, NLPAk2, Ndx6j9 - zero sends ever, single counterparty,
  funded within a day, dormant since (NjEFZv's identical-chunk pattern is the
  strongest of all). Flag stands on solid behavioral ground.
- **Active real-user wallets (weak signature, 7 wallets, 5,715,799.19 FLM):** NSCvRc,
  NLFsAv, NR9vB6, NTTsRc, NQ6h93, NNa7NT, NN9VpH - multi-year histories (two since
  2021/2022), dozens of counterparties in other tokens; their FLM came only from the
  desk and sat untouched. This is *exactly* what a venue customer who withdrew and
  held looks like. **If the desk is a CEX, these seven are ordinary users.**
  Evidence: `snap_state/exact_funded_check.json`. **RESOLVED (2026-10-16): owner
  judged the seven legitimate - unflagged and claimable** (+5,715,799.19 FLM back
  to the claim base; committed FTB total 373,721,196.93).

**Payment-first trace on the remaining flagged wallets (2026-10-15)** - checking whether
any of them are hidden ring buyers:

- **#5 NiU7ic (20.08M):** FLM came from wallets NVUvokc / Nb1zQp; it sent 15,000 NEO
  a month later to two *different* wallets (NagPMQ8, NaJdmhh) that never moved it on.
  Payment does not reach the seller or the collector → **not a confirmed purchase;
  allocation-pattern stands. Stay flagged** unless the owner attests.
- **#8 NWu2ng (15.5M):** its entire position arrived as 4 chunks (1M+5M+2M+3.5M+4M,
  May–Jun 2026) from **NUWq8bh6**, a desk-connected trading wallet that receives NEO
  from desks NXF8xB / NTWC7H and cashed 4,993 NEO to the same NPmdLG collector as the
  #15 ring. No payment leg from NWu2ng → **trading-network vault, not a buyer. Stay
  flagged**, now with a documented link to the desk/collector network.
- **#10 NRfDCVn (4.84M):** still a single 9.24M inbound (NVGFUq), no payment legs,
  11 lifetime transfers. **Unresolved - stays flagged-as-review.**
- **#11 NjEFZv (4.27M):** funded July 2026 in seven identical 599,999.90 FLM chunks
  directly by flagged desk **NXF8xB**, zero sends ever → behaves like **desk
  sub-storage. Stay flagged, confidence raised.**

Net effect: no changes to the flagged set from this trace. (The #15 ring question was
later resolved by the owner's dev-wallet transfer decision - see the resolved section
above.)

**Second-level funding trace (2026-10-15)** - who funded the funders:

- **#5 NiU7ic's funders trace to two roots.** NVUvokc was funded 18.19M FLM by
  **NUqLhf - the same wallet that funded #2 NTRGiig (the confirmed whale)** - and
  11.1M by NV8D4oeD; Nb1zQp was funded 4.7M by NUqLhf. The NEO legs stay inside the
  cluster (Nb1zQp even sent 41,889 NEO *to* NiU7ic). So #5 belongs to the **same
  wallet family as #2**, sharing its funding wallet. If NUqLhf is the whale's own,
  #5 is the whale's satellite and should follow #2's keep decision.
- **#8 NWu2ng's funder NUWq8bh6 was itself fed 7.55M FLM by NV8D4oeD** - the same
  distributor that fed #5's cluster. NV8D4oeD received **73,000,000 FLM in 8 legs
  from NiQGM8Qw (a wallet, not a contract)** and sprayed it to ~dozens of wallets
  (992 sends). So #5 and #8 share one root: a 73M-FLM master distribution wallet.
- **#10 NRfDCVn's root NSPyuNud bought ~15.1M FLM on-market** from FlamingoSwapPair
  pools in Aug 2024 - funded by 94,443 NEO from **BurgerAgent0** (a Neo BurgerDAO
  contract, i.e. ecosystem-fund money, not a personal wallet). It passed 9.24M
  same-day through NVGFUq to NRfDCVn. So #10's stake was market-bought, but with
  ecosystem-fund capital.
- Caveat: NUqLhf's own history exceeds the 1,000-record RPC window; its ultimate
  FLM source is unresolved from public calls alone.

**Relay-network check on #5 (2026-10-15, owner hypothesis: "#5 moves funds from the
main wallet to the trading wallet"):** partially confirmed, with a twist -

- #5's 20.08M FLM **never moved** after arriving (parked, still held at snapshot).
- Its NEO did flow: 41,889 NEO in from Nb1zQp (2026-05-21), then 10,000 → NagPMQ8
  (05-30) and 5,000 → NaJdmhh (06-26).
- Both destinations are **relay wallets**: NagPMQ8 has ~1,000 legs since Nov 2023,
  each forwarding funds to **NUqLhf within ~12 minutes**; NaJdmhh (256 legs since
  Feb 2025) forwards everything to a second hub, **NdtNKmNH…tkWk**, within ~4
  minutes. The relays are fed by dozens of wallets including desks #6/#11 and the
  NVUvokc funder.
- So the verified structure is: funders (NVUvokc, Nb1zQp) → #5 → relay wallets →
  collection hubs (NUqLhf, NdtNKmNH). #5 is a **parking wallet with a pass-through
  NEO channel into the collection network**, not a pure conduit: the FLM stayed.
- **Owner correction (2026-10-15): #5 is NOT related to #2, and #2 has no relations
  with any other wallet in the top 100.** Verified against the chain: of #2's 23
  distinct lifetime counterparties, only six are in the snapshot top 100 - five are
  Flamingo protocol contracts (unavoidable) and one is desk #11 NXF8xB (trading
  counterparty). NUqLhf, which paid both #2 and #5's funders, is therefore a
  **third-party payer** (trading/OTC settlement wallet), not the whale's own wallet:
  the two families share a counterparty, not an owner. #5 remains classified on its
  own evidence (funded from the 73M-distributor network) and **stays flagged**.
  (That correction left the claim base unchanged; the claim base was later reduced
  by the 2026-10-16 desk sub-storage flag extension - see the claim-base table above.)

**The NUDES-sale chain and where #5 enters it (owner account, traced 2026-10-15):**

- **Nov 2025 sale:** #2 moved 0.1T (test) + **229.9T NUDES**, 30 GAS and
  **10,782,083.54 FLOCKS** to trading wallet **NQzqB9AB** (2025-11-04 20:03–20:07;
  NQzqB9's first-ever legs were 100k FLM test transfers with #2 back in Jan 2022).
  NQzqB9 then sold gradually on the order book: FLM proceeds arrived from the
  FLOCKS contract (NSuUJy) in chunks (36.1k on 11-10, 80.9k on 11-17, 60.9k on
  11-24…) and were **forwarded back to #2 within ~1 minute each time**.
- **Oct 2026 sale (post-snapshot):** #2 sent exactly **230.00T NUDES** to the
  FlamingoBroker on 2026-10-04 21:05 and received **8,103,857.45 + 20,621,633.72 =
  28,725,491.17 FLM** from the Broker in the same minutes - this is the +28.7M
  post-exploit accumulation earmarked for the dev-team wallet.
- **#5's position in the chain:** #5 is *not* part of either sale's settlement path,
  and per the owner correction above it is **not related to #2**. On-chain it was
  funded (Apr–May 2026) by NVUvokc / Nb1zQp, which were paid by NUqLhf - the same
  third-party settlement wallet that paid #2 its 19.5M FLM in Apr 2025. A shared
  payer proves a trading relationship with the same counterparty, not common
  ownership; with the owner's attestation, #5 stands on its own evidence and stays
  flagged.

Evidence: `snap_state/flag_top12.json`.

## The decision this supports (final, 2026-10-16)

- **Auto-excluded (no decision needed):** the 38 contracts - 270,708,623.39 FLM + 2,383,291.51 FLOCKS.
- **Flagged (excluded from FTB claim), pending community challenge:** the 16 wallets
  #5 NiU7ic, #6 NTWC7H, #8 NWu2ng, #10 NLdYPX, #11 NXF8xB, #12 NQSbtd, #16 Nfps2r,
  NiF2ukj, NRfDCVn, NjEFZv, plus the 6 inert desk sub-storage wallets NNuxvz,
  Ndwkrm, NNkbhJ, NazUQh, NLPAk2, Ndx6j9
  = **109,999,032.43 FLM (19.1% of supply) + 1,000,000.00 FLOCKS**.
- **Dev-team wallet (owner decision, 2026-10-15):** #15 NUN32i's full position
  (6,989,116.71 FLM + 5,030,925.75 FLOCKS) + #2 NTRGiig's post-exploit accumulation
  (61,341,856.53 FLM as of block 14,080,719, still growing) = **68,330,973.24 FLM +
  5,030,925.75 FLOCKS**, earmarked for the dev team and potentially initial liquidity.
  Address disclosed before launch.
- **Keep:** #2 NTRGiig (confirmed whale, owner-attested; snapshot balance claim only).
- **Review:** NRfDCVn (4.84M, quiet; no desk or cluster signature), and #8 if the
  community wants to be conservative (its FLM was partly bought on-market, but the
  curated round balances argue allocation).

All flag proposals should be published with this evidence and left open to challenge
before the migrator's exclusion list is finalized. No address here is *provably* a
named exchange or named team wallet from chain data alone; the classification rests
on behavior patterns anyone can re-check with the commands in this folder.

## Appendix A - full addresses for the flagged 16 (added 2026-10-21)

The shorthand used above truncates 7 of the 16 addresses. Full base58 forms,
re-verified against `snapshot_full_state.csv` and confirmed absent from the v2
claim tree (#8 excepted - restored, as recorded above):

| Shorthand | Full address | Snapshot FLM | Snapshot FLOCKS |
|---|---|---:|---:|
| #5 NiU7ic | NiU7ic68RHF4oo5QK8Li5JbXx22rs2V3DS | 20,083,298.57 | 0 |
| #6 NTWC7H | NTWC7Hh5VYMQ5K8YJbyCLbmJ4RhfQ1Ej64 | 18,431,767.02 | 0 |
| #8 NWu2ng (restored) | NWu2ng1hBxU6w6AUmGy4QDkscQTXY3pnVM | 15,500,000.00 | 1,000,000.00 |
| #10 NLdYPX | NLdYPXACYPKa6zTvoThPuJxd3uEmeMg93t | 12,961,268.28 | 0 |
| #11 NXF8xB | NXF8xBeBaXSrrvTAHWK6xuGTyutAX9mcVH | 10,385,342.33 | 0 |
| #12 NQSbtd | NQSbtd1YYCe1d6gHubwt275QzNTbcP9JUC | 8,000,000.00 | 0 |
| #16 Nfps2r | Nfps2rFwD9VZ8kd6GRkgWix8LH8A5Y4S4a | 6,324,107.88 | 0 |
| NiF2ukj | NiF2ukjNVxrLbP7XScPBfKLxyyh8s8ifwA | 5,000,000.00 | 0 |
| NRfDCVn | NRfDCVn3zFJ9XkXnYYHSTJaqSXNxvGgfGR | 4,836,421.34 | 0 |
| NjEFZv | NjEFZv4p3ascRwBcKcoTjbSAJtegiAupFm | 4,266,865.91 | 0 |
| NNuxvz | NNuxvzNUHGFfPJoAWdAYRCpe3vYYBo6t6A | 1,323,418.02 | 0 |
| Ndwkrm | NdwkrmxQy3tZu5VDDELPhc6gRqYMf4Akgh | 909,120.84 | 0 |
| NNkbhJ | NNkbhJDHsQJRUc7ff8gtgFxybEFgndRQLT | 542,009.80 | 0 |
| NazUQh | NazUQhvnw3KF71KGa5MgXdfZknM116ze5F | 508,381.05 | 0 |
| NLPAk2 | NLPAk27emKrnCSJN3vvYskfryXBxqmTnJ4 | 489,555.73 | 0 |
| Ndx6j9 | Ndx6j9ZpoUnTqDy6Pq2ERnQNd6HRceddyS | 437,475.66 | 0 |
| **Total** | | **109,999,032.43** | **1,000,000.00** |

Sum matches the flagged total quoted above to the cent.

*On-chain measurement, not legal or financial advice. Figures at block 12,782,704;
reproduce via `snapshot_state.py` and the probes in `snap_state/`.*
