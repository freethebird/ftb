// Neotest suite for the ORACLELESS, KEYLESS term-loan book. Exercises the full lifecycle --
// post, take, repay-before-maturity, overpay refund, maturity default + permissionless close,
// cancel -- plus validation and the okey byte-mask regression (ids >= 256). No oracle, no
// owner: the only authority is the clock. Block time on the test chain is 1s, so a 60,000 ms
// (minimum) term matures after ~60 generated blocks.
//
// Run: go test ./contracts/ftb_loanbook/
package ftbloanbook_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const minTermMs = 60000 // mirrors the contract minimum term (60 test blocks at 1s each)

type sys struct {
	e                              *neotest.Executor
	owner, lender, borrower, other neotest.Signer
	ftb, book, neoH, gasH          util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	s := &sys{e: e}
	s.owner = e.NewAccount(t)
	s.lender = e.NewAccount(t, 100000_0000_0000)
	s.borrower = e.NewAccount(t)
	s.other = e.NewAccount(t)
	s.neoH = e.NativeHash(t, nativenames.Neo)
	s.gasH = e.NativeHash(t, nativenames.Gas)

	ftbC := neotest.CompileFile(t, e.Validator.ScriptHash(),
		"../lend/testdata/mocktoken/mocktoken.go", "../lend/testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, ftbC, nil)
	s.ftb = ftbC.Hash

	bookC := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_loanbook.go", "ftb_loanbook.yml")
	e.DeployContract(t, bookC, s.ftb.BytesBE())
	s.book = bookC.Hash
	return s
}

func (s *sys) mintFTB(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Null{}, "mint", to.BytesBE(), amt)
}
func (s *sys) fundNeo(t *testing.T, to util.Uint160, whole int) {
	s.e.NewInvoker(s.neoH, s.e.Validator).Invoke(t, stackitem.Make(true), "transfer",
		s.e.Validator.ScriptHash().BytesBE(), to.BytesBE(), whole, nil)
}
func (s *sys) postOfferNeo(t *testing.T, lender neotest.Signer, principal, colAmt, rate, termMs int) {
	s.e.NewInvoker(s.ftb, lender).Invoke(t, stackitem.Make(true), "transfer",
		lender.ScriptHash().BytesBE(), s.book.BytesBE(), principal,
		[]any{"offer", s.neoH.BytesBE(), colAmt, rate, termMs})
}
func (s *sys) takeNeo(t *testing.T, borrower neotest.Signer, colAmt, id int) {
	s.e.NewInvoker(s.neoH, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.book.BytesBE(), colAmt, []any{"take", id})
}
func (s *sys) repay(t *testing.T, payer neotest.Signer, amt, id int) {
	s.e.NewInvoker(s.ftb, payer).Invoke(t, stackitem.Make(true), "transfer",
		payer.ScriptHash().BytesBE(), s.book.BytesBE(), amt, []any{"repay", id})
}
func (s *sys) withdraw(t *testing.T, who neotest.Signer, token util.Uint160) {
	s.e.NewInvoker(s.book, who).Invoke(t, stackitem.Null{}, "withdraw", who.ScriptHash().BytesBE(), token.BytesBE())
}
func (s *sys) ftbBal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) neoBal(t *testing.T, h util.Uint160, want int) {
	s.e.NewInvoker(s.neoH, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", h.BytesBE())
}
func (s *sys) status(t *testing.T, id, want int) {
	s.e.NewInvoker(s.book, s.owner).Invoke(t, stackitem.Make(want), "statusOf", id)
}
func (s *sys) claimable(t *testing.T, funder, token util.Uint160, want int) {
	s.e.NewInvoker(s.book, s.owner).Invoke(t, stackitem.Make(want), "claimable", funder.BytesBE(), token.BytesBE())
}

// warp advances the chain clock by ms with a single empty block whose timestamp is
// jumped forward. neotest's generated blocks step the clock only 1ms each, so a
// minute-long term cannot be crossed by block count.
func (s *sys) warp(t *testing.T, ms int) {
	b := s.e.NewUnsignedBlock(t)
	b.Timestamp = s.e.TopBlock(t).Timestamp + uint64(ms) // #nosec G115 -- ms is a small positive test constant
	s.e.SignBlock(b)
	if err := s.e.Chain.AddBlock(b); err != nil {
		t.Fatalf("warp: add block: %v", err)
	}
}

func TestHappyPath(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 1000)
	s.fundNeo(t, s.borrower.ScriptHash(), 10)

	s.postOfferNeo(t, s.lender, 1000, 10, 500, minTermMs) // 5% term
	s.status(t, 0, 1)                                     // OPEN
	s.ftbBal(t, s.book, 1000)                             // principal escrowed
	s.ftbBal(t, s.lender.ScriptHash(), 0)

	s.takeNeo(t, s.borrower, 10, 0)
	s.status(t, 0, 2) // TAKEN
	s.ftbBal(t, s.borrower.ScriptHash(), 1000)
	s.neoBal(t, s.book, 10)
	s.neoBal(t, s.borrower.ScriptHash(), 0)
	s.e.NewInvoker(s.book, s.owner).Invoke(t, stackitem.Make(1050), "repayAmountOf", 0)

	s.mintFTB(t, s.borrower.ScriptHash(), 50) // borrower sources the 50 interest
	s.repay(t, s.borrower, 1050, 0)
	s.status(t, 0, 3)                        // REPAID
	s.neoBal(t, s.borrower.ScriptHash(), 10) // collateral returned
	s.ftbBal(t, s.borrower.ScriptHash(), 0)
	s.claimable(t, s.lender.ScriptHash(), s.ftb, 1050)

	s.withdraw(t, s.lender, s.ftb)
	s.ftbBal(t, s.lender.ScriptHash(), 1050)
	s.claimable(t, s.lender.ScriptHash(), s.ftb, 0)
}

