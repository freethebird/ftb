# FTB Wallet Policy - injected desktop dAPI only; no WalletConnect, no dependencies

**As of:** 8 November 2026.
**Purpose:** why FTB connects a wallet only through an injected Neo N3 dAPI provider in a desktop browser (NeoLine the reference), and why it will not adopt WalletConnect or any runtime dependency to reach mobile. A deliberate, recorded decision, not an oversight.
**Reads with:** [`KEYLESS_ATTESTATION.md`](KEYLESS_ATTESTATION.md) (FTB holds no keys), [`DEX_SAFETY_SELF_ASSESSMENT.md`](DEX_SAFETY_SELF_ASSESSMENT.md) (S1-S10), and the front-end canon enforced by every page's self-only Content-Security-Policy and the deploy gate's `ftb-canon-lint`.

## The one-line policy

Connect a wallet to FTB through an injected Neo N3 dAPI provider in a desktop browser (the NeoLine extension, Ledger-backed for cold keys). Mobile is read-only. FTB ships no WalletConnect, no relay, no third-party SDK, and no runtime dependency of any kind.

## First, the reframe: the wallet is not FTB's safety floor

FTB's fund safety does not come from the wallet. The dApp holds no keys and signs nothing. It builds every write as a plain-language plan shown before signing (WYSIWYS), pins every recipient and contract hash in `config.js`, scopes writes to `CalledByEntry` (never `Global`), and the contract re-validates on-chain. The worst a misbehaving or mis-displaying wallet can do to an FTB user is cause a failed transaction, not a theft. FTB is self-custody only, and never integrates a wallet that holds, derives, splits, or reconstructs a user key.

Consequence: the wallet question is not a safety question. It is a question of dependency and reach. That is the lens for everything below. Choosing NeoLine-on-desktop buys no safety FTB does not already have, and refusing WalletConnect loses none either. What is at stake is whether FTB stays a self-contained, same-origin, un-ownable artifact.

## Why an injected dAPI provider (and why that means desktop)

A wallet can reach a web dApp in exactly two ways:

1. **Injected provider.** The wallet injects a JavaScript object into the page (on Neo N3: `window.NEOLineN3`, or the newer NEP-21 `Neo.DapiProvider`). The dApp calls that object directly. Nothing is fetched, no code is downloaded from a third party, no server sits in the path. This is the zero-dependency path, and it is the one FTB uses.
2. **WalletConnect.** The dApp loads a client SDK and routes every message to the wallet through a hosted relay. This is a dependency - outside code and infrastructure in the signing path - and is dissected below.

An injected provider only exists where the wallet does the injecting: a desktop browser extension, or a wallet's own in-app browser. A normal mobile browser (Safari, Chrome) has no wallet in it to inject, so there is nothing on the page to connect to. On 8 November 2026 a device probe confirmed that NeoLine mobile has no in-app dApp browser, so no injected provider is available on mobile at all. The injected-dAPI path is therefore a desktop path, and that is why mobile is read-only: not a limitation FTB chose arbitrarily, but the only place a zero-dependency wallet connection physically exists.

## Why NeoLine - and why it is a standard, not a lock

NeoLine is the de-facto Neo N3 dApp wallet: open-source (MIT), a browser extension with Ledger hardware support, actively maintained, and it injects the N3 dAPI. For a keyless dApp the Ledger path matters most - NeoLine driving a Ledger keeps the signing key off the hot machine and confirms the transaction on the device's own screen.

FTB is not hard-locked to one brand. The adapter (`ftb-dapi.js`) is written to the N3 dAPI STANDARD: its calls are `getAccount`, `invokeRead`, and `invoke` with `CalledByEntry`, which any compliant injected N3 wallet exposes. Today the detection is keyed to NeoLine's injected object; generalizing it to any injected desktop dAPI provider is a small, zero-dependency change that widens the supported-wallet list without loading a line of third-party code. That generalization is open and allowed, because it keeps FTB on the standard and off any single-vendor dependency. What is not on the table is reaching a wallet through a relay.

## Why not WalletConnect, or any dependency

Every candidate dependency has to argue for its life against FTB's front-end canon, and WalletConnect loses on nearly every count at once, because it is not a library - it is a hosted relay plus an account:

