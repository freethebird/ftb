// Invariant & property suite for FtbLoanBook, complementing ftb_loanbook_test.go.
//
// Focus areas the base suite does not cover:
//   - Conservation: after any mix of post / take / repay / default / cancel, the
//     book's FTB and NEO balances reconcile exactly to (escrowed principal still
//     open) + (proceeds credited but not yet withdrawn). Nothing is created or lost.
//   - State-machine exhaustiveness: every illegal transition faults with its exact
//     message, and no loan can be settled twice (double-repay, double-close,
//     close-after-repay, repay-after-default, take-twice, cancel-after-take,
//     withdraw-twice).
//   - GAS as collateral (the base suite only exercises NEO).
//   - Interest arithmetic: zero-rate, sub-unit flooring, and a large exact case.
//
// Same neotest harness (sys) as ftb_loanbook_test.go. Run: go test ./contracts/ftb_loanbook/
package ftbloanbook_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

// ---- extra helpers on the shared *sys (same package, split across files) ----

func (s *sys) fundGas(t *testing.T, to util.Uint160, amtBase int) {
	s.e.NewInvoker(s.gasH, s.e.Validator).Invoke(t, stackitem.Make(true), "transfer",
		s.e.Validator.ScriptHash().BytesBE(), to.BytesBE(), amtBase, nil)
}
func (s *sys) postOfferGas(t *testing.T, lender neotest.Signer, principal, colAmt, rate, termMs int) {
	s.e.NewInvoker(s.ftb, lender).Invoke(t, stackitem.Make(true), "transfer",
		lender.ScriptHash().BytesBE(), s.book.BytesBE(), principal,
		[]any{"offer", s.gasH.BytesBE(), colAmt, rate, termMs})
}
func (s *sys) takeGas(t *testing.T, borrower neotest.Signer, colAmt, id int) {
	s.e.NewInvoker(s.gasH, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.book.BytesBE(), colAmt, []any{"take", id})
}
func (s *sys) gasBal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.gasH, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) repayAmtOf(t *testing.T, id, want int) {
	s.e.NewInvoker(s.book, s.owner).Invoke(t, stackitem.Make(want), "repayAmountOf", id)
}
func (s *sys) cancel(t *testing.T, who neotest.Signer, id int) {
	s.e.NewInvoker(s.book, who).Invoke(t, stackitem.Null{}, "cancelOffer", id)
}
func (s *sys) closeLoan(t *testing.T, who neotest.Signer, id int) {
	s.e.NewInvoker(s.book, who).Invoke(t, stackitem.Null{}, "close", id)
}
func (s *sys) repayFail(t *testing.T, payer neotest.Signer, amt, id int, msg string) {
	s.e.NewInvoker(s.ftb, payer).InvokeFail(t, msg, "transfer",
		payer.ScriptHash().BytesBE(), s.book.BytesBE(), amt, []any{"repay", id})
}
func (s *sys) closeFail(t *testing.T, who neotest.Signer, id int, msg string) {
	s.e.NewInvoker(s.book, who).InvokeFail(t, msg, "close", id)
}
func (s *sys) cancelFail(t *testing.T, who neotest.Signer, id int, msg string) {
	s.e.NewInvoker(s.book, who).InvokeFail(t, msg, "cancelOffer", id)
}
func (s *sys) withdrawFail(t *testing.T, who neotest.Signer, token util.Uint160, msg string) {
	s.e.NewInvoker(s.book, who).InvokeFail(t, msg, "withdraw", who.ScriptHash().BytesBE(), token.BytesBE())
}
func (s *sys) takeNeoFail(t *testing.T, borrower neotest.Signer, colAmt, id int, msg string) {
	s.e.NewInvoker(s.neoH, borrower).InvokeFail(t, msg, "transfer",
		borrower.ScriptHash().BytesBE(), s.book.BytesBE(), colAmt, []any{"take", id})
}

