// Reference neotest suite for the lend market. Wires the whole system (token, mint-caller,
// mock bNEO, oracle, registry, lend) and exercises supply, collateral, borrow within and
// over the LTV, repay, and a liquidation triggered by a price drop. Everything is no-mint:
// the lend contract only ever moves FTB that suppliers deposited.
//
// This is the highest-risk module and ships behind its own audit; treat this suite as a
// starting point for a far more exhaustive one (interest accrual over time, rounding,
// partial liquidations, oracle-pause behavior).
//
// Run: go test ./contracts/lend/
package lend_test

import (
	"math/big"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const dec = 100_000_000

type sys struct {
	e                             *neotest.Executor
	owner, rep                    neotest.Signer
	ftb, caller, bneo, orac, lend util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	s := &sys{e: e, owner: owner}

	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	s.ftb = tokC.Hash

	callC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/testdata/caller/caller.go", "../ftb_token/testdata/caller/caller.yml")
	e.DeployContract(t, callC, nil)
	s.caller = callC.Hash
	e.NewInvoker(s.ftb, owner).Invoke(t, stackitem.Null{}, "setMigrator", s.caller.BytesBE())

	bneoC := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/mocktoken/mocktoken.go", "testdata/mocktoken/mocktoken.yml")
	e.DeployContract(t, bneoC, nil)
	s.bneo = bneoC.Hash

	oracC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../oracle/oracle.go", "../oracle/oracle.yml")
	e.DeployContract(t, oracC, owner.ScriptHash().BytesBE())
	s.orac = oracC.Hash
	oOwner := e.NewInvoker(s.orac, owner)
	oOwner.Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 1)
	s.rep = e.NewAccount(t)
	oOwner.Invoke(t, stackitem.Null{}, "setReporter", s.rep.ScriptHash().BytesBE(), true)
	e.NewInvoker(s.orac, s.rep).Invoke(t, stackitem.Null{}, "report", s.rep.ScriptHash().BytesBE(), s.bneo.BytesBE(), 50*dec)

	regC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../registry/registry.go", "../registry/registry.yml")
	e.DeployContract(t, regC, owner.ScriptHash().BytesBE())
	rOwner := e.NewInvoker(regC.Hash, owner)
	rOwner.Invoke(t, stackitem.Null{}, "setInitialDelay", 1)
	rOwner.Invoke(t, stackitem.Null{}, "bootstrap", "oracle", s.orac.BytesBE())

	lendC := neotest.CompileFile(t, e.Validator.ScriptHash(), "lend.go", "lend.yml")
	e.DeployContract(t, lendC, owner.ScriptHash().BytesBE())
	s.lend = lendC.Hash
	lOwner := e.NewInvoker(s.lend, owner)
	lOwner.Invoke(t, stackitem.Null{}, "configure", regC.Hash.BytesBE(), s.ftb.BytesBE(), s.bneo.BytesBE())
	lOwner.Invoke(t, stackitem.Null{}, "setParams", 6000, 7500, 800, 0)
	return s
}

func (s *sys) mintFTB(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.caller, s.owner).Invoke(t, stackitem.Null{}, "mint", s.ftb.BytesBE(), to.BytesBE(), amt)
}
func (s *sys) mintBNEO(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.bneo, s.owner).Invoke(t, stackitem.Null{}, "mint", to.BytesBE(), amt)
}
func (s *sys) ftbBal(t *testing.T, who util.Uint160) *neotest.ContractInvoker {
	return s.e.NewInvoker(s.ftb, s.owner)
}

func TestSupplyBorrowRepay(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)

	// supplier deposits 4000 FTB
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.e.NewInvoker(s.ftb, supplier).Invoke(t, stackitem.Make(true), "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 4000*dec, []any{"supply"})

	// borrower posts 100 bNEO (price 50 -> value 5000 FTB, max borrow 60% = 3000)
	s.mintBNEO(t, borrower.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)

	lendBorrower := s.e.NewInvoker(s.lend, borrower)
	lendOwner := s.e.NewInvoker(s.lend, s.owner)

	// borrow 1000 ok
	lendBorrower.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)
	s.ftbBal(t, borrower.ScriptHash()).Invoke(t, stackitem.Make(1000*dec), "balanceOf", borrower.ScriptHash().BytesBE())
	lendOwner.Invoke(t, stackitem.Make(1000*dec), "debtOf", borrower.ScriptHash().BytesBE())

	// borrowing past the LTV is rejected (1000 + 2500 = 3500 > 3000)
	lendBorrower.InvokeFail(t, "exceeds LTV", "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)

	// repay 1000 -> debt zero
	s.e.NewInvoker(s.ftb, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"repay"})
	lendOwner.Invoke(t, stackitem.Make(0), "debtOf", borrower.ScriptHash().BytesBE())
}

