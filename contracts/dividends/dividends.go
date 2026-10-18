// Package dividends is the FTB holders' share of the fee constitution (ECONOMICS.md):
// the dividends module that lend's sweep and the treasury's excess returns pay into.
// FTB is dividend-bearing through staking — the xSUSHI pattern: stake FTB, and every
// distribution raises the per-share accumulator; unstake any time; claim any time.
//
// Why staking and not hold-and-earn: pro-rata dividends on a freely transferable token
// need a balance-change hook inside the token contract — a permanent coupling of every
// FTB transfer to this module, extra gas on every send, and a change to a sealed token.
// Staking keeps ftb_token untouched and makes accounting exact: staked balances live in
// this contract's state, so no transfer can smuggle a dividend claim past a checkpoint.
//
// Revenue received while nothing is staked is queued, not lost: it folds into the next
// distribution once stakers exist. Floor-division dust stays in the contract as cushion
// and rides into later distributions.
//
// No committee, no owner knob after Configure: this contract is pure accounting.
//
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package dividends

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/management"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const scale = 1_000_000_000_000_000_000 // 1e18 fixed-point for the per-share accumulator

// blocksPerMonth is 1/12 of the 2,102,400-block year (15-second target, lend.go).
// Monthly buckets are keyed by ledger height, not wall clock — deterministic and
// callable from the chain itself.
const blocksPerMonth = 175_200

var (
	keyOwner   = []byte{0x01} // deploy owner; may Configure once
	keyFTB     = []byte{0x02} // the FTB token hash
	keyAcc     = []byte{0x03} // cumulative dividends per staked FTB, 1e18-scaled
	keyTotStk  = []byte{0x04} // total staked FTB
	keyQueued  = []byte{0x05} // revenue received while nothing was staked
	keyTotRecv = []byte{0x06} // lifetime revenue received (FTB base units)
	prefixStk  = []byte{0x10} // + account -> staked balance
	prefixChk  = []byte{0x11} // + account -> accPerShare checkpoint at last touch
	prefixPend = []byte{0x12} // + account -> settled but unclaimed dividends
	prefixMon  = []byte{0x20} // + uint32 BE month index -> revenue received that month
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
	storage.Put(storage.GetContext(), keyOwner, owner)
}

// Configure binds the FTB token. Owner only, write-once.
func Configure(ftb interop.Hash160) {
	requireOwner()
	if len(ftb) != 20 {
		panic("configure: bad address")
	}
	ctx := storage.GetContext()
	if storage.Get(ctx, keyFTB) != nil {
		panic("configure: already configured (write-once)")
	}
	storage.Put(ctx, keyFTB, ftb)
	runtime.Notify("Configured", ftb)
}

// OnNEP17Payment is the single inbound door. Only FTB is accepted.
//
//	data == ["stake"]  -> the amount stakes to the sender (transfer-in-and-stake,
//	                      the same routing pattern as the lend pool)
//	anything else      -> the amount is protocol revenue and distributes to stakers
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	if amount <= 0 {
		panic("dividends: amount must be positive")
	}
	if !eq(runtime.GetCallingScriptHash(), ftbHash()) {
		panic("dividends: only FTB accepted")
	}
	if from == nil {
		panic("dividends: unexpected mint") // FTB never mints here; see lend FTB-29
	}
	if data != nil {
		arr := data.([]any) // a malformed tag faults the sender's tx, never the pool
		if len(arr) == 1 && arr[0].(string) == "stake" {
			stakeIn(from, amount)
			return
		}
	}
	// Revenue is counted when RECEIVED, not when distributed — "dividends generated
	// this month" means what the protocol earned, whether or not stakers exist yet
	// (unstaked revenue queues; the month bucket still records its arrival).
	ctx := storage.GetContext()
	mk := monthKey(int(ledger.CurrentIndex()) / blocksPerMonth)
	storage.Put(ctx, mk, getIntCtx(ctx, mk)+amount)
	storage.Put(ctx, keyTotRecv, getIntCtx(ctx, keyTotRecv)+amount)
	distribute(amount)
}

// ---- monthly revenue ledger (the "dividends" module display) ----

// MonthIndex is the current month number (genesis-relative, 175,200-block months).
func MonthIndex() int { return int(ledger.CurrentIndex()) / blocksPerMonth }

// MonthTotal is the FTB revenue received in month idx (base units; 0 if none).
func MonthTotal(idx int) int {
	if idx < 0 {
		panic("bad month")
	}
	return getInt(monthKey(idx))
}

// TotalReceived is the lifetime FTB revenue paid into this module (base units).
func TotalReceived() int { return getInt(keyTotRecv) }

// Update exists for pre-launch fixes only — the v1 dividends lacked it, which
// would have forced a redeploy for this very addition. Same posture as ftb_token.
func Update(nef []byte, manifest []byte, data any) {
	requireOwner()
	management.UpdateWithData(nef, manifest, data)
}