// TestStateMachineExhaustive drives every terminal state and asserts that no
// further transition out of it succeeds, and that a live offer cannot be taken,
// cancelled or settled in an illegal order.
func TestStateMachineExhaustive(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 10000)
	s.fundNeo(t, s.borrower.ScriptHash(), 100)
	// FTB buffer so the fail-attempts reach the contract and fault, instead of
	// halting at the token on an insufficient balance (a false return is not a revert).
	s.mintFTB(t, s.borrower.ScriptHash(), 5000)

	// --- a REPAID loan is terminal ---
	s.postOfferNeo(t, s.lender, 1000, 10, 500, minTermMs) // id 0
	s.takeNeo(t, s.borrower, 10, 0)
	s.mintFTB(t, s.borrower.ScriptHash(), 50) // interest
	s.repay(t, s.borrower, 1050, 0)
	s.status(t, 0, 3)
	s.repayFail(t, s.borrower, 1050, 0, "repay: loan not active") // no double-repay
	s.closeFail(t, s.other, 0, "close: loan not active")          // no close-after-repay
	s.cancelFail(t, s.lender, 0, "cancel: offer not open")        // no cancel-after-settle

	// --- a DEFAULTED loan is terminal ---
	s.postOfferNeo(t, s.lender, 1000, 10, 500, minTermMs) // id 1
	s.takeNeo(t, s.borrower, 10, 1)
	s.warp(t, minTermMs+1000)
	s.closeLoan(t, s.other, 1)
	s.status(t, 1, 4)
	s.closeFail(t, s.other, 1, "close: loan not active")          // no double-close
	s.repayFail(t, s.borrower, 1050, 1, "repay: loan not active") // no repay-after-default
	s.takeNeoFail(t, s.borrower, 10, 1, "take: offer not open")   // cannot re-take

	// --- a TAKEN offer cannot be re-taken or cancelled ---
	s.postOfferNeo(t, s.lender, 1000, 10, 0, minTermMs) // id 2, zero interest
	s.takeNeo(t, s.borrower, 10, 2)
	s.takeNeoFail(t, s.borrower, 10, 2, "take: offer not open")
	s.cancelFail(t, s.lender, 2, "cancel: offer not open")

	// --- the pull ledger drains exactly once ---
	s.warp(t, minTermMs+1000)
	s.closeLoan(t, s.other, 2)
	s.withdraw(t, s.lender, s.neoH)
	s.withdrawFail(t, s.lender, s.neoH, "withdraw: nothing to claim") // no double-withdraw
	s.withdrawFail(t, s.other, s.neoH, "withdraw: nothing to claim")  // never-credited party
}

