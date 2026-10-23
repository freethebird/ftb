// Package lend is the FTB lending market: NO MINT. Suppliers deposit FTB and earn
// interest; borrowers post NEO or GAS collateral — the two native assets, no wrapper, no
// vendor — and borrow FTB from what suppliers put in. Like a bank, it lends only deposits
// and never prints. Supply uses cToken-style shares so interest accrues into every
// share's value; debt accrues through a borrow index.
//
// Economics (docs/governance/ECONOMICS.md): the borrow rate follows utilization along a kinked curve,
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
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package lend

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/gas"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/management"
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
	// sweepCooldown rate-limits the permissionless SweepDividends (D-2 defense-in-depth): the
	// reserve can be swept at most once per ~hour, so a JIT staker cannot trigger a fresh
	// distribution on demand at the instant their stake peaks. The primary anti-JIT fix is
	// the dividends module's own unstake lock; this bounds the sweep's timing as well.
	sweepCooldown = 240 // ~1 hour @ 15s
	// upgradeDelay is the self-timelock on a bytecode upgrade (C2 remediation): a proposed NEF
	// must sit in the open for ~1 day before ApplyUpgrade can land it over all deposits and
	// collateral. A bare one-call owner Update is the retained-key Flamingo failure mode; the
	// delay makes any upgrade visible and contestable. (Renouncing/moving the owner to
	// governance is the Tier-3 step; this closes the no-delay hole in code now.)
	upgradeDelay = 5760 // ~1 day @ 15s
	// Volatility guards (governable via SetGuards; 0 in storage => these defaults). The lagged
	// reference tracks spot at most defMaxRefStep bps of itself per block, so a ~25% gap closes
	// in a few hours @15s blocks; defBreaker is the spot-vs-reference deviation that hard-pauses
	// borrows on a pump / liquidations on a dump. See claude/ftb-liquidation-model-study.md.
	defMaxRefStep = 3    // bps of ref per block
	defBreaker    = 2500 // 25%
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
	keySweepBlk  = []byte{0x24} // height of the last successful dividend sweep (D-2 cooldown)

	keyUpgNef = []byte{0x25} // pending upgrade NEF (C2 self-timelock)
	keyUpgMan = []byte{0x26} // pending upgrade manifest
	keyUpgEta = []byte{0x27} // earliest height ApplyUpgrade may land

	keyRefNeo     = []byte{0x50} // lagged reference price for NEO (volatility guard)
	keyRefGas     = []byte{0x51} // lagged reference price for GAS
	keyRefBlkNeo  = []byte{0x52} // last block refNEO was advanced
	keyRefBlkGas  = []byte{0x53} // last block refGAS was advanced
	keyMaxRefStep = []byte{0x54} // governed: max ref move per block, bps of ref (0 => default)
	keyBreaker    = []byte{0x55} // governed: deviation bps that pauses the one-sided action (0 => default)

	prefixShares = []byte{0x30}
	prefixBNorm  = []byte{0x31}
	prefixColN   = []byte{0x32} // NEO collateral (whole units)
	prefixColG   = []byte{0x33} // GAS collateral (base units)
	prefixLiqs   = []byte{0x34} // per-borrower liquidation count — the board's LIQUIDATED tag
)

//lint:ignore U1000 VM entry point, invoked by the chain via the manifest, not from Go
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
	storage.Put(ctx, keyResBps, 3333) // 1/3 of interest to FTB holders (docs/governance/ECONOMICS.md)
	// rates ship at zero: set a real curve with SetRates before launch
}

// ---- admin (owner = multisig) ----

