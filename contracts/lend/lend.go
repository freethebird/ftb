// Package lend is the FTB lending market: NO MINT. Suppliers deposit FTB and earn
// interest; borrowers post bNEO/GAS collateral and borrow FTB from what suppliers put in.
// Like a bank, it lends only deposits and never prints. Supply uses cToken-style shares so
// interest accrues into every share's value; debt accrues through a borrow index.
//
// The oracle is resolved through the registry each call, so it can be upgraded behind the
// registry timelock without touching this contract (the seam).
//
// HIGHEST-RISK MODULE. Launch 2 only, behind its own dedicated audit. The interest model
// here is simple linear accrual and the liquidation seize is reference-level; both must be
// reviewed and hardened. All token amounts are base units (8 decimals). Prices are FTB
// base units per 1 whole asset, so value = amount * price / 1e8.
//
// Reference implementation pending audit.
package lend

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/gas"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const (
	dec     = 100_000_000               // 1e8, decimals of FTB/bNEO/GAS
	scale   = 1_000_000_000_000_000_000 // 1e18 fixed-point for the borrow index
	bps     = 10000
	maxRate = scale / 100 // per-block rate ceiling (FTB-07): 1% per block is far above any real
	// rate; it only bounds gross misconfiguration and keeps accrual products inside 256 bits.
)

var (
	keyOwner    = []byte{0x01}
	keyRegistry = []byte{0x02}
	keyFTB      = []byte{0x03}
	keyBNEO     = []byte{0x04}
	keyLTV      = []byte{0x05}
	keyLiq      = []byte{0x06}
	keyBonus    = []byte{0x07}
	keyRate     = []byte{0x08}

	keyTotShares = []byte{0x20}
	keyTotBorrow = []byte{0x21}
	keyBIdx      = []byte{0x22}
	keyLastBlk   = []byte{0x23}

	prefixShares = []byte{0x30}
	prefixBNorm  = []byte{0x31}
	prefixColB   = []byte{0x32} // bNEO collateral
	prefixColG   = []byte{0x33} // GAS collateral
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
	storage.Put(ctx, keyLTV, 6000)  // 60% max borrow
	storage.Put(ctx, keyLiq, 7500)  // 75% liquidation threshold
	storage.Put(ctx, keyBonus, 800) // 8% liquidation bonus
	storage.Put(ctx, keyRate, 0)    // set a real per-block rate before launch
}

// ---- admin (owner = multisig) ----

func Configure(registry, ftb, bneo interop.Hash160) {
	requireOwner()
	if len(registry) != 20 || len(ftb) != 20 || len(bneo) != 20 {
		panic("configure: bad hash")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyRegistry, registry)
	storage.Put(ctx, keyFTB, ftb)
	storage.Put(ctx, keyBNEO, bneo)
	runtime.Notify("Configured", registry, ftb, bneo) // FTB-09: wiring changes must be observable
}

func SetParams(ltv, liq, bonus, ratePerBlock int) {
	requireOwner()
	// bonus capped at 30% so a mis-set or malicious value cannot seize a multiple of the
	// repaid debt. Audit item FTB-07. (A ratePerBlock ceiling is a deploy-time policy choice.)
	if ltv <= 0 || ltv >= liq || liq >= bps || bonus < 0 || bonus > 3000 || ratePerBlock < 0 || ratePerBlock > maxRate {
		panic("bad params")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyLTV, ltv)
	storage.Put(ctx, keyLiq, liq)
	storage.Put(ctx, keyBonus, bonus)
	storage.Put(ctx, keyRate, ratePerBlock)
	runtime.Notify("ParamsSet", ltv, liq, bonus, ratePerBlock) // FTB-09: risk changes must be observable
}

// ---- receive tokens ----

func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	accrue()
	token := runtime.GetCallingScriptHash()
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
	} else if eq(token, bneoHash()) {
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
	c := cash() // one cross-contract read, reused for both the share price and the liquidity check
	amount := shares * (c + getIntCtx(ctx, keyTotBorrow)) / getIntCtx(ctx, keyTotShares)
	if amount > c {
		panic("not enough cash; borrowers must repay first")
	}
	storage.Put(ctx, shareKey(user), s-shares)
	storage.Put(ctx, keyTotShares, getIntCtx(ctx, keyTotShares)-shares)
	send(ftbHash(), user, amount)
}

// ---- collateral ----

func addCollateral(user interop.Hash160, amount int, isBneo bool) {
	ctx := storage.GetContext()
	k := colKey(user, isBneo)
	storage.Put(ctx, k, getIntCtx(ctx, k)+amount)
}