// TestConservationMultiLoan runs three offers with three outcomes (cancel, repay,
// default) against one lender and asserts the book's FTB and NEO balances reconcile
// to the live escrow + unwithdrawn proceeds at every milestone, and drain to zero.
func TestConservationMultiLoan(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 6000)
	s.fundNeo(t, s.borrower.ScriptHash(), 50)

	// three open offers: book escrows every principal, nothing lost.
	s.postOfferNeo(t, s.lender, 2000, 20, 500, minTermMs)  // id 0 -> will repay (5%)
	s.postOfferNeo(t, s.lender, 3000, 30, 1000, minTermMs) // id 1 -> will default (10%)
	s.postOfferNeo(t, s.lender, 1000, 10, 0, minTermMs)    // id 2 -> will cancel
	s.ftbBal(t, s.book, 6000)
	s.ftbBal(t, s.lender.ScriptHash(), 0)

	// cancel id 2: principal refunded, book down by exactly that principal.
	s.cancel(t, s.lender, 2)
	s.status(t, 2, 5)
	s.ftbBal(t, s.book, 5000)
	s.ftbBal(t, s.lender.ScriptHash(), 1000)

	// take id 0 and id 1: principals released, collateral escrowed.
	s.takeNeo(t, s.borrower, 20, 0)
	s.takeNeo(t, s.borrower, 30, 1)
	s.ftbBal(t, s.book, 0)  // both principals out
	s.neoBal(t, s.book, 50) // both collaterals in
	s.ftbBal(t, s.borrower.ScriptHash(), 5000)
	s.repayAmtOf(t, 0, 2100) // 2000 + 5%
	s.repayAmtOf(t, 1, 3300) // 3000 + 10%

	// repay id 0: proceeds credited to the lender's pull ledger, collateral returned.
	s.repay(t, s.borrower, 2100, 0)
	s.status(t, 0, 3)
	s.ftbBal(t, s.book, 2100) // repaid FTB sits in the book until withdrawn
	s.neoBal(t, s.book, 30)   // only id 1 collateral remains
	s.neoBal(t, s.borrower.ScriptHash(), 20)
	s.claimable(t, s.lender.ScriptHash(), s.ftb, 2100)

	// default id 1: collateral credited to the lender, no outbound transfer.
	s.warp(t, minTermMs+1000)
	s.closeLoan(t, s.other, 1)
	s.status(t, 1, 4)
	s.neoBal(t, s.book, 30) // physically unchanged; now claimable
	s.claimable(t, s.lender.ScriptHash(), s.neoH, 30)

	// invariant: book FTB == unwithdrawn proceeds (2100), book NEO == defaulted collateral (30).
	s.ftbBal(t, s.book, 2100)
	s.neoBal(t, s.book, 30)

	// withdraw both: the book drains to exactly zero, the lender is made whole.
	s.withdraw(t, s.lender, s.ftb)
	s.ftbBal(t, s.book, 0)
	s.ftbBal(t, s.lender.ScriptHash(), 3100) // 1000 refund + 2100 proceeds
	s.withdraw(t, s.lender, s.neoH)
	s.neoBal(t, s.book, 0)
	s.neoBal(t, s.lender.ScriptHash(), 30)
	s.claimable(t, s.lender.ScriptHash(), s.ftb, 0)
	s.claimable(t, s.lender.ScriptHash(), s.neoH, 0)
}

// TestGasCollateral exercises the full lifecycle with GAS (8-decimal) collateral.
// The book holds no NEO here, so there is no GAS accrual to perturb balances.
func TestGasCollateral(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 1000)
	s.fundGas(t, s.borrower.ScriptHash(), 100_0000_0000) // 100 GAS: collateral + fees

	s.postOfferGas(t, s.lender, 1000, 10_0000_0000, 500, minTermMs) // 10 GAS collateral
	s.ftbBal(t, s.book, 1000)

	s.takeGas(t, s.borrower, 10_0000_0000, 0)
	s.status(t, 0, 2)
	s.gasBal(t, s.book, 10_0000_0000) // exactly the collateral, no accrual
	s.ftbBal(t, s.borrower.ScriptHash(), 1000)

	s.mintFTB(t, s.borrower.ScriptHash(), 50)
	s.repay(t, s.borrower, 1050, 0)
	s.status(t, 0, 3)
	s.gasBal(t, s.book, 0) // collateral returned in full
	s.claimable(t, s.lender.ScriptHash(), s.gasH, 0)
	s.claimable(t, s.lender.ScriptHash(), s.ftb, 1050)
	s.withdraw(t, s.lender, s.ftb)
	s.ftbBal(t, s.lender.ScriptHash(), 1050)
}

// TestInterestRoundingAndZero pins the flat-interest formula principal+principal*rate/bps:
// zero rate -> principal; sub-unit interest floors to zero; a large exact case.
func TestInterestRoundingAndZero(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 100001003)
	s.fundNeo(t, s.borrower.ScriptHash(), 3)

	s.postOfferNeo(t, s.lender, 1000, 1, 0, minTermMs) // id 0: zero interest
	s.takeNeo(t, s.borrower, 1, 0)
	s.repayAmtOf(t, 0, 1000)

	s.postOfferNeo(t, s.lender, 3, 1, 1, minTermMs) // id 1: 3 * 1 / 10000 == 0 (floors)
	s.takeNeo(t, s.borrower, 1, 1)
	s.repayAmtOf(t, 1, 3)

	s.postOfferNeo(t, s.lender, 100000000, 1, 1, minTermMs) // id 2: 1e8 * 1 / 10000 == 10000
	s.takeNeo(t, s.borrower, 1, 2)
	s.repayAmtOf(t, 2, 100010000)
}
