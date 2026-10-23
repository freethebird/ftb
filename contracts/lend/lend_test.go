// Reference neotest suite for the lend market (v2: NEO+GAS collateral, utilization curve,
// 2/3 : 1/3 fee split). Wires the whole system (token, mint-caller, oracle, registry,
// lend) and exercises supply, NEO/GAS collateral, borrow within and over the LTV, repay,
// liquidations (NEO-first seize, spillover into GAS), interest accrual, the reserve
// split, the dividend sweep, and the APY views. NEO collateral is posted by transferring
// whole NEO from the chain validator — the real native asset, not a mock.
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

	"github.com/nspcc-dev/neo-go/pkg/core/native/nativenames"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/util"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

const dec = 100_000_000

type sys struct {
	e                            *neotest.Executor
	owner                        neotest.Signer
	reps                         []neotest.Signer // a quorum of 3 oracle reporters (O1)
	ftb, caller, orac, reg, lend util.Uint160
	neo, gas                     util.Uint160
}

func setup(t *testing.T) *sys {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)
	s := &sys{e: e, owner: owner, neo: e.NativeHash(t, nativenames.Neo), gas: e.NativeHash(t, nativenames.Gas)}

	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	s.ftb = tokC.Hash

	callC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/testdata/caller/caller.go", "../ftb_token/testdata/caller/caller.yml")
	e.DeployContract(t, callC, nil)
	s.caller = callC.Hash
	e.NewInvoker(s.ftb, owner).Invoke(t, stackitem.Null{}, "setMigrator", s.caller.BytesBE())

	oracC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../oracle/oracle.go", "../oracle/oracle.yml")
	e.DeployContract(t, oracC, owner.ScriptHash().BytesBE())
	s.orac = oracC.Hash
	oOwner := e.NewInvoker(s.orac, owner)
	// O1: a real quorum of 3 independent reporters (never a single key)
	oOwner.Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 3)
	s.reps = []neotest.Signer{e.NewAccount(t), e.NewAccount(t), e.NewAccount(t)}
	for _, r := range s.reps {
		oOwner.Invoke(t, stackitem.Null{}, "setReporter", r.ScriptHash().BytesBE(), true)
	}
	// NEO at 50 FTB (per whole NEO), GAS at 5 FTB (per whole GAS)
	s.setPrice(t, s.neo, 50*dec)
	s.setPrice(t, s.gas, 5*dec)

	regC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../registry/registry.go", "../registry/registry.yml")
	e.DeployContract(t, regC, owner.ScriptHash().BytesBE())
	s.reg = regC.Hash
	rOwner := e.NewInvoker(s.reg, owner)
	rOwner.Invoke(t, stackitem.Null{}, "setInitialDelay", 1)
	rOwner.Invoke(t, stackitem.Null{}, "bootstrap", "oracle", s.orac.BytesBE())

	lendC := neotest.CompileFile(t, e.Validator.ScriptHash(), "lend.go", "lend.yml")
	e.DeployContract(t, lendC, owner.ScriptHash().BytesBE())
	s.lend = lendC.Hash
	lOwner := e.NewInvoker(s.lend, owner)
	lOwner.Invoke(t, stackitem.Null{}, "configure", s.reg.BytesBE(), s.ftb.BytesBE())
	lOwner.Invoke(t, stackitem.Null{}, "setRisk", 6000, 7500, 800)
	lOwner.Invoke(t, stackitem.Null{}, "setRates", 0, 0, 0, 8000, 3333)
	return s
}

// setPrice moves the whole quorum to `price` for an asset, so the median actually shifts
// (a single reporter can no longer move the median — that is the O1 point). Each step stays
// within the oracle's generous sanity band, so a realistic crash/pump lands in one call.
func (s *sys) setPrice(t *testing.T, asset util.Uint160, price int) {
	for _, r := range s.reps {
		s.e.NewInvoker(s.orac, r).Invoke(t, stackitem.Null{}, "report",
			r.ScriptHash().BytesBE(), asset.BytesBE(), price)
	}
}

func (s *sys) mintFTB(t *testing.T, to util.Uint160, amt int) {
	s.e.NewInvoker(s.caller, s.owner).Invoke(t, stackitem.Null{}, "mint", s.ftb.BytesBE(), to.BytesBE(), amt)
}

// fundNeo sends whole NEO (0 decimals) from the validator to an account.
func (s *sys) fundNeo(t *testing.T, to util.Uint160, wholeNeo int) {
	s.e.NewInvoker(s.neo, s.e.Validator).Invoke(t, stackitem.Make(true), "transfer",
		s.e.Validator.ScriptHash().BytesBE(), to.BytesBE(), wholeNeo, nil)
}

// postNeo posts whole NEO as collateral.
func (s *sys) postNeo(t *testing.T, who neotest.Signer, wholeNeo int) {
	s.e.NewInvoker(s.neo, who).Invoke(t, stackitem.Make(true), "transfer",
		who.ScriptHash().BytesBE(), s.lend.BytesBE(), wholeNeo, nil)
}