func WithdrawCollateral(user interop.Hash160, bneo, gasAmt int) {
	if !runtime.CheckWitness(user) {
		panic("no witness")
	}
	accrue()
	if bneo < 0 || gasAmt < 0 {
		panic("negative")
	}
	ctx := storage.GetContext()
	cb := col(user, true) // one storage read each, reused below
	cg := col(user, false)
	if bneo > cb || gasAmt > cg {
		panic("too much")
	}
	storage.Put(ctx, colKey(user, true), cb-bneo)
	storage.Put(ctx, colKey(user, false), cg-gasAmt)
	// Debt-free users withdraw without touching the oracle, so a paused oracle never traps
	// their own collateral. Audit item FTB-10.
	if d := DebtOf(user); d > 0 {
		// oracle resolved once per transaction, not once per price read
		if d > collateralValueFrom(oracleHash(), user)*getInt(keyLTV)/bps {
			panic("would be undercollateralized")
		}
	}
	if bneo > 0 {
		send(bneoHash(), user, bneo)
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
	if amount > cash() {
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
	cb := col(borrower, true)                                // one storage read each, reused below
	cg := col(borrower, false)
	pB := priceFrom(orc, bneoHash())
	bneoVal := cb * pB / dec

	var seizeB, seizeG int
	if seizeFtb <= bneoVal {
		seizeB = seizeFtb * dec / pB
	} else {
		seizeB = cb
		remFtb := seizeFtb - bneoVal
		pG := priceFrom(orc, interop.Hash160(gas.Hash)) // only needed when bNEO cannot cover the seize
		seizeG = remFtb * dec / pG
		if seizeG > cg {
			seizeG = cg
		}
	}
	storage.Put(ctx, colKey(borrower, true), cb-seizeB)
	storage.Put(ctx, colKey(borrower, false), cg-seizeG)
	if seizeB > 0 {
		send(bneoHash(), liquidator, seizeB)
	}
	if seizeG > 0 {
		send(interop.Hash160(gas.Hash), liquidator, seizeG)
	}
	if repay > pay {
		send(ftbHash(), liquidator, repay-pay) // refund overpay
	}
}

// ---- interest accrual ----

func accrue() {
	ctx := storage.GetContext()
	last := getIntCtx(ctx, keyLastBlk)
	now := ledger.CurrentIndex()
	blocks := now - last
	if blocks <= 0 {
		return
	}
	rate := getIntCtx(ctx, keyRate)
	tb := getIntCtx(ctx, keyTotBorrow)
	if rate > 0 && tb > 0 {
		factor := rate * blocks
		interest := tb * factor / scale
		storage.Put(ctx, keyTotBorrow, tb+interest)
		bi := getIntCtx(ctx, keyBIdx)
		storage.Put(ctx, keyBIdx, bi+bi*factor/scale)
		// interest raised poolValue, so every supplier share is worth more. No mint.
	}
	storage.Put(ctx, keyLastBlk, now)
}

// ---- views (safe) ----

func DebtOf(user interop.Hash160) int { return getInt(bnormKey(user)) * bidx() / scale }
func CollateralValue(user interop.Hash160) int {
	return collateralValueFrom(oracleHash(), user)
}

// collateralValueFrom prices both collateral assets against an already-resolved oracle, so
// hot paths pay the registry lookup once per transaction instead of once per price read.
func collateralValueFrom(orc, user interop.Hash160) int {
	v := 0
	if b := col(user, true); b > 0 {
		v += b * priceFrom(orc, bneoHash()) / dec
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

// ---- helpers ----

func poolValue() int { return cash() + getInt(keyTotBorrow) }
func cash() int {
	return contract.Call(ftbHash(), "balanceOf", contract.ReadOnly, runtime.GetExecutingScriptHash()).(int)
}
func price(asset interop.Hash160) int {
	return priceFrom(oracleHash(), asset)
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
func ftbHash() interop.Hash160  { return storage.Get(storage.GetContext(), keyFTB).(interop.Hash160) }
func bneoHash() interop.Hash160 { return storage.Get(storage.GetContext(), keyBNEO).(interop.Hash160) }
func bidx() int                 { return getInt(keyBIdx) }

func send(token, to interop.Hash160, amount int) {
	// FTB-15: check the NEP-17 result. FTB's own transfer faults on real failures, but
	// bNEO is an external contract; if it ever returns false instead of faulting, our
	// storage was already debited and the tokens must provably have moved.
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
func colKey(u interop.Hash160, bneo bool) []byte {
	if bneo {
		return append(prefixColB, u...)
	}
	return append(prefixColG, u...)
}
func col(u interop.Hash160, bneo bool) int { return getInt(colKey(u, bneo)) }
func getInt(key []byte) int                { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