func TestLiquidationOnPriceDrop(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	liquidator := s.e.NewAccount(t)

	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.e.NewInvoker(s.ftb, supplier).Invoke(t, stackitem.Make(true), "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 4000*dec, []any{"supply"})

	s.mintBNEO(t, borrower.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)

	lendBorrower := s.e.NewInvoker(s.lend, borrower)
	lendOwner := s.e.NewInvoker(s.lend, s.owner)

	// borrow 2500 (within the 3000 cap at price 50)
	lendBorrower.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)
	lendOwner.Invoke(t, stackitem.Make(false), "isUnhealthy", borrower.ScriptHash().BytesBE())

	// bNEO price falls 50 -> 20. Collateral value 2000, threshold 75% = 1500, debt 2500 -> unhealthy
	s.e.NewInvoker(s.orac, s.rep).Invoke(t, stackitem.Null{}, "report", s.rep.ScriptHash().BytesBE(), s.bneo.BytesBE(), 20*dec)
	lendOwner.Invoke(t, stackitem.Make(true), "isUnhealthy", borrower.ScriptHash().BytesBE())

	// liquidator repays 1000 FTB and seizes bNEO at an 8% bonus:
	// seizeFTB = 1080, at price 20 that is 54 bNEO
	s.mintFTB(t, liquidator.ScriptHash(), 2000*dec)
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"liq", borrower.ScriptHash()})

	// borrower debt down to 1500, liquidator holds 54 bNEO
	lendOwner.Invoke(t, stackitem.Make(1500*dec), "debtOf", borrower.ScriptHash().BytesBE())
	s.e.NewInvoker(s.bneo, s.owner).Invoke(t, stackitem.Make(54*dec), "balanceOf", liquidator.ScriptHash().BytesBE())
}

// FTB-07: the liquidation bonus is capped at 30%; above that SetParams must reject.
func TestLiquidationBonusIsBounded(t *testing.T) {
	s := setup(t)
	lOwner := s.e.NewInvoker(s.lend, s.owner)
	lOwner.Invoke(t, stackitem.Null{}, "setParams", 6000, 7500, 3000, 0) // at the cap, allowed
	lOwner.InvokeFail(t, "bad params", "setParams", 6000, 7500, 3001, 0) // over the cap, rejected
}

// FTB-08: a supply that would mint zero shares (here a zero-value supply) is rejected rather
// than silently taking the deposit for nothing.
func TestSupplyRejectsZeroShares(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	// a 0-amount transfer still invokes onNEP17Payment -> supplyIn(…, 0) -> shares 0 -> revert
	s.e.NewInvoker(s.ftb, supplier).InvokeFail(t, "amount too small", "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 0, []any{"supply"})
}

// FTB-10: a debt-free user can withdraw collateral even while the oracle is paused (too few
// fresh prices). We pause it by requiring 2 reports when only 1 reporter exists.
func TestDebtFreeWithdrawDuringOraclePause(t *testing.T) {
	s := setup(t)
	user := s.e.NewAccount(t)

	// user posts 100 bNEO as collateral and takes no loan (debt-free)
	s.mintBNEO(t, user.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, user).Invoke(t, stackitem.Make(true), "transfer",
		user.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)

	// pause the oracle: require 2 fresh reports when only 1 reporter is configured
	s.e.NewInvoker(s.orac, s.owner).Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 2)

	// sanity: a price-dependent read now reverts, proving the oracle is paused
	lendOwner := s.e.NewInvoker(s.lend, s.owner)
	lendOwner.InvokeFail(t, "not enough fresh prices", "collateralValue", user.ScriptHash().BytesBE())

	// the debt-free user still withdraws all collateral (no oracle call on this path)
	s.e.NewInvoker(s.lend, user).Invoke(t, stackitem.Null{}, "withdrawCollateral",
		user.ScriptHash().BytesBE(), 100*dec, 0)
	s.e.NewInvoker(s.bneo, s.owner).Invoke(t, stackitem.Make(100*dec), "balanceOf", user.ScriptHash().BytesBE())
}

// FTB-07: when a position is NOT underwater, a single liquidation is capped at half the debt.
func TestCloseFactorCapsLiquidation(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	liquidator := s.e.NewAccount(t)

	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.e.NewInvoker(s.ftb, supplier).Invoke(t, stackitem.Make(true), "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 4000*dec, []any{"supply"})

	// borrower posts 100 bNEO at price 50 (value 5000) and borrows 2500 (within 60% LTV = 3000)
	s.mintBNEO(t, borrower.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)

	// price dips 50 -> 30: collateral value 3000 (>= debt 2500, so NOT underwater); threshold
	// 75% = 2250, debt 2500 -> unhealthy. Close factor caps one repay at 2500/2 = 1250.
	s.e.NewInvoker(s.orac, s.rep).Invoke(t, stackitem.Null{}, "report", s.rep.ScriptHash().BytesBE(), s.bneo.BytesBE(), 30*dec)

	// liquidator offers 2000 FTB, but only 1250 is accepted; the rest is refunded
	s.mintFTB(t, liquidator.ScriptHash(), 2000*dec)
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 2000*dec, []any{"liq", borrower.ScriptHash()})

	lendOwner := s.e.NewInvoker(s.lend, s.owner)
	lendOwner.Invoke(t, stackitem.Make(1250*dec), "debtOf", borrower.ScriptHash().BytesBE()) // 2500 - 1250
	// seize = 1250 * 1.08 = 1350 FTB-value, at price 30 -> 45 bNEO
	s.e.NewInvoker(s.bneo, s.owner).Invoke(t, stackitem.Make(45*dec), "balanceOf", liquidator.ScriptHash().BytesBE())
	// liquidator refunded 2000 - 1250 = 750 FTB
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(750*dec), "balanceOf", liquidator.ScriptHash().BytesBE())
}

