# FtbHtlc - state machine

The Neo N3 settlement leg of the non-custodial Neo N3 <-> Neo X atomic swap (Module 1,
component 1). One swap is keyed by its `hashlock` (SHA-256 of a 256-bit secret). The contract
is asset-agnostic, keyless, immutable, and has no owner. This is the state of record for the
N3 contract; the full design is `../../docs/expansion/neox/HTLC_LINK_SPEC.md`.

## States (per hashlock)

```
0  NONE       the zero value: this hashlock has never been used
1  LOCKED     an asset is escrowed, awaiting claim or refund
2  CLAIMED    the recipient was paid on a correct preimage (terminal)
3  REFUNDED   the sender was repaid after the timelock (terminal)
```

`NONE` being the zero value is load-bearing twice: it distinguishes an unused hashlock from a
live swap, and the `state != NONE` test is the uniqueness guard that faults a reused hashlock.

## Transitions

```
NONE --- lock(hashlock, recipient, timelock) via OnNEP17Payment ---> LOCKED
LOCKED --- claim(hashlock, preimage),  sha256(preimage) == hashlock ---> CLAIMED
LOCKED --- refund(hashlock),           currentHeight >= timelock    ---> REFUNDED
```

Both CLAIMED and REFUNDED are terminal: there is no transition out of them, so an escrow is
paid out exactly once. Every other call faults.

### lock (value entry, the only method that receives value)

- Reached through `OnNEP17Payment(from, amount, ["lock", hashlock, recipient, timelock])`:
  the locker transfers any NEP-17 asset, which is the escrow. The calling token is the asset.
- Guards (each faults, reverting the locker's own transfer, so nothing ever half-locks):
  `amount > 0`; `len(hashlock) == 32`; `len(recipient) == 20`;
  `timelock > currentHeight + 1`; and `state == NONE` (no reused hashlock).
- Effect: records sender, recipient, token, amount, timelock; sets LOCKED; emits
  `Locked(hashlock, sender, recipient, token, amount, timelock)`.

### claim (permissionless)

- Guards: `state == LOCKED`; `len(preimage) == 32`; `crypto.Sha256(preimage) == hashlock`.
- Effect: sets CLAIMED (before the transfer), pays `amount` of `token` to the fixed
  `recipient`, emits `Claimed(hashlock, preimage)`.
- Permissionless because revealing the preimage is the authorization and funds only ever go
  to the recipient fixed at lock; the caller's identity is irrelevant. The preimage is
  emitted so the counterparty's watcher extracts it to claim the other leg.

### refund (permissionless)

- Guards: `state == LOCKED`; `currentHeight >= timelock` (block height via
  `ledger.CurrentIndex()`).
- Effect: sets REFUNDED (before the transfer), returns `amount` of `token` to the fixed
  `sender`, emits `Refunded(hashlock)`.
- Permissionless for the same reason: funds only ever go to the sender.

## Invariants

- **Conservation.** For every hashlock the escrowed amount is paid out exactly once, to the
  recipient via claim or to the sender via refund, never both; refund is always eventually
  reachable, so funds are never stuck LOCKED.
- **Uniqueness.** One live swap per hashlock; a reused hashlock faults.
- **No custody.** No path moves funds except claim and refund; no owner, admin, or third
  party can move or freeze an escrow.
- **Keyless.** No reachable admin key; `_deploy` stores only a version tag; nothing to
  renounce because nothing was ever held.
- **Checks-effects-interactions.** State is written before any outbound transfer; the
  outbound transfer checks the NEP-17 boolean (FTB-15).

## The one rule the contract cannot enforce

The cross-leg timelock ladder (T_A > T_B) is a client invariant, because only the client sees
both chains at once. The contract enforces only the local rule: refund only at or after its
own timelock, claim only with the preimage while LOCKED. See HTLC_LINK_SPEC.md, section 5.

## Storage (field-per-key, single-byte prefixes)

```
0x01            -> version tag
0x10 + hashlock -> sender
0x11 + hashlock -> recipient
0x12 + hashlock -> token (NEP-17 hash)
0x13 + hashlock -> amount
0x14 + hashlock -> timelock (block height)
0x15 + hashlock -> state
```
