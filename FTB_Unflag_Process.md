# FTB Unflag Request — Ownership Proof Process (DRAFT)

Date: 2026-10-17 · Status: draft for owner review · Applies to: flagged wallets whose
owners dispute the flag (e.g. wallet #8 `NWu2ng1hBxU6w6AUmGy4QDkscQTXY3pnVM`).

## The idea

A flag means "we believe this wallet belongs to the team / a desk / the whale
cluster, not an ordinary holder." To overturn it, a claimant must prove two
things: **they control the private key** and **a human stands behind it**.

Neo N3 MainNet and TestNet derive the *same address* from the *same key*. So
whoever controls a flagged MainNet wallet also controls that exact address on
TestNet — where GAS is free. The proof costs the claimant nothing real, and it
cannot be faked by anyone who does not hold the key.

## The message claimants send us

Posted to the claim channel (Discord / claim form), containing exactly three
things:

```
I request the unflagging of my wallet for the FTB migration.

Wallet address:  N<flagged wallet address>
TestNet proof:   <txid of a TestNet GAS transfer from this same address
                  to the FTB team TestNet wallet>
Contact:         <how we reach you for follow-up>
```

## The on-chain proof (step by step for the claimant)

1. Open your wallet (NeoLine, NEON, O3 — any wallet that holds the flagged
   wallet's key) and switch it to **Neo N3 TestNet**. Your address string is
   identical there.
2. Get free TestNet GAS from the public faucet to your address.
3. Send **any small amount of TestNet GAS** (e.g. 0.1 GAS) **from your flagged
   address** to the FTB team TestNet wallet:

   **`NeNEyksWAkbJdEXqVta2X1N3rx1hmzwReP`**

4. Copy the transaction id and paste it into the message above.

## How we verify

- Pull `getnep17transfers` for the team TestNet wallet and find the txid.
- Confirm the **sender address is character-identical** to the flagged MainNet
  wallet being claimed. Same address on TestNet can only be sent from by the
  holder of the same private key — that is the whole proof.
- Confirm the claim message and the tx match (address + txid pair, one per claim).
- Unflagging is still a human decision after the technical proof passes —
  the proof establishes key control, not entitlement; the flag rationale is
  re-reviewed against the claim (as done for #8).

## What we will NEVER ask for

- Your private key, seed phrase, keystore file, or wallet password.
- Any MainNet funds. The proof transfer happens on TestNet only, with free
  faucet GAS. Anyone asking for keys or MainNet funds "to unflag you" is a
  scammer.

## Operational notes (internal)

- Team TestNet wallet: `NeNEyksWAkbJdEXqVta2X1N3rx1hmzwReP` (the deploy wallet —
  receive-only for this purpose; no spending from it is involved).
- One txid per claim; a txid already used for a claim cannot back a second one.
- Keep a public log of reviewed claims (wallet, verdict, reason) — the flag list
  is a published, challengeable decision, so reviews must be published too.
- SLA to state publicly: acknowledge within 7 days, verdict within 30 days.

## Claims log (public)

| # | Wallet | Proof txid | Verdict | Reason |
|---|--------|-----------|---------|--------|
| 1 | `NWu2ng1hBxU6w6AUmGy4QDkscQTXY3pnVM` (flag #8, 15.5M FLM + 1M FLOCKS) | `0xcb423f7a878cefc711b0c3653cd8b2c07a33c5e08bcc81b22db74db025a2893c` | **UNFLAGGED 2026-10-17** | Proof verified on-chain: HALT, 1.0 TestNet GAS, signer + sender identical to the claimed address, recipient = team TestNet wallet. Independent on-chain review (4-yr DeFi history, desk-funded accumulation, zero cluster overlap) also supported removal. |
