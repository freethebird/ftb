// Package lend is the FTB lending market: NO MINT. Suppliers deposit FTB and earn
// interest; borrowers post NEO or GAS collateral — the two native assets, no wrapper, no
// vendor — and borrow FTB from what suppliers put in. Like a bank, it lends only deposits
// and never prints. Supply uses cToken-style shares so interest accrues into every
// share's value; debt accrues through a borrow index.
//
// Economics (ECONOMICS.md): the borrow rate follows utilization along a kinked curve,
// and interest splits 2/3 to suppliers (share value) and 1/3 to the dividend reserve for
// FTB holders. The reserve is swept to the "dividends" module bound in the registry —
// fail-closed until it is bound. NEO is indivisible (0 decimals): it is priced per whole
// NEO, and liquidation seizes floor-divided whole NEO, leaving any dust value with the
// borrower.
//
// The oracle is resolved through the registry each call, so it can be upgraded behind the
// registry timelock without touching this contract (the seam).
//
// HIGHEST-RISK MODULE. Launch 2 only, behind its own dedicated audit. All FTB/GAS amounts
// are base units (8 decimals); NEO amounts are whole units. Prices are FTB base units per
// 1 whole asset, so value = amount * price / 1e8 for 8-decimal assets and
// value = wholeNeo * price for NEO.
//
// Reference implementation pending audit.
package lend

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/gas"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/neo"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const (
	dec     = 100_000_000               // 1e8, decimals of FTB/GAS
	scale   = 1_000_000_000_000_000_000 // 1e18 fixed-point for rates and the borrow index
	bps     = 10000
	maxRate = scale / 100 // per-block rate ceiling (FTB-07): 1% per block is far above any real
	// rate; it only bounds gross misconfiguration and keeps accrual products inside 256 bits.
	blocksPerYear = 2_102_400 // 15-second target block time; linear annualization
	maxReserveBps = 5000      // holders' fee share can never exceed half, even by vote
)

var (
	keyOwner      = []byte{0x01}
	keyRegistry   = []byte{0x02}
	keyFTB        = []byte{0x03}
	keyLTV        = []byte{0x05}
	keyLiq        = []byte{0x06}
	keyBonus      = []byte{0x07}
	keyBase       = []byte{0x08} // base borrow rate per block (1e18 scale)
	keySlope1     = []byte{0x09} // slope below the kink
	keySlope2     = []byte{0x0A} // slope above the kink
	keyKink       = []byte{0x0B} // kink, in bps of utilization
	keyResBps     = []byte{0x0C} // holders' share of interest (the 1/3)
	keyResAccrued = []byte{0x0D} // FTB owed to the dividends module

	keyTotShares = []byte{0x20}
	keyTotBorrow = []byte{0x21}
	keyBIdx      = []byte{0x22}
	keyLastBlk   = []byte{0x23}

	prefixShares = []byte{0x30}
	prefixBNorm  = []byte{0x31}
	prefixColN   = []byte{0x32} // NEO collateral (whole units)
	prefixColG   = []byte{0x33} // GAS collateral (base units)
)

func _deploy(data any, isUpdate bool) {
	if isUpdate {
		return
	}
	owner := data.(interop.Hash160)
	if len(owner) != 20 {
		panic("deploy: owner must be a 20-byte address")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyOwner, owner)
	storage.Put(ctx, keyBIdx, scale)
	storage.Put(ctx, keyLastBlk, ledger.CurrentIndex())
	storage.Put(ctx, keyLTV, 6000)    // 60% max borrow
	storage.Put(ctx, keyLiq, 7500)    // 75% liquidation threshold
	storage.Put(ctx, keyBonus, 800)   // 8% liquidation bonus
	storage.Put(ctx, keyKink, 8000)   // the curve steepens past 80% utilization
	storage.Put(ctx, keyResBps, 3333) // 1/3 of interest to FTB holders (ECONOMICS.md)
	// rates ship at zero: set a real curve with SetRates before launch
}

// ---- admin (owner = multisig) ----

func Configure(registry, ftb interop.Hash160) {
	requireOwner()
	if len(registry) != 20 || len(ftb) != 20 {
		panic("configure: bad hash")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyRegistry, registry)
	storage.Put(ctx, keyFTB, ftb)
	runtime.Notify("Configured", registry, ftb) // FTB-09: wiring changes must be observable
}

