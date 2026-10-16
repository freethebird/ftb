# FTB Migration — Pre-Exploit Snapshot Findings

*Community disclosure · published 2026-10-16 · everything below is verifiable on-chain
with the tools linked at the end. Nothing here is final until the challenge window closes.*

---

## TL;DR

We rebuilt the complete FLM + FLOCKS holder state one block before the exploit mint,
classified every large wallet, and set the FTB migration math:

- **Claim base:** 186,427,071.14 FLM + 113,932,226.80 FLOCKS across **4,830 wallets**
- **Dev-team wallet:** 68,330,973.24 FLM-equivalent + 5,030,925.75 FLOCKS (disclosed below)
- **FTB total supply (committed): 373,721,196.93** at 1 FLM = 1 FTB, 1 FLOCKS = 1 FTB
- **Excluded from claims:** 38 protocol contracts, the attacker's wallet, and 16 flagged
  wallets (110.0M FLM + 1.0M FLOCKS) — each with published evidence, open to challenge

## 1. The snapshot

The exploit minted 37.7 trillion FLM, so today's balances are meaningless for a fair
migration. Instead we took the state at **block 12,782,704** — one block before the
exploit transaction — directly from authenticated chain state (state root
`0x86a3042d…baf7`), not from any explorer API.

The reconstruction sums **exactly** to the on-chain total-supply anchors
(FLM 574,708,785.33 / FLOCKS 122,346,444.06), which means the list is complete by
construction: every token that existed at that block is accounted for. Anyone can
reproduce it with `snapshot_state.py`.

## 2. Who is excluded from the claim

### 2a. Protocol contracts (38) — automatic

Flamingo pair contracts, the FLOCKS staking contract, FLUND, vaults and routers:
270,708,623.39 FLM + 2,383,291.51 FLOCKS. Tokens inside protocol contracts are not
owned by any claimer; they are excluded automatically.

### 2b. The attacker wallet

The pre-exploit attacker address held a real 584,941.65 FLM before the exploit. It is
excluded anyway.

### 2c. Flagged wallets (23) — evidence published, open to challenge

