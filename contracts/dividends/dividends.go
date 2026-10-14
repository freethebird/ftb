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
// Reference implementation pending audit.
package dividends

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const scale = 1_000_000_000_000_000_000 // 1e18 fixed-point for the per-share accumulator

var (
	keyOwner   = []byte{0x01} // deploy owner; may Configure once
	keyFTB     = []byte{0x02} // the FTB token hash
	keyAcc     = []byte{0x03} // cumulative dividends per staked FTB, 1e18-scaled
	keyTotStk  = []byte{0x04} // total staked FTB
	keyQueued  = []byte{0x05} // revenue received while nothing was staked
	prefixStk  = []byte{0x10} // + account -> staked balance
	prefixChk  = []byte{0x11} // + account -> accPerShare checkpoint at last touch
	prefixPend = []byte{0x12} // + account -> settled but unclaimed dividends
)

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
	distribute(amount)
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