func (s *sys) supply(t *testing.T, who neotest.Signer, amt int) {
	s.e.NewInvoker(s.ftb, who).Invoke(t, stackitem.Make(true), "transfer",
		who.ScriptHash().BytesBE(), s.lend.BytesBE(), amt, []any{"supply"})
}

func (s *sys) neoBal(t *testing.T, who util.Uint160, want int) {
	s.e.NewInvoker(s.neo, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", who.BytesBE())
}

func (s *sys) ftbBal(t *testing.T, who util.Uint160, want int) {
	s.e.NewInvoker(s.ftb, s.owner).Invoke(t, stackitem.Make(want), "balanceOf", who.BytesBE())
}

// ---- volatility-protection test helpers (claude/ftb-liquidation-model-study.md) ----
//
// The lagged reference only converges toward spot on WRITE paths, at maxRefStepBps of itself per
// block. Tests whose subject is the seize math (not the transient-dump protection — that is
// TestVolatility*) must settle the reference to a sustained new price first; these helpers do so
// deterministically.

// fastGuards widens the per-block reference step so a short settle fully converges the reference.
func (s *sys) fastGuards(t *testing.T) {
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setGuards", 1000, 2500)
}

// newKeeper is a large, safe borrower whose write paths advance the shared reference — standing in
// for the aggregate market activity that pulls it toward a sustained price. Create it AFTER the
// pool has supply: it borrows a dust amount so its 0/0 withdraw has a (ref-advancing) health check.
func (s *sys) newKeeper(t *testing.T) neotest.Signer {
	k := s.e.NewAccount(t)
	s.fundNeo(t, k.ScriptHash(), 1000)
	s.postNeo(t, k, 1000)
	s.e.NewInvoker(s.lend, k).Invoke(t, stackitem.Null{}, "borrow", k.ScriptHash().BytesBE(), 1*dec)
	return k
}

// settle drives the stored reference to the current spot: it mines blocks, then has the keeper
// touch a write path (a 0/0 withdrawCollateral — runs the ref-advancing health check, moves no
// collateral, applies no breaker). With fastGuards the reference converges fully.
func (s *sys) settle(t *testing.T, keeper neotest.Signer) {
	s.e.GenerateNewBlocks(t, 20)
	s.e.NewInvoker(s.lend, keeper).Invoke(t, stackitem.Null{}, "withdrawCollateral", keeper.ScriptHash().BytesBE(), 0, 0)
}

func TestSupplyBorrowRepay(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)

	// supplier deposits 4000 FTB
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)

	// borrower posts 100 whole NEO (price 50 -> value 5000 FTB, max borrow 60% = 3000)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)

	lendBorrower := s.e.NewInvoker(s.lend, borrower)
	lendOwner := s.e.NewInvoker(s.lend, s.owner)

	// borrow 1000 ok
	lendBorrower.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)
	s.ftbBal(t, borrower.ScriptHash(), 1000*dec)
	lendOwner.Invoke(t, stackitem.Make(1000*dec), "debtOf", borrower.ScriptHash().BytesBE())

	// borrowing past the LTV is rejected (1000 + 2500 = 3500 > 3000)
	lendBorrower.InvokeFail(t, "exceeds LTV", "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)

	// repay 1000 -> debt zero
	s.e.NewInvoker(s.ftb, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"repay"})
	lendOwner.Invoke(t, stackitem.Make(0), "debtOf", borrower.ScriptHash().BytesBE())
}

// Regression (TestNet report): floor division in the normalized-debt math made the
// smallest debts unkillable — once the borrow index moved above 1e18, a repay covering
// the full debt rounded the normalized reduction DOWN, stranding 1 base unit of debt
// (and its collateral) forever. Ceiling division (divUp) must close the position
// completely: a repay >= debtOf zeroes the normalized debt.
func TestRepayClosesDustDebt(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	// aggressive flat rate so the borrow index moves well above 1e18 within the test
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setRates", 1_000_000_000_000, 0, 0, 8000, 3333)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 100*dec)
	for i := 0; i < 50; i++ {
		s.e.AddNewBlock(t)
	}
	// trigger accrual; debt is now strictly greater than the borrowed 100 FTB
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1)
	// repay more than the debt: overpay is refunded, and the full debt must die
	s.mintFTB(t, borrower.ScriptHash(), 200*dec)
	s.e.NewInvoker(s.ftb, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 300*dec, []any{"repay"})
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Make(0), "debtOf", borrower.ScriptHash().BytesBE())
}

// GAS prices through the same oracle with 8-decimal scaling; NEO stays whole-unit.
func TestGasCollateralValues(t *testing.T) {
	s := setup(t)
	user := s.e.NewAccount(t) // 100 GAS from NewAccount
	s.e.NewInvoker(s.gas, user).Invoke(t, stackitem.Make(true), "transfer",
		user.ScriptHash().BytesBE(), s.lend.BytesBE(), 40*dec, nil)

	// 40 GAS at 5 FTB -> 200 FTB of collateral value
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Make(200*dec), "collateralValue", user.ScriptHash().BytesBE())
}

