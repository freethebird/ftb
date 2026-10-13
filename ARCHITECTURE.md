# FTB Architecture

One language end to end: Go, via neo-go (contracts, backend, and node). The audited
Flamingo swap is reused as a deployed NEF and called by hash, so it never drags another
language into the repo.

## The production MO, applied

1. **Visible cost traded for an invisible one.** Every power is explicit and on-chain:
   the owner is passed at deploy (no hidden constant), the cap is a constant anyone can
   read, and the irreversible acts (CloseMigration, RenounceOwnership) are public,
   event-emitting, one-way doors. We pay a visible cost (an explicit admin step, an event)
   to remove an invisible one (a silent key, an unlogged change). Every state change emits
   an event. Methods fail loudly with a reason, never silently.
2. **Never leave stones unturned.** Every input is validated (length, sign, bounds,
   witness). Missing storage is handled, not assumed. Every code path has a test.
3. **Speed, weight, security, productivity.** Single-byte storage prefixes, minimal
   storage writes, no loops in hot paths, simple integer types. One language (Go) for
   contracts and tooling keeps the team fast.
4. **Audit every step.** Each module is small and independently testable with neotest.
   Nothing reaches mainnet without two independent audits, a public TestNet with a bug
   bounty, and a guarded low-cap launch.
5. **Modular via seams.** Each concern is its own contract. Contracts reference each
   other through stored hashes (the seams) and a Registry, so a module can be replaced
   without rewriting the others. The token, which holds the supply invariant, is sealed
   and made immutable; the peripheral modules are upgradeable behind multisig and timelock.

## Modules (each its own contract = its own seam)

| Module | Role | Mutability |
|---|---|---|
| `ftb_token` | Capped NEP-17. The only mint path is the Migrator; then sealed. | Immutable after launch (CloseMigration + RenounceOwnership) |
| `ftb_migrator` | Snapshot Merkle claim. One claim per address, attacker excluded. | One-shot, inert after Close |
| `registry` | Holds the current hashes of the swappable modules. The seam board. | Upgradeable behind multisig + timelock |
| swap (reused) | Flamingo's audited Factory/Pair/Router, deployed NEF, called by hash. | External, audited |
| `oracle` | Price feed for lend collateral. Hardened, multi-source, time-weighted. | Upgradeable module (later phase) |
| `lend` | No-mint supply/borrow market. Its own project, its own audit. | Separate later phase |

## The seam rule

Where a module must call another, it resolves the target hash at call time (from storage
or the Registry), never by hardcoding. To upgrade module B, deploy B2 and point the
Registry (or the referencing field) at B2 under governance. A, C, D are untouched. The one
exception is the token: it is designed to become referenceless and immutable, because the
supply invariant must never be upgradeable.

## What is immutable vs upgradeable

- **Immutable after launch:** the token (no mint after CloseMigration, no admin after
  RenounceOwnership, upgrade renounced) and the migrator (becomes inert after Close).
- **Upgradeable behind multisig + timelock:** the registry, the oracle, and the future
  lend market. Every upgrade is announced, delayed by a timelock, and emits an event, so
  it is a visible cost, never a silent swap.

## The client layer (`web/`)

NeoLine and other dApi wallets reach the contracts through `web/ftb-dapi.js`, a dependency-free
adapter that builds one typed invocation per action and reads chain state. It holds no keys and
enforces nothing the contracts do not; it sits entirely above the signing boundary, so a fault in
the client cannot move funds, only ask the wallet to sign something the user reviews first. Builders
are pure (build, inspect `plan.summary`, then `execute`), writes default to the least-privilege
CalledByEntry scope, and contract hashes come from config, never hard-coded. `web/invocations.md` is
the wallet-neutral spec and `web/demo.html` is a read-only-by-default reference page. See
`FTB_Stack_Map.png` for where this sits in the whole stack.

## Build order (safety-first sequencing)

- Increment 1 (this): `ftb_token` + tests.
- Increment 2: `ftb_migrator` + the off-chain Merkle-root builder that pairs with its leaf
  format, + tests.
- Increment 3: `registry` + the Go deploy/wire driver.
- Launch 1: token + migration + reused swap, seeded with protocol-owned liquidity.
- Launch 2 (separate audit): `oracle` + `lend`.

Lending is deliberately not in the first launch. It is the highest-risk primitive and the
exact class of bug that destroyed Flamingo, so it ships later, on its own, with its own
audit.
