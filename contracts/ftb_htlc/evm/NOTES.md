# FtbHtlc (Neo X) - security notes and linter dispositions

Governed by CANONICAL_BIBLE_SOL.md. This file is the contract's README security section: the
declared critical functions, the linter dispositions recorded in the open (Bible Book III: an
accepted finding is recorded, never silently suppressed), and the named refinements.

## Critical functions (PROVE-tier)

- `lock` / `claim` / `refund` - the swap state machine (NONE -> LOCKED -> CLAIMED | REFUNDED).
- the SHA-256 preimage check in `claim`.
- the conservation law: every escrow is paid out exactly once (to the recipient on a correct
  preimage, or to the sender after the timelock), never both, and never stuck LOCKED.

Covered by the Foundry suite (`test/FtbHtlc.t.sol`): lifecycle (ERC-20 + native), wrong
preimage, double-claim, refund timing, reused hashlock, validation, the token-quirk trio
(fee-on-transfer measured receipt, no-return, false-return), reentrancy (malicious native
recipient and a cross-function malicious token), and two fuzzed conservation properties
(256 runs each). The cross-chain differential KAT against the N3 neo-go contract is the next
PROVE artifact.

## Linter dispositions (forge lint / solar)

The contract carries a storage `nonReentrant` guard on every state-changing entry, which the
linter does not model, so its reentrancy detectors fire on guarded code. Each disposition is
recorded here and the specific line is annotated with a `forge-lint: disable-next-line` and its
reason. One finding (events emitted after the payout in claim/refund) was a real CEI nit and is
fixed, not suppressed: both now emit before the interaction.

| Finding | Lines | Disposition |
|---|---|---|
| reentrancy-events (claim, refund) | - | FIXED: emit moved before `_payout` (checks-effects-interactions). |
| reentrancy-events (lock) | `emit Locked` | Accepted. The emit follows the measured-receipt pull (the pull is required to know the delivered amount, Book II point 9); `nonReentrant` blocks re-entry, so logs cannot be reordered. |
| reentrancy-no-eth (`_safeTransfer`, `_safeTransferFrom`) | token `.call` | Accepted. `nonReentrant` guards every entry; in lock the pull precedes the `swaps` write only for measured receipt; in claim/refund the state is already written before `_payout`. Proven by the cross-function reentrancy test. |
| reentrancy-eth (native payout) | `payable(to).call{value}` | Accepted. The guard is held across the call (correct reentrancy-guard semantics); proven by the malicious-recipient test. |
| arbitrary-send-eth (native payout) | `payable(to).call{value}` | Accepted by design. Funds only ever go to the address fixed at lock - that is the HTLC guarantee, not an open sink (Book IV rule 13). |
| block-timestamp (lock, refund) | timelock checks | Accepted by design. The timelock is coarse; no money pivots on a to-the-second deadline, and swap safety lives in the client timelock ladder, not the chain clock (Book VI.15 / Book IV rule 7). |

## Named refinements (Bible-tracked, not yet applied)

- Returndata-bomb bound (Book VI.13): the safe-transfer low-level calls copy the callee's full
  returndata. A bounded returndata copy (a reviewed `assembly` call limiting the copy to 32
  bytes, enumerated in the UNSAFE inventory) closes the grief fully. The operative mitigation
  today is the client asset allow-list (only vetted tokens are ever locked), and the calls parse
  their return to a single bool. To apply before mainnet.
- `optimizer_runs` is at the default 200; tune and record it against the deploy/call profile
  once the solver and integration tests fix the call mix (Book VII).

## Reproducibility

`bytecode_hash = "none"`, `cbor_metadata = false`, solc 0.8.26, `evm_version = "paris"` pinned in
`foundry.toml`. The deployed-bytecode hash and the constructor arguments are the recorded,
re-ratified artifact. `evm_version` is pinned to paris for Neo X fork compatibility; confirm
against the target fork and raise only if supported.