func TestLiquidationOnPriceDrop(t *testing.T) {
	s := setup(t)
	s.fastGuards(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	liquidator := s.e.NewAccount(t)

	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	keeper := s.newKeeper(t) // created after supply (it borrows a dust amount)

	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)

	lendBorrower := s.e.NewInvoker(s.lend, borrower)
	lendOwner := s.e.NewInvoker(s.lend, s.owner)

	// borrow 2500 (within the 3000 cap at price 50); this seeds the reference at 50
	lendBorrower.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)
	lendOwner.Invoke(t, stackitem.Make(false), "isUnhealthy", borrower.ScriptHash().BytesBE())

	// NEO price falls 50 -> 20. The lagged reference still protects the position (max basis), so the
	// raw drop alone does NOT mark it liquidatable — the transient-dump protection, covered fully by
	// TestVolatilityDumpBreakerAndRecovery. A sustained move (settle) pulls the reference down.
	s.setPrice(t, s.neo, 20*dec)
	lendOwner.Invoke(t, stackitem.Make(false), "isUnhealthy", borrower.ScriptHash().BytesBE())
	s.settle(t, keeper)
	// at the converged price: collateral value 2000, threshold 75% = 1500, debt 2500 -> unhealthy
	lendOwner.Invoke(t, stackitem.Make(true), "isUnhealthy", borrower.ScriptHash().BytesBE())

	// liquidator repays 1000 FTB and seizes NEO at an 8% bonus (seize uses spot, not the reference):
	// seizeFTB = 1080, at price 20 that is exactly 54 whole NEO
	s.mintFTB(t, liquidator.ScriptHash(), 2000*dec)
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"liq", borrower.ScriptHash()})

	// borrower debt down to 1500, liquidator holds 54 whole NEO
	lendOwner.Invoke(t, stackitem.Make(1500*dec), "debtOf", borrower.ScriptHash().BytesBE())
	s.neoBal(t, liquidator.ScriptHash(), 54)

	// the liquidation is recorded on the borrower and outlives the position:
	// the loans board reads liquidationsOf for its LIQUIDATED tag
	lendOwner.Invoke(t, stackitem.Make(1), "liquidationsOf", borrower.ScriptHash().BytesBE())
	lendOwner.Invoke(t, stackitem.Make(0), "liquidationsOf", liquidator.ScriptHash().BytesBE())
}

// When NEO alone cannot cover the seize, the remainder spills into GAS. (The sustained drop is
// settled first; the breaker/lag protection itself is covered by TestVolatility*.)
func TestLiquidationSpillsIntoGas(t *testing.T) {
	s := setup(t)
	s.fastGuards(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t, 200*dec) // posts 100 GAS as collateral, keeps the rest for fees
	liquidator := s.e.NewAccount(t)

	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	keeper := s.newKeeper(t)

	// 10 NEO (500 FTB) + 100 GAS (500 FTB) = 1000 FTB; LTV 60% -> borrow 600
	s.fundNeo(t, borrower.ScriptHash(), 10)
	s.postNeo(t, borrower, 10)
	s.e.NewInvoker(s.gas, borrower).Invoke(t, stackitem.Make(true), "transfer",
		borrower.ScriptHash().BytesBE(), s.lend.BytesBE(), 100*dec, nil) // 100 GAS = 500 FTB
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 600*dec)

	// NEO 50 -> 20 (10 NEO = 200), GAS 5 -> 2 (100 GAS = 200): cv 400 < debt 600 -> underwater
	s.setPrice(t, s.neo, 20*dec)
	s.setPrice(t, s.gas, 2*dec)
	s.settle(t, keeper) // converges the NEO reference; the liquidation itself converges the GAS one

	// liquidator repays 300: seize = 300 * 1.08 = 324 FTB. NEO covers only 200 -> all 10 NEO
	// seized, and the remaining 124 FTB spills into GAS at 2 FTB/GAS -> exactly 62 GAS.
	s.mintFTB(t, liquidator.ScriptHash(), 1000*dec)
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 300*dec, []any{"liq", borrower.ScriptHash()})

	s.neoBal(t, liquidator.ScriptHash(), 10)
	// wallet GAS balances are polluted by funding and fees, so assert on the contract's
	// own collateral bookkeeping: the borrower keeps exactly the unseized 38 GAS.
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Make(0), "collateralOf", borrower.ScriptHash().BytesBE(), true)
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Make(38*dec), "collateralOf", borrower.ScriptHash().BytesBE(), false)
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Make(300*dec), "debtOf", borrower.ScriptHash().BytesBE())
}

// FTB-07: the liquidation bonus is capped at 30%; above that SetRisk must reject.
func TestLiquidationBonusIsBounded(t *testing.T) {
	s := setup(t)
	lOwner := s.e.NewInvoker(s.lend, s.owner)
	lOwner.Invoke(t, stackitem.Null{}, "setRisk", 6000, 7500, 3000) // at the cap, allowed
	lOwner.InvokeFail(t, "bad risk params", "setRisk", 6000, 7500, 3001)
}

