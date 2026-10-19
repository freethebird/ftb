# Community AMM on TestNet — the Decision 3 follow-through (19 October 2026)

Decision 3 (11 Oct) left the router a placeholder on TestNet "until pools exist
(MainNet, or a minimal community AMM stood up for TestNet later — a separate
decision)". This is that separate decision, executed: the swap module must be
testable before the IAR, and TestNet has no AMM (neo-project/neo#3285).

## What is deployed

The reused, audited Flamingo swap contracts — byte-source-identical to
`github.com/flamingo-finance/flamingo-contract-swap` (master, vendored here
with its .git removed), compiled with the project's pinned compiler
(`Neo3.Compiler.CSharp.Dev` 3.0.3, .NET 6 SDK). The ONLY source edits are
deployment constants, each visible in `git log -p` for this directory:

| File | Constant | From | To |
| --- | --- | --- | --- |
| all four `*.Admin.cs` | `superAdmin` | Flamingo's MainNet admin | the TestNet deploy wallet |
| `FlamingoSwapRouter…Admin.cs` | `Factory` | MainNet factory | `0x2b582cdc…` (below) |
| `FlamingoSwapPair…Admin.cs` | `WhiteListContract` | MainNet whitelist | `0x0009b28b…` (below) |
| `FlamingoSwapPair…Admin.cs` | `TokenA` / `TokenB` / `Symbol` | fWBTC/fUSDT | GAS / FTB / `FLP-GAS-FTB` |

Admin on TestNet is the deployer's single key — same FTB-14 caveat as the
core, acceptable on a valueless network; MainNet redeploys with the
published multisig and recomputed constants.

## Live contracts (TestNet)

| Contract | Hash | Deploy tx |
| --- | --- | --- |
| FlamingoSwapPairWhiteList | `0x0009b28b90c23b5fc2c49265572695defe2c9f57` | `0xaba2e314…` |
| FlamingoSwapFactory | `0x2b582cdc325e0788a6732a79671687580d985dc4` | `0x884e0791…` |
| FlamingoSwapPair (GAS/FTB) | `0x907598701d7b5c7090e4a15715784ad3c8163a53` | `0xa5232f37…` |
| FlamingoSwapRouter | `0x7e3fa9356e3294acf709c51062ca051f1df6cb5c` | `0x19f5b871…` |

Wiring: `whitelist.addRouter(router)` tx `0x1e56af9e…`,
`factory.registerExchangePair(pair)` tx `0xc3c540c8…`.
Seed: `router.addLiquidity(1.25 GAS, 50 FTB)` tx `0x31880ada…` — first pool,
ratio 1 GAS = 40 FTB (the Decision placeholder).

Dead first-deploy contracts (immutable, harmless, documented for the IAR):
pair `0xc1b961f6…` and router `0x3c8ca0d7…` were built with the whitelist /
factory constants in byte-reversed order (the hash tool printed BE; C#
`InitialValue` expects the canonical 0x form). They answer no calls; the
factory mapping and whitelist point only at the corrected contracts. The
`cmd/contracthash` tool now prints `StringLE` and its output was verified
against `getcontractstate` for all four live hashes.

## Verified before wiring the UI

- `router.getReserves(GAS, FTB)` → `[125000000, 5000000000]` (HALT) — the
  exact read the swap ticket, ratio board, ticker, and TVL tile all share.
- Simulated `swapTokenInForTokenOut(0.1 GAS → FTB)` → HALT, 3.69341335 FTB
  out (matches the UI's constant-product formula), both Transfer events plus
  the pair's Synced/Swapped, 0.0247 GAS system fee.
- `addLiquidity` / swap writes need a `CustomContracts` signer naming the
  router, both path tokens, and the pair — the same scope `ftb-dapi.js`
  documents for swap (CalledByEntry cannot cross the router boundary).

## Tooling added

- `cmd/ammdeploy` — phased deployer (deploy | wire | seed), resume-safe,
  same self-check discipline as cmd/deploy.
- `cmd/contracthash` — prints the canonical 0x hash a NEF will have when
  deployed by a given address (sender + checksum + name).