- **Signing-surface supply chain.** The client's one unique power is to ask a human to sign. Any third-party code on the page has the same DOM access FTB's own code does, and can silently rewrite the recipient, amount, or contract a user is about to approve. WYSIWYS is the only defense, and a single compromised dependency defeats it. The WalletConnect / Reown SDK is large, with a deep transitive dependency tree - exactly the attacker-controlled surface, on the one page where it is most dangerous. Vendoring and pinning shrink the blast radius; they do not remove it.
- **Liveness coupling.** FTB is built to keep working as a frozen IPFS/IPNS bundle with nothing fetched. WalletConnect routes through a hosted relay (`relay.walletconnect.com`) and requires a Project ID the relay validates server-side. FTB would then go dark whenever that relay is down, rate-limits it, or de-platforms it. A protocol an outside server can switch off is not ownerless.
- **Self-only CSP breach.** Every FTB page ships `default-src 'self'`, and the deploy gate's `ftb-canon-lint` makes same-origin an enforced rule, not a promise. WalletConnect requires opening that CSP to the relay and the Reown origins - a permanent hole that every future XSS and vendor compromise can use.
- **Reproducibility loss.** The published bundle is a byte-deterministic function of the source, and its IPFS CID is the reproducibility proof anyone can check. A runtime dependency breaks that: the artifact stops being verifiable from the source alone.
- **Capture and centralization.** WalletConnect is now Reown, with a company behind it, a Project ID account you must hold, and a WCT token steering it. The relay sees every pairing - which dApp, which wallet, timing, origin - making it a third-party observer of users' connection graphs and a censorship chokepoint. An ownerless protocol with a revocable vendor API key in its path is not ownerless.

And the two facts that end the argument on their own: WalletConnect buys FTB no safety, because safety is wallet-independent (above); and it would not even reach NeoLine, which has no in-app browser and is not a WalletConnect wallet. The entire dependency cost would be paid to reach other wallets (Neon mobile, OneGate), not the one the question started from.

## The posture, stated plainly

Mobile users get the full read experience: browse FTB, read the ballots, check claims, verify the chain - every read works with no wallet at all. Connecting and signing is a desktop action, keyless, with no relay in the path and no third party watching the connection. That is a stronger security story than mobile convenience would be, and FTB states it as a choice, not an apology.

## Reconsider if

- NeoLine (or another N3 wallet) ships a mobile in-app browser, or injects a NEP-21 `Neo.DapiProvider` into a mobile webview. Then the zero-dependency injected path reaches mobile and FTB extends the adapter to it, no dependency added.
- A genuinely canon-acceptable bridge emerges: self-hostable with no third-party account, a minimal auditable client, isolable from the signing surface. No such option exists today.

## Honest limits (named, not hidden)

- This supersedes the earlier internal recommendation to add a WalletConnect seam for mobile reach. That recommendation correctly held that FTB's safety is wallet-independent; the final decision weighs the dependency cost higher than the mobile-reach benefit and declines it.
- Detection in `ftb-dapi.js` is NeoLine-keyed today, so in practice the supported wallet is NeoLine on desktop. Generalizing detection to any injected desktop dAPI provider is open and encouraged; it is a zero-dependency change, not a policy exception.
- This is a reach-versus-purity choice, not a safety claim, and it is named here in the open so the trade is legible: FTB trades mobile signing for zero dependency, a self-only CSP, and reproducibility.

## Replay (verify the claims above)

- **Self-only CSP:** view-source any page on the site; the `Content-Security-Policy` meta is `default-src 'self'; script-src 'self'; ...` with no third-party origin.
- **No third-party code:** the deploy gate runs `ops/ipfs/ftb-canon-lint.js` over `web/` and refuses any external script, style, or fetch; the published bundle's IPFS CID is the byte-for-byte proof of what shipped.
- **Wallet-independent safety:** see `web/ftb-dapi.js` (every write is a pure plan builder, `CalledByEntry` witness scope) and [`KEYLESS_ATTESTATION.md`](KEYLESS_ATTESTATION.md) (the contracts hold no reachable key).