// The rate curve and the holders' share are both capped (FTB-07 for economics).
func TestRateParamsAreBounded(t *testing.T) {
	s := setup(t)
	lOwner := s.e.NewInvoker(s.lend, s.owner)
	lOwner.Invoke(t, stackitem.Null{}, "setRates", 0, 0, 0, 8000, 5000) // reserve at the 50% cap, allowed
	lOwner.InvokeFail(t, "bad rate params", "setRates", 0, 0, 0, 8000, 5001)
	lOwner.InvokeFail(t, "bad rate params", "setRates", 0, 0, 0, 0, 3333) // kink must be inside (0, 100%)
	lOwner.InvokeFail(t, "bad rate params", "setRates", 0, 0, 0, 10000, 3333)
	// a curve peaking above the per-block ceiling is rejected
	lOwner.InvokeFail(t, "ceiling", "setRates", 0, 0, 2_000_000_000_000_000_000, 8000, 3333)
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
// fresh prices). We pause it by requiring more fresh reports than the configured reporters supply.
func TestDebtFreeWithdrawDuringOraclePause(t *testing.T) {
	s := setup(t)
	user := s.e.NewAccount(t)

	// user posts 10 whole NEO as collateral and takes no loan (debt-free)
	s.fundNeo(t, user.ScriptHash(), 10)
	s.postNeo(t, user, 10)

	// pause the oracle: require more fresh reports (4) than the 3 configured reporters can give
	s.e.NewInvoker(s.orac, s.owner).Invoke(t, stackitem.Null{}, "setParams", 100_000_000, 4)

	// sanity: a price-dependent read now reverts, proving the oracle is paused
	lendOwner := s.e.NewInvoker(s.lend, s.owner)
	lendOwner.InvokeFail(t, "not enough fresh prices", "collateralValue", user.ScriptHash().BytesBE())

	// the debt-free user still withdraws all collateral (no oracle call on this path)
	s.e.NewInvoker(s.lend, user).Invoke(t, stackitem.Null{}, "withdrawCollateral",
		user.ScriptHash().BytesBE(), 10, 0)
	s.neoBal(t, user.ScriptHash(), 10)
}

// FTB-07: when a position is NOT underwater, a single liquidation is capped at half the debt.
// (The sustained dip is settled first; the transient-dump protection is covered by TestVolatility*.)
func TestCloseFactorCapsLiquidation(t *testing.T) {
	s := setup(t)
	s.fastGuards(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	liquidator := s.e.NewAccount(t)

	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	keeper := s.newKeeper(t)

	// borrower posts 100 NEO at price 50 (value 5000) and borrows 2500 (within 60% LTV = 3000)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)

	// price dips 50 -> 30 (sustained): collateral value 3000 (>= debt 2500, so NOT underwater);
	// threshold 75% = 2250, debt 2500 -> unhealthy. Close factor caps one repay at 2500/2 = 1250.
	s.setPrice(t, s.neo, 30*dec)
	s.settle(t, keeper)

	// liquidator offers 2000 FTB, but only 1250 is accepted; the rest is refunded
	s.mintFTB(t, liquidator.ScriptHash(), 2000*dec)
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 2000*dec, []any{"liq", borrower.ScriptHash()})

	lendOwner := s.e.NewInvoker(s.lend, s.owner)
	lendOwner.Invoke(t, stackitem.Make(1250*dec), "debtOf", borrower.ScriptHash().BytesBE()) // 2500 - 1250
	// seize = 1250 * 1.08 = 1350 FTB-value, at price 30 -> exactly 45 whole NEO
	s.neoBal(t, liquidator.ScriptHash(), 45)
	// liquidator refunded 2000 - 1250 = 750 FTB
	s.ftbBal(t, liquidator.ScriptHash(), 750*dec)
}

// Two suppliers get shares in proportion; with a zero rate a share is worth one FTB.
func TestMultipleSuppliersShareProportionally(t *testing.T) {
	s := setup(t)
	a := s.e.NewAccount(t)
	b := s.e.NewAccount(t)
	s.mintFTB(t, a.ScriptHash(), 1000*dec)
	s.mintFTB(t, b.ScriptHash(), 3000*dec)
	lo := s.e.NewInvoker(s.lend, s.owner)

	s.supply(t, a, 1000*dec)
	lo.Invoke(t, stackitem.Make(1000*dec), "sharesOf", a.ScriptHash().BytesBE())

	s.supply(t, b, 3000*dec)
	lo.Invoke(t, stackitem.Make(3000*dec), "sharesOf", b.ScriptHash().BytesBE())
	lo.Invoke(t, stackitem.Make(4000*dec), "totalShares")

	// a withdraws all its shares and gets its 1000 FTB back
	s.e.NewInvoker(s.lend, a).Invoke(t, stackitem.Null{}, "withdrawSupply", a.ScriptHash().BytesBE(), 1000*dec)
	s.ftbBal(t, a.ScriptHash(), 1000*dec)
	lo.Invoke(t, stackitem.Make(0), "sharesOf", a.ScriptHash().BytesBE())
}

// A supplier cannot withdraw more than the cash left after funds are lent out.
func TestWithdrawSupplyUtilizationLock(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 2000*dec)
	s.supply(t, supplier, 2000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)

	ls := s.e.NewInvoker(s.lend, supplier)
	ls.InvokeFail(t, "not enough cash", "withdrawSupply", supplier.ScriptHash().BytesBE(), 2000*dec)
	ls.Invoke(t, stackitem.Null{}, "withdrawSupply", supplier.ScriptHash().BytesBE(), 1000*dec)
	s.ftbBal(t, supplier.ScriptHash(), 1000*dec)
}