// Two suppliers get shares in proportion; at price 1 (no interest) a share is worth one FTB.
func TestMultipleSuppliersShareProportionally(t *testing.T) {
	s := setup(t)
	a := s.e.NewAccount(t)
	b := s.e.NewAccount(t)
	s.mintFTB(t, a.ScriptHash(), 1000*dec)
	s.mintFTB(t, b.ScriptHash(), 3000*dec)
	lo := s.e.NewInvoker(s.lend, s.owner)

	s.e.NewInvoker(s.ftb, a).Invoke(t, stackitem.Make(true), "transfer",
		a.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"supply"})
	lo.Invoke(t, stackitem.Make(1000*dec), "sharesOf", a.ScriptHash().BytesBE())

	s.e.NewInvoker(s.ftb, b).Invoke(t, stackitem.Make(true), "transfer",
		b.ScriptHash().BytesBE(), s.lend.BytesBE(), 3000*dec, []any{"supply"})
	lo.Invoke(t, stackitem.Make(3000*dec), "sharesOf", b.ScriptHash().BytesBE())
	lo.Invoke(t, stackitem.Make(4000*dec), "totalShares")

	// a withdraws all its shares and gets its 1000 FTB back
	s.e.NewInvoker(s.lend, a).Invoke(t, stackitem.Null{}, "withdrawSupply", a.ScriptHash().BytesBE(), 1000*dec)
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(1000*dec), "balanceOf", a.ScriptHash().BytesBE())
	lo.Invoke(t, stackitem.Make(0), "sharesOf", a.ScriptHash().BytesBE())
}

// A supplier cannot withdraw more than the cash left after funds are lent out.
func TestWithdrawSupplyUtilizationLock(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 2000*dec)
	s.e.NewInvoker(s.ftb, supplier).Invoke(t, stackitem.Make(true), "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 2000*dec, []any{"supply"})
	s.mintBNEO(t, borrower.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)

	ls := s.e.NewInvoker(s.lend, supplier)
	ls.InvokeFail(t, "not enough cash", "withdrawSupply", supplier.ScriptHash().BytesBE(), 2000*dec)
	ls.Invoke(t, stackitem.Null{}, "withdrawSupply", supplier.ScriptHash().BytesBE(), 1000*dec)
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(1000*dec), "balanceOf", supplier.ScriptHash().BytesBE())
}

// Withdrawing collateral that would push the borrower past the LTV is rejected; a safe amount is allowed.
func TestWithdrawCollateralHealthCheck(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.e.NewInvoker(s.ftb, supplier).Invoke(t, stackitem.Make(true), "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 4000*dec, []any{"supply"})
	s.mintBNEO(t, borrower.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)
	// 50 bNEO out would leave value 2500, max borrow 1500 < debt 2500 -> rejected
	lb.InvokeFail(t, "undercollateralized", "withdrawCollateral", borrower.ScriptHash().BytesBE(), 50*dec, 0)
	// 10 bNEO out leaves value 4500, max borrow 2700 >= debt 2500 -> allowed
	lb.Invoke(t, stackitem.Null{}, "withdrawCollateral", borrower.ScriptHash().BytesBE(), 10*dec, 0)
	s.e.NewInvoker(s.bneo, s.owner).Invoke(t, stackitem.Make(10*dec), "balanceOf", borrower.ScriptHash().BytesBE())
}

// With a non-zero rate, debt grows over blocks: accrue books interest with no mint. Asserted as
// a strict increase to stay robust to the exact block delta (uses InvokeAndCheck).
func TestInterestAccrues(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setParams", 6000, 7500, 800, 1_000_000_000_000) // 1e12/block, under maxRate
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.e.NewInvoker(s.ftb, supplier).Invoke(t, stackitem.Make(true), "transfer",
		supplier.ScriptHash().BytesBE(), s.lend.BytesBE(), 4000*dec, []any{"supply"})
	s.mintBNEO(t, borrower.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.bneo, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)
	for i := 0; i < 50; i++ {
		s.e.AddNewBlock(t)
	}
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1*dec) // triggers accrue
	s.e.NewInvoker(s.lend, s.owner).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		got := stack[0].Value().(*big.Int)
		want := big.NewInt(1001 * dec)
		if got.Cmp(want) <= 0 {
			t.Fatalf("expected debt > %s after accrual, got %s", want, got)
		}
	}, "debtOf", borrower.ScriptHash().BytesBE())
}
