// Invariant & property suite for FtbHtlc, complementing ftb_htlc_test.go.
//
// Focus areas the base suite does not cover in one place:
//   - Conservation: every escrow is paid out exactly once -- to the recipient on claim or
//     back to the sender on refund -- and the contract nets to zero, nothing created or lost.
//   - No double-settle: claim-after-refund, refund-after-claim, and settling an unknown
//     hashlock all fault; a swap has exactly one terminal transition.
//   - Independence: concurrent swaps keyed by distinct hashlocks never touch each other's
//     funds or state.
//
// Same neotest harness (sys) and helpers as ftb_htlc_test.go. Run: go test ./contracts/ftb_htlc/
package ftbhtlc_test

import "testing"

func TestConservation(t *testing.T) {
	s := setup(t)

	// Claim conservation: escrow moves alice -> contract -> bob, exactly once.
	preA, hlA := mkSwap("cons-claim")
	s.mintTok(t, s.alice.ScriptHash(), 777)
	s.lockTok(t, s.alice, 777, hlA, s.bob.ScriptHash(), s.top(t)+50)
	s.tokBal(t, s.htlc, 777)
	s.tokBal(t, s.alice.ScriptHash(), 0)
	s.claim(t, s.other, hlA, preA)
	s.tokBal(t, s.bob.ScriptHash(), 777)
	s.tokBal(t, s.htlc, 0)

	// Refund conservation: escrow returns to the sender after the timelock.
	_, hlB := mkSwap("cons-refund")
	s.mintTok(t, s.alice.ScriptHash(), 123)
	s.lockTok(t, s.alice, 123, hlB, s.bob.ScriptHash(), s.top(t)+10)
	s.e.GenerateNewBlocks(t, 12)
	s.refund(t, s.other, hlB)
	s.tokBal(t, s.alice.ScriptHash(), 123)
	s.tokBal(t, s.htlc, 0)
}

func TestNoDoubleSettle(t *testing.T) {
	s := setup(t)

	// Claim, then refund must fault (terminal state).
	pre1, hl1 := mkSwap("ds-1")
	s.mintTok(t, s.alice.ScriptHash(), 100)
	s.lockTok(t, s.alice, 100, hl1, s.bob.ScriptHash(), s.top(t)+5)
	s.claim(t, s.other, hl1, pre1)
	s.e.GenerateNewBlocks(t, 7) // past the timelock, to prove refund is blocked by state not height
	s.refundFail(t, s.other, hl1, "refund: not locked")

	// Refund, then claim must fault.
	pre2, hl2 := mkSwap("ds-2")
	s.mintTok(t, s.alice.ScriptHash(), 100)
	s.lockTok(t, s.alice, 100, hl2, s.bob.ScriptHash(), s.top(t)+5)
	s.e.GenerateNewBlocks(t, 7)
	s.refund(t, s.other, hl2)
	s.claimFail(t, s.other, hl2, pre2, "claim: not locked")
}

func TestSettleOnUnknown(t *testing.T) {
	s := setup(t)
	pre, hl := mkSwap("unknown")
	s.claimFail(t, s.other, hl, pre, "claim: not locked")
	s.refundFail(t, s.other, hl, "refund: not locked")
	s.stateOf(t, hl, 0) // NONE
}

func TestIndependentSwaps(t *testing.T) {
	s := setup(t)
	pre1, hl1 := mkSwap("ind-1")
	_, hl2 := mkSwap("ind-2")
	s.mintTok(t, s.alice.ScriptHash(), 1500)

	s.lockTok(t, s.alice, 500, hl1, s.bob.ScriptHash(), s.top(t)+50)     // claimable, to bob
	s.lockTok(t, s.alice, 1000, hl2, s.solver.ScriptHash(), s.top(t)+10) // refundable, to solver
	s.tokBal(t, s.htlc, 1500)

	// Settle the first without disturbing the second.
	s.claim(t, s.other, hl1, pre1)
	s.tokBal(t, s.bob.ScriptHash(), 500)
	s.tokBal(t, s.htlc, 1000)
	s.stateOf(t, hl1, 2) // CLAIMED
	s.stateOf(t, hl2, 1) // still LOCKED

	// Refund the second back to its sender (alice), recipient (solver) notwithstanding.
	s.e.GenerateNewBlocks(t, 12)
	s.refund(t, s.other, hl2)
	s.stateOf(t, hl2, 3) // REFUNDED
	s.tokBal(t, s.alice.ScriptHash(), 1000)
	s.tokBal(t, s.solver.ScriptHash(), 0)
	s.tokBal(t, s.htlc, 0)
}