// Withdrawing collateral that would push the borrower past the LTV is rejected; a safe amount is allowed.
func TestWithdrawCollateralHealthCheck(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)
	// 50 NEO out would leave value 2500, max borrow 1500 < debt 2500 -> rejected
	lb.InvokeFail(t, "undercollateralized", "withdrawCollateral", borrower.ScriptHash().BytesBE(), 50, 0)
	// 10 NEO out leaves value 4500, max borrow 2700 >= debt 2500 -> allowed
	lb.Invoke(t, stackitem.Null{}, "withdrawCollateral", borrower.ScriptHash().BytesBE(), 10, 0)
	s.neoBal(t, borrower.ScriptHash(), 10)
}

// With a non-zero base rate, debt grows over blocks: accrue books interest with no mint.
func TestInterestAccrues(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setRates", 1_000_000_000_000, 0, 0, 8000, 3333)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
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

// The 1/3 holders' share accrues to the reserve and sweeps out, callable by anyone; the
// reserve is excluded from suppliers' share value. Unbound dividends fails closed; a bound
// treasury takes 1/3 of the reserve (1/9 of interest), leaving 2/3 (2/9) to dividends.
func TestReserveSplitAndSweep(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	dividends := s.e.NewAccount(t)

	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setRates", 1_000_000_000_000, 0, 0, 8000, 3333)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)
	for i := 0; i < 50; i++ {
		s.e.AddNewBlock(t)
	}
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1*dec) // accrue

	lo := s.e.NewInvoker(s.lend, s.owner)
	// reserve = floor(interest * 3333 / 10000), interest = totalBorrows - principal
	lo.InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		reserve := stack[0].Value().(*big.Int)
		requirePositive(t, "reserve", reserve)
		tb := readBig(t, s, "totalBorrows")
		interest := new(big.Int).Sub(tb, big.NewInt(1001*dec))
		// floor(interest*3333/10000) == reserve  <=>  reserve*10000 <= interest*3333 < (reserve+1)*10000
		lhs := new(big.Int).Mul(reserve, big.NewInt(10000))
		rhs := new(big.Int).Mul(interest, big.NewInt(3333))
		rhsHi := new(big.Int).Mul(new(big.Int).Add(reserve, big.NewInt(1)), big.NewInt(10000))
		if lhs.Cmp(rhs) > 0 || rhs.Cmp(rhsHi) >= 0 {
			t.Fatalf("reserve split wrong: reserve %s, interest %s", reserve, interest)
		}
	}, "reserveAccrued")

	// sweep before the dividends module is bound fails closed
	lo.InvokeFail(t, "dividends module not bound", "sweepDividends")

	// bind it, sweep from a random caller, and the reserve lands there exactly
	// (no treasury bound yet: the holders receive the whole reserve)
	s.e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "bootstrap", "dividends", dividends.ScriptHash().BytesBE())
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "sweepDividends")
	lo.Invoke(t, stackitem.Make(0), "reserveAccrued")
	first := dividendsBalance(t, s, dividends.ScriptHash())
	if first.Sign() <= 0 {
		t.Fatalf("dividends module received nothing")
	}

	// with the treasury bound the sweep splits: 1/3 of the reserve to the treasury
	// (1/9 of total interest), 2/3 to dividends (2/9)
	treasury := s.e.NewAccount(t)
	s.e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "bootstrap", "treasury", treasury.ScriptHash().BytesBE())
	// advance past the D-2 sweep cooldown (240 blocks) so the second sweep is allowed, and
	// let interest accrue over the gap
	s.e.GenerateNewBlocks(t, 260)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1*dec) // accrue again
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "sweepDividends")

	treasuryGot := dividendsBalance(t, s, treasury.ScriptHash())
	dividendsGot := new(big.Int).Sub(dividendsBalance(t, s, dividends.ScriptHash()), first)
	// treasuryGot == floor(reserve/3)  <=>  3*treasuryGot <= dividendsGot+treasuryGot < 3*(treasuryGot+1)
	total := new(big.Int).Add(treasuryGot, dividendsGot)
	if new(big.Int).Mul(treasuryGot, big.NewInt(3)).Cmp(total) > 0 {
		t.Fatalf("treasury received more than a third: %s of %s", treasuryGot, total)
	}
	if total.Cmp(new(big.Int).Mul(new(big.Int).Add(treasuryGot, big.NewInt(1)), big.NewInt(3))) >= 0 {
		t.Fatalf("treasury received less than floor(reserve/3): %s of %s", treasuryGot, total)
	}
	lo.Invoke(t, stackitem.Make(0), "reserveAccrued")
}