// SetRisk sets the collateral-risk knobs. The bonus is capped at 30% so a mis-set or
// malicious value cannot seize a multiple of the repaid debt (FTB-07).
func SetRisk(ltv, liq, bonus int) {
	requireOwner()
	if ltv <= 0 || ltv >= liq || liq >= bps || bonus < 0 || bonus > 3000 {
		panic("bad risk params")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyLTV, ltv)
	storage.Put(ctx, keyLiq, liq)
	storage.Put(ctx, keyBonus, bonus)
	runtime.Notify("RiskSet", ltv, liq, bonus) // FTB-09: risk changes must be observable
}

// SetRates sets the utilization curve and the fee split. The peak rate (at 100%
// utilization) is capped far above any real rate; the holders' share is capped at 50%
// so no vote can starve the pool that pays it.
func SetRates(base, slope1, slope2, kink, reserveBps int) {
	requireOwner()
	if base < 0 || slope1 < 0 || slope2 < 0 || kink <= 0 || kink >= bps ||
		reserveBps < 0 || reserveBps > maxReserveBps {
		panic("bad rate params")
	}
	peak := base + kink*slope1/bps + (bps-kink)*slope2/bps
	if peak > maxRate {
		panic("rate curve exceeds the ceiling")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyBase, base)
	storage.Put(ctx, keySlope1, slope1)
	storage.Put(ctx, keySlope2, slope2)
	storage.Put(ctx, keyKink, kink)
	storage.Put(ctx, keyResBps, reserveBps)
	runtime.Notify("RatesSet", base, slope1, slope2, kink, reserveBps)
}

// ---- receive tokens ----

func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	accrue()
	token := runtime.GetCallingScriptHash()
	if from == nil {
		// A null sender means a mint, and the only mint that can land here is the GAS
		// claim distributed whenever the contract's NEO balance changes (deposits,
		// withdrawals, liquidations). It is not collateral and not a repayment; the
		// amounts are dust and belong to no user. Accept it into the contract's
		// unaccounted GAS balance — rejecting it would fault every NEO transfer out
		// and brick withdrawals and liquidations (a protocol-level DoS).
		if !eq(token, interop.Hash160(gas.Hash)) {
			panic("unexpected mint")
		}
		return
	}
	if eq(token, ftbHash()) {
		arr := data.([]any)
		action := arr[0].(string)
		if action == "supply" {
			supplyIn(from, amount)
		} else if action == "repay" {
			repayIn(from, amount)
		} else if action == "liq" {
			liquidateIn(from, amount, arr[1].(interop.Hash160))
		} else {
			panic("bad action")
		}
	} else if eq(token, interop.Hash160(neo.Hash)) {
		addCollateral(from, amount, true)
	} else if eq(token, interop.Hash160(gas.Hash)) {
		addCollateral(from, amount, false)
	} else {
		panic("token not accepted")
	}
}

// ---- supply side (cToken shares) ----

func supplyIn(user interop.Hash160, amount int) {
	ctx := storage.GetContext()
	ts := getIntCtx(ctx, keyTotShares)
	var shares int
	if ts == 0 {
		shares = amount
	} else {
		pre := poolValue() - amount // cash() already counts the just-received amount
		if pre <= 0 {
			panic("supply: pool insolvent") // FTB-08: no divide-by-zero / nonsense pricing
		}
		shares = amount * ts / pre
	}
	if shares <= 0 {
		panic("supply: amount too small at current share price") // FTB-08: no silent zero-share loss
	}
	storage.Put(ctx, shareKey(user), getIntCtx(ctx, shareKey(user))+shares)
	storage.Put(ctx, keyTotShares, ts+shares)
}

func WithdrawSupply(user interop.Hash160, shares int) {
	if !runtime.CheckWitness(user) {
		panic("no witness")
	}
	accrue()
	ctx := storage.GetContext()
	s := getIntCtx(ctx, shareKey(user))
	if shares <= 0 || shares > s {
		panic("bad shares")
	}
	c := cashAvailable() // one read, reused for the share price and the liquidity check
	amount := shares * (c + getIntCtx(ctx, keyTotBorrow)) / getIntCtx(ctx, keyTotShares)
	if amount > c {
		panic("not enough cash; borrowers must repay first")
	}
	storage.Put(ctx, shareKey(user), s-shares)
	storage.Put(ctx, keyTotShares, getIntCtx(ctx, keyTotShares)-shares)
	send(ftbHash(), user, amount)
}