| Wallet | FLM at snapshot | Why flagged |
|--------|----------------:|-------------|
| NiU7ic (#5) | 20,083,298.57 | Parking wallet; funded from a 73M-FLM distributor network, NEO pass-through to collection hubs |
| NTWC7H (#6) | 18,431,767.02 | Exchange/MM desk behavior |
| NWu2ng (#8) | 15,500,000.00 | Trading-network vault; entire position arrived as 4 round chunks from a desk-connected wallet, no payment legs |
| NLdYPX (#10) | 12,961,268.28 | Exchange/MM desk; ~416k NEO, one of the largest NEO movers chain-wide |
| NXF8xB (#11) | 10,385,342.33 | The central desk: 44,404 transfers, 6,028 counterparties; funded #15, NjEFZv and the 13 sub-storage wallets below |
| NQSbtd (#12) | 8,000,000.00 | Member of the #15 OTC ring; counterparties are almost entirely ring wallets |
| Nfps2r (#16) | 6,324,107.88 | Owner call — on-chain evidence is actually clean (possible LP whale); **strongest challenge candidate** |
| NiF2ukj | 5,000,000.00 | #15 ring buyer (paid NEO, received 5M FLM) |
| NRfDCVn | 4,836,421.34 | Quiet accumulator; entire position from one 9.24M transfer, 11 lifetime transfers, no payment trail |
| NjEFZv | 4,266,865.91 | Desk sub-storage: funded by NXF8xB in seven identical 599,999.90 chunks, zero sends ever |
| NNuxvz, Ndwkrm, NNkbhJ, NazUQh, NLPAk2, Ndx6j9 (6 wallets) | 4,209,961.10 combined | Desk sub-storage: each received its **entire exact snapshot balance** directly from desk NXF8xB, then sat completely inert — zero sends ever, single counterparty, dormant since funding |

Total flagged: **109,999,032.43 FLM + 1,000,000.00 FLOCKS** (19.1% of FLM supply).

### 2d. Review list (7 wallets, not flagged)

Seven more top-100 wallets received desk money but also moved it (#30 NbSMUD,
#37 NeQzBT, #39 NQuH4t, #68 NMLtz, #80 NesaYc, #90 NUupYC, #93 NhSCNK — ~7.04M FLM
combined). The signature is weaker, so they **remain claimable** unless the community
decides otherwise.

**Disclosed caveat on the desk-funded flags:** a venue hot wallet paying customer
withdrawals produces a similar one-way signature, and the desk is an *unnamed* venue —
so these flags are behavior-pattern judgments, not proof. Thirteen wallets originally
matched the "entire balance from the desk" pattern; we then pulled each wallet's full
history. Six were completely inert (zero sends ever, funded in a day) and stay
flagged. Seven (NSCvRc, NLFsAv, NR9vB6, NTTsRc, NQ6h93, NNa7NT, NN9VpH —
5,715,799.19 FLM) turned out to be long-active, multi-token wallets — ordinary users
by every measure — and **were unflagged and remain fully claimable**. This is the
challenge process working as intended.

## 3. The dev-team wallet — full disclosure

The dev-team allocation has exactly two sources, and it is **not** carved
out of anyone's claim:

- **The team's own post-exploit proceeds.** Wallet #2 (the confirmed whale driving the
  relaunch) sold **his entire FLOCKS position (20,355,588.46) and his NUDES holdings**
  into the post-exploit market, and **transferred the proceeds to the dev wallet**.
  Total so far: **61,341,856.53 FLM** (verified on-chain at
  block 14,080,719; still accumulating, final figure re-read at launch). This is the
  team converting its own ecosystem assets into launch funding at post-exploit prices.
- **#15 NUN32i's snapshot position, held in custody — not owned.** #15's
  6,989,116.71 FLM + 5,030,925.75 FLOCKS sits **under the custody of the dev wallet
  until someone claims it and proves ownership**. #15 was an OTC seller whose network
  ties (desk funding, ring hub behavior, shared cash-out collector) made an automatic
  community claim inappropriate — but custody is not confiscation: **if a legitimate
  owner proves control during the challenge window, the position is released to them.
  If it is never claimed, the amount moves to the Governance contract proposal
  wallet** — community-controlled, not team-controlled.

Dev wallet total: **68,330,973.24 FLM-equivalent + 5,030,925.75 FLOCKS** (of which
12,020,042.46 is the #15 custody position, not team property), earmarked
for the team and potentially initial liquidity. **The dev wallet address will be
published before launch.**

## 4. The whale that stays

Wallet #2 (NTRGiig, 85,103,198.42 FLM + 20,355,588.46 FLOCKS at snapshot) is a
confirmed real person, attested by the project owner. It claims at the snapshot block
like everyone else — no special treatment in either direction. Its post-exploit
accumulation is the dev-wallet funding described above, not a claim.

## 5. The committed numbers

| Token | Claim base | Dev wallet | FTB total | vs. pre-exploit supply |
|-------|-----------:|-----------:|----------:|-----------------------:|
| FLM | 186,427,071.14 | 68,330,973.24 | 254,758,044.38 | 44.3% |
| FLOCKS | 113,932,226.80 | 5,030,925.75 | 118,963,152.55 | 97.2% |

**FTB total supply: 373,721,196.93** (committed 2026-10-16), at r1 = r2 = 1.
The dev-wallet FLM leg is still accumulating; its final size is re-read at launch and
settled inside this total.

## 6. Verify and challenge

- Reproduce the snapshot: `snapshot_state.py` (walks balance storage at the snapshot
  state root; sums must match the supply anchors exactly or it fails).
- Claim list: `ftb_claim_list.csv` (4,830 entries; sums: 186,427,071.14378729 FLM,
  113,932,226.80281632 FLOCKS).
- Full classification evidence: `FLM_Top20_Ownership.md`, the #15 deep dive
  (`FLM_15_NUN32i_DeepDive.md`), and the raw RPC pulls in `snap_state/`.

**Every flag is challengeable.** If you own a flagged wallet, come forward with the
address and your side of the story before the migrator's exclusion list is finalized;
one wallet (#2) has already been reclassified this way. No address here is provably a
named exchange or team wallet from chain data alone — every classification is a
behavior pattern that anyone can re-check.

---

*On-chain measurement, not legal or financial advice. Figures at block 12,782,704
(snapshot) and block 14,080,719 (dev-wallet accumulation), Neo N3.*