// C2 remediation: the bytecode upgrade is two-step and self-timelocked. This exercises the
// gating (owner-only, nothing-pending, timelock-not-elapsed, cancel) without landing a real
// NEF — a bogus NEF would fault inside management.update, which is not what we assert here.
func TestUpgradeTimelock(t *testing.T) {
	s := setup(t)
	lo := s.e.NewInvoker(s.lend, s.owner)
	other := s.e.NewAccount(t)
	nef, man := []byte{0x01}, []byte{0x02}

	// non-owner cannot propose
	s.e.NewInvoker(s.lend, other).InvokeFail(t, "only owner", "proposeUpgrade", nef, man)
	// nothing pending yet
	lo.InvokeFail(t, "nothing pending", "applyUpgrade")
	// propose; an ETA upgradeDelay blocks out is stamped
	lo.Invoke(t, stackitem.Null{}, "proposeUpgrade", nef, man)
	lo.InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		requirePositive(t, "pendingUpgradeEta", stack[0].Value().(*big.Int))
	}, "pendingUpgradeEta")
	// cannot apply before the timelock elapses
	lo.InvokeFail(t, "timelock not elapsed", "applyUpgrade")
	// owner cancels; nothing pending again
	lo.Invoke(t, stackitem.Null{}, "cancelUpgrade")
	lo.Invoke(t, stackitem.Make(0), "pendingUpgradeEta")
	lo.InvokeFail(t, "nothing pending", "applyUpgrade")
}

// APY views: utilization and both APYs are pure functions of pool state.
func TestApyViews(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	base := int64(1_000_000_000_000) // 1e12 per block
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setRates", base, 0, 0, 8000, 3333)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)

	lo := s.e.NewInvoker(s.lend, s.owner)
	lo.Invoke(t, stackitem.Make(2500), "utilizationBps") // 1000 / (3000 + 1000)

	// borrowApyBps = rate * blocksPerYear * 1e4 / 1e18
	blocksPerYear := big.NewInt(2_102_400)
	scaleI := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	wantBorrow := new(big.Int).Mul(big.NewInt(base), blocksPerYear)
	wantBorrow.Mul(wantBorrow, big.NewInt(10000))
	wantBorrow.Div(wantBorrow, scaleI)
	lo.Invoke(t, stackitem.Make(wantBorrow), "borrowApyBps")

	// supplyRate = rate * u * (1 - reserveBps); supplyApy likewise
	supplyRate := new(big.Int).Mul(big.NewInt(base), big.NewInt(2500))
	supplyRate.Div(supplyRate, big.NewInt(10000))
	supplyRate.Mul(supplyRate, big.NewInt(6667))
	supplyRate.Div(supplyRate, big.NewInt(10000))
	wantSupply := new(big.Int).Mul(supplyRate, blocksPerYear)
	wantSupply.Mul(wantSupply, big.NewInt(10000))
	wantSupply.Div(wantSupply, scaleI)
	lo.Invoke(t, stackitem.Make(wantSupply), "supplyApyBps")

	// suppliers always earn strictly less than borrowers pay
	if wantSupply.Cmp(wantBorrow) >= 0 {
		t.Fatalf("supply APY must be below borrow APY")
	}
}

func readBig(t testing.TB, s *sys, method string) *big.Int {
	var out *big.Int
	s.e.NewInvoker(s.lend, s.owner).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		out = stack[0].Value().(*big.Int)
	}, method)
	return out
}

func dividendsBalance(t *testing.T, s *sys, who util.Uint160) *big.Int {
	var out *big.Int
	s.e.NewInvoker(s.ftb, s.owner).InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		out = stack[0].Value().(*big.Int)
	}, "balanceOf", who.BytesBE())
	return out
}

func requirePositive(t testing.TB, name string, v *big.Int) {
	t.Helper()
	if v.Sign() <= 0 {
		t.Fatalf("%s must be positive, got %s", name, v)
	}
}