// ---- collateral ----

func addCollateral(user interop.Hash160, amount int, isNeo bool) {
	ctx := storage.GetContext()
	k := colKey(user, isNeo)
	storage.Put(ctx, k, getIntCtx(ctx, k)+amount)
}

func WithdrawCollateral(user interop.Hash160, neoAmt, gasAmt int) {
	if !runtime.CheckWitness(user) {
		panic("no witness")
	}
	accrue()
	if neoAmt < 0 || gasAmt < 0 {
		panic("negative")
	}
	ctx := storage.GetContext()
	cn := col(user, true) // one storage read each, reused below
	cg := col(user, false)
	if neoAmt > cn || gasAmt > cg {
		panic("too much")
	}
	storage.Put(ctx, colKey(user, true), cn-neoAmt)
	storage.Put(ctx, colKey(user, false), cg-gasAmt)
	// Debt-free users withdraw without touching the oracle, so a paused oracle never traps
	// their own collateral. Audit item FTB-10.
	if d := DebtOf(user); d > 0 {
		// oracle resolved once per transaction, not once per price read
		if d > collateralValueFrom(oracleHash(), user)*getInt(keyLTV)/bps {
			panic("would be undercollateralized")
		}
	}
	if neoAmt > 0 {
		send(interop.Hash160(neo.Hash), user, neoAmt)
	}
	if gasAmt > 0 {
		send(interop.Hash160(gas.Hash), user, gasAmt)
	}
}

// ---- borrow / repay ----

func Borrow(user interop.Hash160, amount int) {
	if !runtime.CheckWitness(user) {
		panic("no witness")
	}
	accrue()
	if amount <= 0 {
		panic("amount")
	}
	if amount > cashAvailable() {
		panic("not enough liquidity")
	}
	if DebtOf(user)+amount > collateralValueFrom(oracleHash(), user)*getInt(keyLTV)/bps {
		panic("exceeds LTV")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, bnormKey(user), getIntCtx(ctx, bnormKey(user))+amount*scale/bidx())
	storage.Put(ctx, keyTotBorrow, getIntCtx(ctx, keyTotBorrow)+amount)
	send(ftbHash(), user, amount)
}

func repayIn(user interop.Hash160, amount int) {
	ctx := storage.GetContext()
	debt := DebtOf(user)
	pay := amount
	if pay > debt {
		pay = debt
	}
	storage.Put(ctx, bnormKey(user), getIntCtx(ctx, bnormKey(user))-pay*scale/bidx())
	storage.Put(ctx, keyTotBorrow, getIntCtx(ctx, keyTotBorrow)-pay)
	if amount > pay {
		send(ftbHash(), user, amount-pay) // refund overpay
	}
}

// ---- liquidation ----