func TestDefault(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 1000)
	s.fundNeo(t, s.borrower.ScriptHash(), 10)

	s.postOfferNeo(t, s.lender, 1000, 10, 1000, minTermMs) // 10% term
	s.takeNeo(t, s.borrower, 10, 0)

	s.e.NewInvoker(s.book, s.other).InvokeFail(t, "close: not yet matured", "close", 0)

	s.warp(t, minTermMs+5000) // jump the chain clock past the term

	s.mintFTB(t, s.borrower.ScriptHash(), 200)
	s.e.NewInvoker(s.ftb, s.borrower).InvokeFail(t, "repay: past maturity", "transfer",
		s.borrower.ScriptHash().BytesBE(), s.book.BytesBE(), 1100, []any{"repay", 0})

	s.e.NewInvoker(s.book, s.other).Invoke(t, stackitem.Null{}, "close", 0)
	s.status(t, 0, 4) // DEFAULTED
	s.claimable(t, s.lender.ScriptHash(), s.neoH, 10)
	s.ftbBal(t, s.borrower.ScriptHash(), 1200) // 1000 principal + 200 minted, none repaid

	s.withdraw(t, s.lender, s.neoH)
	s.neoBal(t, s.lender.ScriptHash(), 10)
	s.claimable(t, s.lender.ScriptHash(), s.neoH, 0)
}

func TestOverpayRefund(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 1000)
	s.fundNeo(t, s.borrower.ScriptHash(), 10)
	s.postOfferNeo(t, s.lender, 1000, 10, 500, minTermMs)
	s.takeNeo(t, s.borrower, 10, 0)
	s.mintFTB(t, s.borrower.ScriptHash(), 200) // borrower now holds 1200
	s.repay(t, s.borrower, 1200, 0)            // owes 1050, overpays 150
	s.status(t, 0, 3)
	s.claimable(t, s.lender.ScriptHash(), s.ftb, 1050)
	s.ftbBal(t, s.borrower.ScriptHash(), 150) // overpay refunded
	s.neoBal(t, s.borrower.ScriptHash(), 10)  // collateral back
}

func TestCancel(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 1000)
	s.postOfferNeo(t, s.lender, 1000, 10, 500, minTermMs)
	s.ftbBal(t, s.book, 1000)
	s.ftbBal(t, s.lender.ScriptHash(), 0)

	s.e.NewInvoker(s.book, s.other).InvokeFail(t, "cancel: only funder", "cancelOffer", 0)
	s.e.NewInvoker(s.book, s.lender).Invoke(t, stackitem.Null{}, "cancelOffer", 0)
	s.status(t, 0, 5) // CANCELLED
	s.ftbBal(t, s.lender.ScriptHash(), 1000)

	s.e.NewInvoker(s.book, s.lender).InvokeFail(t, "cancel: offer not open", "cancelOffer", 0)
	s.fundNeo(t, s.borrower.ScriptHash(), 10)
	s.e.NewInvoker(s.neoH, s.borrower).InvokeFail(t, "take: offer not open", "transfer",
		s.borrower.ScriptHash().BytesBE(), s.book.BytesBE(), 10, []any{"take", 0})
}

func TestValidation(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 10000)
	in := s.e.NewInvoker(s.ftb, s.lender)
	bad := func(msg string, data []any, amt int) {
		in.InvokeFail(t, msg, "transfer", s.lender.ScriptHash().BytesBE(), s.book.BytesBE(), amt, data)
	}
	bad("offer: collateral token not allowed", []any{"offer", s.ftb.BytesBE(), 10, 500, minTermMs}, 100)
	bad("offer: rate out of range", []any{"offer", s.neoH.BytesBE(), 10, 1000001, minTermMs}, 100)
	bad("offer: term out of range", []any{"offer", s.neoH.BytesBE(), 10, 500, minTermMs - 1}, 100)
	bad("offer: collateral amount must be positive", []any{"offer", s.neoH.BytesBE(), 0, 500, minTermMs}, 100)
	bad("ftb: bad action", []any{"frobnicate"}, 100)

	s.postOfferNeo(t, s.lender, 1000, 10, 500, minTermMs) // id 0, valid
	s.fundNeo(t, s.borrower.ScriptHash(), 20)
	s.e.NewInvoker(s.neoH, s.borrower).InvokeFail(t, "take: collateral amount mismatch", "transfer",
		s.borrower.ScriptHash().BytesBE(), s.book.BytesBE(), 9, []any{"take", 0})
	s.e.NewInvoker(s.neoH, s.borrower).InvokeFail(t, "take: offer not open", "transfer",
		s.borrower.ScriptHash().BytesBE(), s.book.BytesBE(), 10, []any{"take", 99})
}

func TestLargeId(t *testing.T) {
	s := setup(t)
	s.mintFTB(t, s.lender.ScriptHash(), 300)
	in := s.e.NewInvoker(s.ftb, s.lender)
	for i := 0; i < 260; i++ {
		in.Invoke(t, stackitem.Make(true), "transfer",
			s.lender.ScriptHash().BytesBE(), s.book.BytesBE(), 1,
			[]any{"offer", s.neoH.BytesBE(), 1, 0, minTermMs})
	}
	s.status(t, 256, 1) // okey masks the low byte: id 256 stored and read correctly
	s.e.NewInvoker(s.book, s.owner).Invoke(t, stackitem.Make(260), "offerCount")
}