// End-to-end money path: lend interest accrues, the sweep splits the reserve, and a
// staked FTB holder claims their dividend from the REAL dividends module — the fee
// constitution executed by the contracts, not asserted in a comment.
func TestSweepIntoRealDividendsModule(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	holder := s.e.NewAccount(t)

	// deploy the real dividends module and bind it in the registry
	divC := neotest.CompileFile(t, s.e.Validator.ScriptHash(), "../dividends/dividends.go", "../dividends/dividends.yml")
	s.e.DeployContract(t, divC, s.owner.ScriptHash().BytesBE())
	s.e.NewInvoker(divC.Hash, s.owner).Invoke(t, stackitem.Null{}, "configure", s.ftb.BytesBE())
	s.e.NewInvoker(s.reg, s.owner).Invoke(t, stackitem.Null{}, "bootstrap", "dividends", divC.Hash.BytesBE())

	// the holder stakes FTB into the dividends module (the xSUSHI door)
	s.mintFTB(t, holder.ScriptHash(), 100*dec)
	s.e.NewInvoker(s.ftb, holder).Invoke(t, stackitem.Make(true), "transfer",
		holder.ScriptHash().BytesBE(), divC.Hash.BytesBE(), 100*dec, []any{"stake"})

	// the pool earns interest
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setRates", 1_000_000_000_000, 0, 0, 8000, 3333)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec)
	for i := 0; i < 50; i++ {
		s.e.AddNewBlock(t)
	}
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1*dec) // accrue

	// sweep: no treasury bound, so the whole reserve lands in the dividends module
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "sweepDividends")

	// the sole staker is owed the whole reserve and claims it
	dv := s.e.NewInvoker(divC.Hash, s.owner)
	var pending *big.Int
	dv.InvokeAndCheck(t, func(t testing.TB, stack []stackitem.Item) {
		pending = stack[0].Value().(*big.Int)
	}, "pendingOf", holder.ScriptHash().BytesBE())
	requirePositive(t, "holder pending dividend", pending)
	before := dividendsBalance(t, s, holder.ScriptHash())
	s.e.NewInvoker(divC.Hash, holder).Invoke(t, stackitem.Null{}, "claim", holder.ScriptHash().BytesBE())
	after := dividendsBalance(t, s, holder.ScriptHash())
	if new(big.Int).Sub(after, before).Cmp(pending) != 0 {
		t.Fatalf("claim paid %s, pending was %s", new(big.Int).Sub(after, before), pending)
	}
}

// ---- volatility-protective liquidation model (claude/ftb-liquidation-model-study.md) ----

// A sharp upward move (a pump) past the breaker pauses NEW borrows while the reference lags, so an
// inflate-then-borrow attack cannot size a loan against a transiently-inflated collateral price.
func TestVolatilityPumpBreakerPausesBorrow(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 1000*dec) // seeds the reference at 50

	// NEO spot pumps 2x; with the default (slow) guards the reference lags far below, so the upside
	// breaker fires and borrows are paused
	s.setPrice(t, s.neo, 100*dec)
	lb.InvokeFail(t, "borrows paused", "borrow", borrower.ScriptHash().BytesBE(), 1*dec)
}

// A smaller pump, inside the breaker band, does NOT pause borrows — but sizing still uses the
// lagged reference, not the pumped spot, so extra borrow that only the spot price would allow is
// still rejected on LTV. This is the softer half of the pump protection.
func TestVolatilityPumpLagBluntsBorrow(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	// borrow near the cap at the seeded price (ref 50 -> max borrow 100*50*0.6 = 3000)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2900*dec)

	// NEO pumps 50 -> 60 (+20%, inside the 25% breaker). Sizing still uses min(spot, ref) ~ 50, so
	// a further 200 (which only the spot price of 60 would allow) exceeds the reference-based LTV.
	s.setPrice(t, s.neo, 60*dec)
	lb.InvokeFail(t, "exceeds LTV", "borrow", borrower.ScriptHash().BytesBE(), 200*dec)
}

// A sharp downward move (a dump): the max-basis valuation keeps a healthy position from reading
// liquidatable, AND the downside breaker hard-pauses liquidations — so a flash-dump cannot
// mass-liquidate users. Once the move is sustained and the reference catches down, protection
// lifts and the liquidation proceeds normally.
func TestVolatilityDumpBreakerAndRecovery(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	liquidator := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	keeper := s.newKeeper(t)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lo := s.e.NewInvoker(s.lend, s.owner)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec) // seeds the reference at 50

	// NEO dumps 50 -> 30 with the default (slow) guards: the reference lags, so the max-basis health
	// view still reads healthy and the downside breaker pauses liquidations
	s.setPrice(t, s.neo, 30*dec)
	lo.Invoke(t, stackitem.Make(false), "isUnhealthy", borrower.ScriptHash().BytesBE())
	s.mintFTB(t, liquidator.ScriptHash(), 2000*dec)
	s.e.NewInvoker(s.ftb, liquidator).InvokeFail(t, "liquidations paused", "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"liq", borrower.ScriptHash()})

	// sustain the move: the reference catches down to 30, protection lifts, liquidation proceeds
	s.fastGuards(t)
	s.settle(t, keeper)
	lo.Invoke(t, stackitem.Make(true), "isUnhealthy", borrower.ScriptHash().BytesBE())
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"liq", borrower.ScriptHash()})
	lo.Invoke(t, stackitem.Make(1), "liquidationsOf", borrower.ScriptHash().BytesBE())
}