// distribute folds any queued revenue into the new amount and raises the accumulator.
// With no stakers the revenue queues for the first distribution that has any.
func distribute(amount int) {
	ctx := storage.GetContext()
	total := amount + getIntCtx(ctx, keyQueued)
	storage.Put(ctx, keyQueued, 0)
	staked := getIntCtx(ctx, keyTotStk)
	if staked == 0 {
		storage.Put(ctx, keyQueued, total)
		runtime.Notify("Queued", total)
		return
	}
	storage.Put(ctx, keyAcc, getIntCtx(ctx, keyAcc)+total*scale/staked)
	runtime.Notify("Distributed", total, getIntCtx(ctx, keyTotStk))
}

// stakeIn settles the staker's pending dividends at the old balance, then adds the stake.
func stakeIn(user interop.Hash160, amount int) {
	ctx := storage.GetContext()
	settle(ctx, user)
	storage.Put(ctx, stkKey(user), getIntCtx(ctx, stkKey(user))+amount)
	storage.Put(ctx, keyTotStk, getIntCtx(ctx, keyTotStk)+amount)
	runtime.Notify("Staked", user, amount)
}

// Unstake returns staked FTB. Dividends earned up to this block stay claimable.
func Unstake(user interop.Hash160, amount int) {
	if !runtime.CheckWitness(user) {
		panic("no witness")
	}
	ctx := storage.GetContext()
	s := getIntCtx(ctx, stkKey(user))
	if amount <= 0 || amount > s {
		panic("bad amount")
	}
	settle(ctx, user)
	storage.Put(ctx, stkKey(user), s-amount)
	storage.Put(ctx, keyTotStk, getIntCtx(ctx, keyTotStk)-amount)
	send(ftbHash(), user, amount)
	runtime.Notify("Unstaked", user, amount)
}

// Claim pays out everything the staker has earned. Callable by the staker only.
func Claim(user interop.Hash160) {
	if !runtime.CheckWitness(user) {
		panic("no witness")
	}
	ctx := storage.GetContext()
	settle(ctx, user)
	pk := pendKey(user)
	owed := getIntCtx(ctx, pk)
	if owed <= 0 {
		panic("nothing to claim")
	}
	storage.Put(ctx, pk, 0)
	send(ftbHash(), user, owed)
	runtime.Notify("Claimed", user, owed)
}

// settle folds dividends earned since the last checkpoint into the pending balance.
func settle(ctx storage.Context, user interop.Hash160) {
	acc := getIntCtx(ctx, keyAcc)
	ck := getIntCtx(ctx, chkKey(user))
	if acc > ck {
		earned := getIntCtx(ctx, stkKey(user)) * (acc - ck) / scale
		if earned > 0 {
			pk := pendKey(user)
			storage.Put(ctx, pk, getIntCtx(ctx, pk)+earned)
		}
		storage.Put(ctx, chkKey(user), acc)
	}
}

// ---- views (safe) ----

// PendingOf is what the user could claim this block: settled pending plus the
// not-yet-settled accrual since their checkpoint.
func PendingOf(user interop.Hash160) int {
	ctx := storage.GetContext()
	acc := getIntCtx(ctx, keyAcc)
	ck := getIntCtx(ctx, chkKey(user))
	pending := getIntCtx(ctx, pendKey(user))
	if acc > ck {
		pending += getIntCtx(ctx, stkKey(user)) * (acc - ck) / scale
	}
	return pending
}

func StakedOf(user interop.Hash160) int { return getInt(stkKey(user)) }
func TotalStaked() int                  { return getInt(keyTotStk) }

// AccPerShare is the cumulative 1e18-scaled dividend per staked FTB — the growth curve
// the front end samples for the dividend APY.
func AccPerShare() int { return getInt(keyAcc) }

// Queued is revenue waiting for the first staker distribution.
func Queued() int { return getInt(keyQueued) }

func Token() interop.Hash160 { return ftbHash() }

// ---- helpers ----

func stkKey(u interop.Hash160) []byte  { return append(prefixStk, u...) }
func chkKey(u interop.Hash160) []byte  { return append(prefixChk, u...) }
func pendKey(u interop.Hash160) []byte { return append(prefixPend, u...) }

// monthKey is fixed-width (big-endian uint32) so buckets sort and stay unambiguous.
func monthKey(idx int) []byte {
	return append(prefixMon, byte(idx>>24), byte(idx>>16), byte(idx>>8), byte(idx))
}

func send(token, to interop.Hash160, amount int) {
	if amount == 0 {
		return
	}
	ok := contract.Call(token, "transfer", contract.All,
		runtime.GetExecutingScriptHash(), to, amount, nil).(bool)
	if !ok {
		panic("transfer failed")
	}
}

func ftbHash() interop.Hash160 {
	h := storage.Get(storage.GetContext(), keyFTB)
	if h == nil {
		panic("not configured")
	}
	return h.(interop.Hash160)
}

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil {
		panic("owner not set")
	}
	if !runtime.CheckWitness(owner.(interop.Hash160)) {
		panic("only owner")
	}
}

func eq(a, b interop.Hash160) bool { return string(a) == string(b) }
func getInt(key []byte) int        { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