func liquidateIn(liquidator interop.Hash160, repay int, borrower interop.Hash160) {
	ctx := storage.GetContext()
	orc := oracleHash() // resolved once per transaction, not once per price read
	cv := collateralValueFrom(orc, borrower)
	debt := DebtOf(borrower)
	if debt <= cv*getInt(keyLiq)/bps {
		panic("borrower healthy")
	}
	// Close factor (audit item FTB-07): a single liquidation repays at most half the debt, so a
	// borrower cannot be fully wiped on a small dip. The exception is an underwater position
	// (collateral worth less than the debt), where full repayment is allowed to clear bad debt.
	maxClose := debt
	if cv >= debt {
		maxClose = debt / 2
	}
	pay := repay
	if pay > maxClose {
		pay = maxClose
	}
	storage.Put(ctx, bnormKey(borrower), getIntCtx(ctx, bnormKey(borrower))-pay*scale/bidx())
	storage.Put(ctx, keyTotBorrow, getIntCtx(ctx, keyTotBorrow)-pay)

	seizeFtb := pay * (bps + getIntCtx(ctx, keyBonus)) / bps // FTB-value of collateral to seize
	cn := col(borrower, true)                                // one storage read each, reused below
	cg := col(borrower, false)
	pN := priceFrom(orc, interop.Hash160(neo.Hash))
	neoVal := cn * pN // NEO is whole-unit: value = wholeNeo * price, no decimal scaling

	var seizeN, seizeG int
	if seizeFtb <= neoVal {
		seizeN = seizeFtb / pN // floor: up to one whole NEO of dust value stays with the borrower
	} else {
		seizeN = cn
		remFtb := seizeFtb - neoVal
		pG := priceFrom(orc, interop.Hash160(gas.Hash)) // only needed when NEO cannot cover the seize
		seizeG = remFtb * dec / pG
		if seizeG > cg {
			seizeG = cg
		}
	}
	storage.Put(ctx, colKey(borrower, true), cn-seizeN)
	storage.Put(ctx, colKey(borrower, false), cg-seizeG)
	if seizeN > 0 {
		send(interop.Hash160(neo.Hash), liquidator, seizeN)
	}
	if seizeG > 0 {
		send(interop.Hash160(gas.Hash), liquidator, seizeG)
	}
	if repay > pay {
		send(ftbHash(), liquidator, repay-pay) // refund overpay
	}
}

// ---- interest accrual (utilization curve + the 2/3 : 1/3 split) ----

func accrue() {
	ctx := storage.GetContext()
	last := getIntCtx(ctx, keyLastBlk)
	now := ledger.CurrentIndex()
	blocks := now - last
	if blocks <= 0 {
		return
	}
	tb := getIntCtx(ctx, keyTotBorrow)
	rate := borrowRatePerBlock()
	if rate > 0 && tb > 0 {
		factor := rate * blocks
		interest := tb * factor / scale
		reserve := interest * getIntCtx(ctx, keyResBps) / bps
		storage.Put(ctx, keyResAccrued, getIntCtx(ctx, keyResAccrued)+reserve)
		storage.Put(ctx, keyTotBorrow, tb+interest)
		bi := getIntCtx(ctx, keyBIdx)
		storage.Put(ctx, keyBIdx, bi+bi*factor/scale)
		// interest minus the reserve raises poolValue, so every supplier share is worth
		// more — the 2/3. The reserve is the holders' 1/3. No mint, either way.
	}
	storage.Put(ctx, keyLastBlk, now)
}

// SweepDividends pays the accrued reserve to the dividends module bound in the registry.
// Callable by anyone: the binding is timelocked and in the open, and the funds can only
// go to that address (fail-closed while unbound).
func SweepDividends() {
	accrue()
	ctx := storage.GetContext()
	reserve := getIntCtx(ctx, keyResAccrued)
	if reserve <= 0 {
		panic("sweep: no reserve")
	}
	div := contract.Call(regHash(), "get", contract.ReadOnly, "dividends")
	if div == nil {
		panic("sweep: dividends module not bound")
	}
	to := div.(interop.Hash160)
	storage.Put(ctx, keyResAccrued, 0)
	send(ftbHash(), to, reserve)
	runtime.Notify("DividendsSwept", to, reserve)
}

// ---- views (safe) ----

func DebtOf(user interop.Hash160) int { return getInt(bnormKey(user)) * bidx() / scale }
func CollateralValue(user interop.Hash160) int {
	return collateralValueFrom(oracleHash(), user)
}

// collateralValueFrom prices both collateral assets against an already-resolved oracle, so
// hot paths pay the registry lookup once per transaction instead of once per price read.
// NEO is whole-unit (0 decimals): value = wholeNeo * price. GAS is 8 decimals: /dec.
func collateralValueFrom(orc, user interop.Hash160) int {
	v := 0
	if n := col(user, true); n > 0 {
		v += n * priceFrom(orc, interop.Hash160(neo.Hash))
	}
	if g := col(user, false); g > 0 {
		v += g * priceFrom(orc, interop.Hash160(gas.Hash)) / dec
	}
	return v
}
func MaxBorrow(user interop.Hash160) int { return CollateralValue(user) * getInt(keyLTV) / bps }
func IsUnhealthy(user interop.Hash160) bool {
	return DebtOf(user) > CollateralValue(user)*getInt(keyLiq)/bps
}
func SharesOf(user interop.Hash160) int { return getInt(shareKey(user)) }
func TotalShares() int                  { return getInt(keyTotShares) }
func TotalBorrows() int                 { return getInt(keyTotBorrow) }