// SetGuards is owner-only and bounds both knobs so a mis-set can neither disable the protection
// (maxRefStep 1..1000) nor freeze the market permanently (breaker 500..9000 bps).
func TestSetGuardsBounds(t *testing.T) {
	s := setup(t)
	lo := s.e.NewInvoker(s.lend, s.owner)
	lo.InvokeFail(t, "bad guard params", "setGuards", 0, 2500)
	lo.InvokeFail(t, "bad guard params", "setGuards", 1001, 2500)
	lo.InvokeFail(t, "bad guard params", "setGuards", 3, 499)
	lo.InvokeFail(t, "bad guard params", "setGuards", 3, 9001)
	lo.Invoke(t, stackitem.Null{}, "setGuards", 1, 500)     // lower edges accepted
	lo.Invoke(t, stackitem.Null{}, "setGuards", 1000, 9000) // upper edges accepted
	other := s.e.NewAccount(t)
	s.e.NewInvoker(s.lend, other).InvokeFail(t, "only owner", "setGuards", 3, 2500)
}

// HealthFactor returns a large sentinel when debt-free, and the liquidation-basis collateral value
// times the liquidation threshold as a bps multiple of debt otherwise (>= 10000 is healthy).
func TestHealthFactor(t *testing.T) {
	s := setup(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	s.mintFTB(t, supplier.ScriptHash(), 4000*dec)
	s.supply(t, supplier, 4000*dec)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lo := s.e.NewInvoker(s.lend, s.owner)

	// debt-free -> sentinel (returns before any oracle read)
	lo.Invoke(t, stackitem.Make(2147483647), "healthFactor", borrower.ScriptHash().BytesBE())

	// borrow 2500 at price 50: HF = (100*50) * 7500 / 2500 = 15000 bps (1.5x)
	s.e.NewInvoker(s.lend, borrower).Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 2500*dec)
	lo.Invoke(t, stackitem.Make(15000), "healthFactor", borrower.ScriptHash().BytesBE())
}

// O4: when a liquidation exhausts a borrower's collateral and debt remains, that residual is
// written off immediately — charged to the holders' reserve first (their first-loss buffer), the
// rest socialized to suppliers — so no uncollectible debt lingers accruing in the pool.
func TestO4BadDebtWriteOff(t *testing.T) {
	s := setup(t)
	s.fastGuards(t)
	supplier := s.e.NewAccount(t)
	borrower := s.e.NewAccount(t)
	liquidator := s.e.NewAccount(t)
	// a real rate so interest accrues into the holders' reserve (the first-loss buffer O4 taps first)
	s.e.NewInvoker(s.lend, s.owner).Invoke(t, stackitem.Null{}, "setRates", 10_000_000_000_000, 0, 0, 8000, 3333)
	s.mintFTB(t, supplier.ScriptHash(), 10000*dec)
	s.supply(t, supplier, 10000*dec)
	keeper := s.newKeeper(t)
	s.fundNeo(t, borrower.ScriptHash(), 100)
	s.postNeo(t, borrower, 100)
	lb := s.e.NewInvoker(s.lend, borrower)
	lo := s.e.NewInvoker(s.lend, s.owner)
	lb.Invoke(t, stackitem.Null{}, "borrow", borrower.ScriptHash().BytesBE(), 3000*dec) // max LTV at price 50

	// accrue interest so the reserve is non-zero. The borrower sits at the LTV cap, so trigger
	// accrual from the keeper (which has ample headroom) rather than the borrower.
	s.e.GenerateNewBlocks(t, 100)
	s.e.NewInvoker(s.lend, keeper).Invoke(t, stackitem.Null{}, "borrow", keeper.ScriptHash().BytesBE(), 1)
	reserveBefore := readBig(t, s, "reserveAccrued")
	requirePositive(t, "reserveAccrued before write-off", reserveBefore)

	// NEO collapses 50 -> 10: 100 NEO now worth 1000, far below the ~3000 debt (deeply underwater)
	s.setPrice(t, s.neo, 10*dec)
	s.settle(t, keeper)
	lo.Invoke(t, stackitem.Make(true), "isUnhealthy", borrower.ScriptHash().BytesBE())

	// liquidator repays 1000: seizeFTB 1080 > NEO value 1000 -> all 100 NEO seized, collateral
	// exhausted, and the residual debt is written off (reserve first)
	s.mintFTB(t, liquidator.ScriptHash(), 1000*dec)
	s.e.NewInvoker(s.ftb, liquidator).Invoke(t, stackitem.Make(true), "transfer",
		liquidator.ScriptHash().BytesBE(), s.lend.BytesBE(), 1000*dec, []any{"liq", borrower.ScriptHash()})

	s.neoBal(t, liquidator.ScriptHash(), 100)                                              // seized every NEO
	lo.Invoke(t, stackitem.Make(0), "collateralOf", borrower.ScriptHash().BytesBE(), true) // collateral exhausted
	lo.Invoke(t, stackitem.Make(0), "debtOf", borrower.ScriptHash().BytesBE())             // residual written off
	reserveAfter := readBig(t, s, "reserveAccrued")
	if reserveAfter.Cmp(reserveBefore) >= 0 {
		t.Fatalf("reserve must be drawn down first for bad debt: before %s, after %s", reserveBefore, reserveAfter)
	}
}