func Configure(registry, ftb interop.Hash160) {
	requireOwner()
	if len(registry) != 20 || len(ftb) != 20 {
		panic("configure: bad hash")
	}
	ctx := storage.GetContext()
	// H2 remediation: wiring is write-once. A re-pointable registry/ftb seam lets the owner
	// swap the oracle/token references instantly, bypassing the registry timelock. Set once
	// at launch; a later change must go through a timelocked module replacement.
	if storage.Get(ctx, keyFTB) != nil {
		panic("configure: already configured")
	}
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

// SetGuards tunes the volatility protection (claude/ftb-liquidation-model-study.md): maxRefStep
// is the bps of the lagged reference the price may move per block (smaller = longer protective
// window); breaker is the spot-vs-reference deviation in bps that pauses borrows on a pump and
// liquidations on a dump. Owner only; both bounded so a mis-set can neither disable the
// protection nor freeze the market permanently.
func SetGuards(maxRefStep, breaker int) {
	requireOwner()
	if maxRefStep <= 0 || maxRefStep > 1000 || breaker < 500 || breaker > 9000 {
		panic("bad guard params")
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyMaxRefStep, maxRefStep)
	storage.Put(ctx, keyBreaker, breaker)
	runtime.Notify("GuardsSet", maxRefStep, breaker)
}

// ---- self-timelocked upgrade (C2 remediation) ----
//
// The bytecode upgrade is a two-step, timelocked, evented act over the contract that custodies
// all supplier FTB and all NEO/GAS collateral. ProposeUpgrade records the new NEF+manifest and
// stamps an ETA upgradeDelay blocks out; the delay elapses in the open; ApplyUpgrade then lands
// it. CancelUpgrade drops a pending proposal. This replaces the former one-call owner Update —
// the retained-key Flamingo failure mode — so no upgrade can land without a visible delay.

// ProposeUpgrade stages a bytecode upgrade behind the timelock. Owner only.
func ProposeUpgrade(nef []byte, manifest []byte) {
	requireOwner()
	if len(nef) == 0 || len(manifest) == 0 {
		panic("upgrade: empty nef or manifest")
	}
	ctx := storage.GetContext()
	eta := ledger.CurrentIndex() + upgradeDelay
	storage.Put(ctx, keyUpgNef, nef)
	storage.Put(ctx, keyUpgMan, manifest)
	storage.Put(ctx, keyUpgEta, eta)
	runtime.Notify("UpgradeProposed", eta)
}

// CancelUpgrade discards a pending upgrade. Owner only.
func CancelUpgrade() {
	requireOwner()
	ctx := storage.GetContext()
	storage.Delete(ctx, keyUpgNef)
	storage.Delete(ctx, keyUpgMan)
	storage.Delete(ctx, keyUpgEta)
	runtime.Notify("UpgradeCancelled")
}

// ApplyUpgrade performs a staged upgrade once its timelock has elapsed. Owner only.
func ApplyUpgrade() {
	requireOwner()
	ctx := storage.GetContext()
	eta := getIntCtx(ctx, keyUpgEta)
	if eta == 0 {
		panic("upgrade: nothing pending")
	}
	if ledger.CurrentIndex() < eta {
		panic("upgrade: timelock not elapsed")
	}
	nef := storage.Get(ctx, keyUpgNef).([]byte)
	manifest := storage.Get(ctx, keyUpgMan).([]byte)
	storage.Delete(ctx, keyUpgNef)
	storage.Delete(ctx, keyUpgMan)
	storage.Delete(ctx, keyUpgEta)
	management.UpdateWithData(nef, manifest, nil) // replaces this contract; no code runs after
}

// PendingUpgradeEta is the height at which a staged upgrade becomes applicable, or 0 if none.
func PendingUpgradeEta() int { return getInt(keyUpgEta) }

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
		if d > collatValue(oracleHash(), user, false, false, true)*getInt(keyLTV)/bps {
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
	if DebtOf(user)+amount > collatValue(oracleHash(), user, false, true, true)*getInt(keyLTV)/bps {
		panic("exceeds LTV")
	}
	ctx := storage.GetContext()
	// Normalized debt rounds UP (divUp), never down: floor division let the smallest
	// debts round to zero normalized units, which made 1 base unit of debt unrepayable
	// (a repay computed zero reduction, so the dust could never die) and let borrows
	// under-charge by dust. Ceiling charges the borrower the dust, so a repay that
	// covers DebtOf always zeroes the position — a closed loan closes completely.
	storage.Put(ctx, bnormKey(user), getIntCtx(ctx, bnormKey(user))+divUp(amount*scale, bidx()))
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
	storage.Put(ctx, bnormKey(user), getIntCtx(ctx, bnormKey(user))-divUp(pay*scale, bidx()))
	storage.Put(ctx, keyTotBorrow, getIntCtx(ctx, keyTotBorrow)-pay)
	if amount > pay {
		send(ftbHash(), user, amount-pay) // refund overpay
	}
}

// ---- liquidation ----

func liquidateIn(liquidator interop.Hash160, repay int, borrower interop.Hash160) {
	ctx := storage.GetContext()
	// O6 remediation: no self-liquidation — a borrower cannot pay themselves the bonus to
	// exit a position; liquidation is a third-party action.
	if string(liquidator) == string(borrower) {
		panic("liquidate: self-liquidation")
	}
	orc := oracleHash() // resolved once per transaction, not once per price read
	cv := collatValue(orc, borrower, true, true, true)
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
	storage.Put(ctx, bnormKey(borrower), getIntCtx(ctx, bnormKey(borrower))-divUp(pay*scale, bidx()))
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
	// O4: if the seizure exhausts the borrower's collateral and debt remains, that residual is
	// bad debt. Write it off now — charged to the holders' reserve first (their fee and
	// first-loss buffer), the remainder socialized to suppliers via totBorrow — so no
	// uncollectible interest lingers in the pool or keeps accruing into the reserve.
	if cn-seizeN == 0 && cg-seizeG == 0 {
		residual := DebtOf(borrower)
		if residual > 0 {
			resv := getIntCtx(ctx, keyResAccrued)
			absorbed := residual
			if absorbed > resv {
				absorbed = resv
			}
			storage.Put(ctx, keyResAccrued, resv-absorbed)
			storage.Put(ctx, keyTotBorrow, getIntCtx(ctx, keyTotBorrow)-residual)
			storage.Put(ctx, bnormKey(borrower), 0)
			runtime.Notify("BadDebtWritten", borrower, residual, absorbed)
		}
	}
	if seizeN > 0 {
		send(interop.Hash160(neo.Hash), liquidator, seizeN)
	}
	if seizeG > 0 {
		send(interop.Hash160(gas.Hash), liquidator, seizeG)
	}
	if repay > pay {
		send(ftbHash(), liquidator, repay-pay) // refund overpay
	}
	// Record it: a liquidation is a fact about the borrower that outlives the
	// position itself (the board tags LIQUIDATED even after the debt is gone).
	lk := liqKey(borrower)
	storage.Put(ctx, lk, getIntCtx(ctx, lk)+1)
	runtime.Notify("Liquidated", borrower, pay, seizeN, seizeG)
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

// SweepDividends pays the accrued reserve: 1/3 of it to the treasury (the dev fund — 1/9
// of total interest), the rest to the dividends module (2/9). Both are registry bindings,
// timelocked and in the open. Dividends is fail-closed while unbound; the treasury leg is
// optional — until it is bound, the holders receive the whole reserve and nothing is stuck.
func SweepDividends() {
	ctx := storage.GetContext()
	// D-2 defense-in-depth: rate-limit the permissionless sweep so its timing cannot be
	// weaponized. The first sweep (keySweepBlk == 0) is always allowed; after that at least
	// sweepCooldown blocks must pass between sweeps.
	now := ledger.CurrentIndex()
	last := getIntCtx(ctx, keySweepBlk)
	if last != 0 && now-last < sweepCooldown {
		panic("sweep: cooldown not elapsed")
	}
	accrue()
	reserve := getIntCtx(ctx, keyResAccrued)
	if reserve <= 0 {
		panic("sweep: no reserve")
	}
	div := contract.Call(regHash(), "get", contract.ReadOnly, "dividends")
	if div == nil {
		panic("sweep: dividends module not bound")
	}
	tre := contract.Call(regHash(), "get", contract.ReadOnly, "treasury")
	storage.Put(ctx, keyResAccrued, 0)
	storage.Put(ctx, keySweepBlk, now) // consume the cooldown only on a real sweep
	dividendsAmt := reserve
	if tre != nil {
		treasuryAmt := reserve / 3 // of total interest this leg is exactly 1/9
		if treasuryAmt > 0 {
			send(ftbHash(), tre.(interop.Hash160), treasuryAmt)
			dividendsAmt -= treasuryAmt
			runtime.Notify("TreasurySwept", tre.(interop.Hash160), treasuryAmt)
		}
	}
	send(ftbHash(), div.(interop.Hash160), dividendsAmt)
	runtime.Notify("DividendsSwept", div.(interop.Hash160), dividendsAmt)
}

// ---- views (safe) ----

func DebtOf(user interop.Hash160) int { return getInt(bnormKey(user)) * bidx() / scale }

// CollateralValue is the conservative borrow-basis value (min(spot,ref)) of a user's
// collateral — what new borrows are sized against.
func CollateralValue(user interop.Hash160) int {
	return collatValue(oracleHash(), user, false, false, false)
}

// collatValue prices a user's NEO+GAS collateral under the volatility protection
// (claude/ftb-liquidation-model-study.md). forLiquidation selects max(spot,ref) over
// min(spot,ref); applyBreaker enables the one-sided pause panic (pump→borrows, dump→
// liquidations); advance updates the lagged reference and so is set only on write paths —
// views pass false and read the stored reference.
func collatValue(orc, user interop.Hash160, forLiquidation, applyBreaker, advance bool) int {
	v := 0
	if n := col(user, true); n > 0 {
		v += n * guardedPrice(orc, true, forLiquidation, applyBreaker, advance)
	}
	if g := col(user, false); g > 0 {
		v += g * guardedPrice(orc, false, forLiquidation, applyBreaker, advance) / dec
	}
	return v
}
func MaxBorrow(user interop.Hash160) int { return CollateralValue(user) * getInt(keyLTV) / bps }

// IsUnhealthy uses the liquidation basis (max(spot,ref)) — the valuation liquidateIn enforces —
// so a transient dump does not mark a position liquidatable.
func IsUnhealthy(user interop.Hash160) bool {
	return DebtOf(user) > collatValue(oracleHash(), user, true, false, false)*getInt(keyLiq)/bps
}

// HealthFactor is the liquidation-basis collateral value times the liquidation threshold as a
// multiple of debt, in bps: >= bps healthy, < bps liquidatable. It reflects the same
// max(spot,ref) valuation the contract acts on, so a warning bot sees exactly what will fire.
// A debt-free user returns a large sentinel.
func HealthFactor(user interop.Hash160) int {
	d := DebtOf(user)
	if d <= 0 {
		return 2147483647
	}
	return collatValue(oracleHash(), user, true, false, false) * getInt(keyLiq) / d
}
func SharesOf(user interop.Hash160) int { return getInt(shareKey(user)) }
func TotalShares() int                  { return getInt(keyTotShares) }
func TotalBorrows() int                 { return getInt(keyTotBorrow) }

// CollateralOf is the user's posted collateral: whole NEO when isNeo, GAS base units
// otherwise. Bookkeeping only — the contract's raw GAS balance also holds claim dust.
func CollateralOf(user interop.Hash160, isNeo bool) int { return col(user, isNeo) }

// ReserveAccrued is the FTB owed to FTB holders, waiting for the next sweep.
func ReserveAccrued() int { return getInt(keyResAccrued) }

// LiquidationsOf is the borrower's lifetime liquidation count — the loans board
// tags LIQUIDATED from this, even after the position itself has closed.
func LiquidationsOf(user interop.Hash160) int { return getInt(liqKey(user)) }

// UtilizationBps: totalBorrows as bps of the lendable pool (available cash + borrows).
func UtilizationBps() int {
	tb := getInt(keyTotBorrow)
	if tb == 0 {
		return 0
	}
	denom := cashAvailable() + tb
	// O5 remediation: in a bad-debt edge (reserve > cash) the denominator can go <= 0;
	// clamp utilization to 100% rather than divide by a non-positive number or report an
	// out-of-range value that would push the rate past the audited ceiling.
	if denom <= 0 {
		return bps
	}
	u := tb * bps / denom
	if u > bps {
		u = bps
	}
	return u
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
	p := contract.Call(orc, "price", contract.ReadOnly, asset).(int)
	// O1 remediation: never trust the oracle blindly. A non-positive price is used as a
	// divisor in liquidation (seize-all / div-by-zero) and would zero collateral value in
	// borrow checks — fail closed instead of acting on garbage.
	if p <= 0 {
		panic("price: oracle returned non-positive")
	}
	return p
}

// ---- volatility protection: lagged reference + dual valuation + breakers ----
// Design: claude/ftb-liquidation-model-study.md.

func maxRefStepBps() int {
	s := getInt(keyMaxRefStep)
	if s <= 0 {
		return defMaxRefStep
	}
	return s
}
func breakerBps() int {
	b := getInt(keyBreaker)
	if b <= 0 {
		return defBreaker
	}
	return b
}
func assetHash(isNeo bool) interop.Hash160 {
	if isNeo {
		return interop.Hash160(neo.Hash)
	}
	return interop.Hash160(gas.Hash)
}
func refKey(isNeo bool) []byte {
	if isNeo {
		return keyRefNeo
	}
	return keyRefGas
}
func refBlkKey(isNeo bool) []byte {
	if isNeo {
		return keyRefBlkNeo
	}
	return keyRefBlkGas
}

// touchRef advances one asset's reference toward spot by at most maxRefStepBps of the
// reference per block elapsed. Write path only; idempotent within a block.
func touchRef(isNeo bool, spot int) {
	ctx := storage.GetContext()
	rk := refKey(isNeo)
	ref := getIntCtx(ctx, rk)
	now := ledger.CurrentIndex()
	if ref <= 0 {
		storage.Put(ctx, rk, spot)
		storage.Put(ctx, refBlkKey(isNeo), now)
		return
	}
	blocks := now - getIntCtx(ctx, refBlkKey(isNeo))
	if blocks <= 0 {
		return
	}
	mv := ref * maxRefStepBps() * blocks / bps
	if spot > ref {
		ref += mv
		if ref > spot {
			ref = spot
		}
	} else if spot < ref {
		ref -= mv
		if ref < spot {
			ref = spot
		}
	}
	storage.Put(ctx, rk, ref)
	storage.Put(ctx, refBlkKey(isNeo), now)
}

// guardedPrice returns the protected unit price: min(spot,ref) for borrow, max(spot,ref) for
// liquidation. applyBreaker panics the one-sided pause (pump→borrows, dump→liquidations);
// advance updates the reference (write paths only).
func guardedPrice(orc interop.Hash160, isNeo, forLiquidation, applyBreaker, advance bool) int {
	spot := priceFrom(orc, assetHash(isNeo))
	if advance {
		touchRef(isNeo, spot)
	}
	ref := getInt(refKey(isNeo))
	if ref <= 0 {
		ref = spot
	}
	b := breakerBps()
	if forLiquidation {
		if applyBreaker && spot*bps <= ref*(bps-b) {
			panic("breaker: abnormal downward move, liquidations paused")
		}
		if ref > spot {
			return ref
		}
		return spot
	}
	if applyBreaker && spot*bps >= ref*(bps+b) {
		panic("breaker: abnormal upward move, borrows paused")
	}
	if ref < spot {
		return ref
	}
	return spot
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
func liqKey(u interop.Hash160) []byte   { return append(prefixLiqs, u...) }

// divUp is ceiling division for the normalized-debt math. Safety at repay: DebtOf
// floors (bnorm*bidx/scale), so bnorm*bidx >= debt*scale; with pay <= debt,
// divUp(pay*scale, bidx) <= bnorm — the subtraction can never underflow.
func divUp(a, b int) int { return (a + b - 1) / b }
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