// CollateralOf is the user's posted collateral: whole NEO when isNeo, GAS base units
// otherwise. Bookkeeping only — the contract's raw GAS balance also holds claim dust.
func CollateralOf(user interop.Hash160, isNeo bool) int { return col(user, isNeo) }

// ReserveAccrued is the FTB owed to FTB holders, waiting for the next sweep.
func ReserveAccrued() int { return getInt(keyResAccrued) }

// UtilizationBps: totalBorrows as bps of the lendable pool (available cash + borrows).
func UtilizationBps() int {
	tb := getInt(keyTotBorrow)
	if tb == 0 {
		return 0
	}
	return tb * bps / (cashAvailable() + tb)
}

// BorrowRatePerBlock is the current point on the kinked utilization curve (1e18 scale).
func BorrowRatePerBlock() int { return borrowRatePerBlock() }

// SupplyRatePerBlock: what suppliers earn after the holders' 1/3 is taken out.
func SupplyRatePerBlock() int {
	return borrowRatePerBlock() * UtilizationBps() / bps * (bps - getInt(keyResBps)) / bps
}

// BorrowApyBps / SupplyApyBps annualize linearly at the 15-second target block time.
func BorrowApyBps() int { return borrowRatePerBlock() * blocksPerYear * bps / scale }
func SupplyApyBps() int { return SupplyRatePerBlock() * blocksPerYear * bps / scale }

// ---- helpers ----

func borrowRatePerBlock() int {
	u := UtilizationBps()
	kink := getInt(keyKink)
	if kink == 0 {
		return 0 // curve not configured yet
	}
	base := getInt(keyBase)
	if u <= kink {
		return base + u*getInt(keySlope1)/bps
	}
	return base + kink*getInt(keySlope1)/bps + (u-kink)*getInt(keySlope2)/bps
}

// poolValue is the suppliers' claim: cash + borrows, minus the holders' reserve.
func poolValue() int { return cash() + getInt(keyTotBorrow) - getInt(keyResAccrued) }

// cashAvailable is what can actually leave the contract: the reserve is spoken for.
func cashAvailable() int { return cash() - getInt(keyResAccrued) }

func cash() int {
	return contract.Call(ftbHash(), "balanceOf", contract.ReadOnly, runtime.GetExecutingScriptHash()).(int)
}
func priceFrom(orc, asset interop.Hash160) int {
	return contract.Call(orc, "price", contract.ReadOnly, asset).(int)
}
func oracleHash() interop.Hash160 {
	h := contract.Call(regHash(), "get", contract.ReadOnly, "oracle")
	if h == nil {
		panic("oracle not set in registry")
	}
	return h.(interop.Hash160)
}
func regHash() interop.Hash160 {
	return storage.Get(storage.GetContext(), keyRegistry).(interop.Hash160)
}
func ftbHash() interop.Hash160 { return storage.Get(storage.GetContext(), keyFTB).(interop.Hash160) }
func bidx() int                { return getInt(keyBIdx) }

func send(token, to interop.Hash160, amount int) {
	// FTB-15: check the NEP-17 result. FTB's own transfer faults on real failures; if an
	// asset ever returns false instead of faulting, our storage was already debited and the
	// tokens must provably have moved.
	if !contract.Call(token, "transfer", contract.All, runtime.GetExecutingScriptHash(), to, amount, nil).(bool) {
		panic("send: token transfer returned false")
	}
}
func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil || !runtime.CheckWitness(owner.(interop.Hash160)) {
		panic("only owner")
	}
}
func eq(a, b interop.Hash160) bool      { return string(a) == string(b) }
func shareKey(u interop.Hash160) []byte { return append(prefixShares, u...) }
func bnormKey(u interop.Hash160) []byte { return append(prefixBNorm, u...) }
func colKey(u interop.Hash160, isNeo bool) []byte {
	if isNeo {
		return append(prefixColN, u...)
	}
	return append(prefixColG, u...)
}
func col(u interop.Hash160, isNeo bool) int { return getInt(colKey(u, isNeo)) }
func getInt(key []byte) int                 { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
