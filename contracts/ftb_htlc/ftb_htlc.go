// Package ftbhtlc is the FTB hash-time-locked contract: the Neo N3 settlement leg of the
// non-custodial Neo N3 <-> Neo X atomic swap (Module 1, component 1). It is the reference
// implementation of the shared FASP settlement primitive; Module 4 (BTC/XMR/ZEC) reuses the
// same primitive with chain-specific adapters.
//
// There is no owner, no oracle, no governance, no upgrade path, and no admin key. The
// contract is asset-agnostic: whatever NEP-17 token is transferred in is the escrowed asset.
// It enforces exactly one fact per swap: the escrowed amount is paid out once -- to the
// recipient on a correct preimage, or back to the sender after the timelock -- never both,
// and refund is always eventually reachable, so funds are never stuck locked.
//
// Lifecycle. A locker transfers `amount` of any NEP-17 asset with
// data = ["lock", hashlock, recipient, timelock], which escrows the asset under `hashlock`
// (SHA-256 of a 256-bit secret) and a `timelock` (a Neo N3 block height). Before the
// timelock, anyone revealing the preimage claims the asset for the fixed `recipient`
// (revealing the secret is the authorization; the caller's identity is irrelevant). At or
// after the timelock, anyone may refund the asset to the fixed `sender`. The preimage is
// emitted in the Claimed event so the counterparty's watcher extracts it for the other leg.
//
// The one safety rule the contract cannot enforce is the cross-leg timelock ladder
// (T_A > T_B); only the client sees both chains at once. See the spec, section 5.
//
// Security: checks-effects-interactions on every path (state is written before any token is
// sent); claim and refund pay only the addresses fixed at lock, so both are permissionless;
// a reused hashlock faults, which reverts the locker's own transfer, so nothing is ever
// half-locked. The outbound transfer checks the NEP-17 boolean (FTB-15).
//
// Reference implementation. Internal audit lineage only -- verify before trusting value.
package ftbhtlc

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/crypto"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/gas"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const (
	// A hashlock is SHA-256(secret); a preimage is the 256-bit secret. Both are fixed width.
	hashLen     = 32
	preimageLen = 32

	// A lock must be refundable strictly in the future, with a one-block margin, so a swap is
	// never created already past its own refund height. This is a foot-gun guard, not the
	// safety rule; the real cross-leg ladder is the client's (spec section 5).
	minLockBlocks = 1

	// Swap states. stateNone(0) is the zero value, so an unused hashlock is distinguishable
	// from a live swap -- and that same state != NONE check is the uniqueness guard that
	// faults a reused hashlock.
	stateNone     = 0
	stateLocked   = 1
	stateClaimed  = 2
	stateRefunded = 3
)

var (
	keyVersion = []byte{0x01} // contract version tag, written once at _deploy

	prefixSender = []byte{0x10} // + hashlock -> locker (sender)
	prefixRecip  = []byte{0x11} // + hashlock -> recipient
	prefixToken  = []byte{0x12} // + hashlock -> asset (NEP-17 hash)
	prefixAmount = []byte{0x13} // + hashlock -> amount
	prefixLock   = []byte{0x14} // + hashlock -> timelock (block height)
	prefixState  = []byte{0x15} // + hashlock -> state
)

//lint:ignore U1000 VM entry point, invoked by the chain via the manifest, not from Go
func _deploy(data any, isUpdate bool) {
	if isUpdate {
		return
	}
	ctx := storage.GetContext()
	storage.Put(ctx, keyVersion, 1)
	runtime.Notify("Deployed", 1)
}

// OnNEP17Payment is the only entry for value. A locker transfers any NEP-17 asset with
// data = ["lock", hashlock, recipient, timelock]; every other token payload faults, which
// reverts the sender's own transfer, so nothing is ever half-locked.
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	token := runtime.GetCallingScriptHash()
	if from == nil {
		// A null sender is a mint. The only mint that can land here is the GAS the chain
		// distributes to this contract whenever its NEO balance changes (it can hold NEO as an
		// escrowed asset). It belongs to no swap; accept it silently. Rejecting it would fault
		// every NEO movement and brick claims and refunds of NEO escrows.
		if !eq(token, interop.Hash160(gas.Hash)) {
			panic("unexpected mint")
		}
		return
	}
	arr := data.([]any)
	action := arr[0].(string)
	if action != "lock" {
		panic("htlc: bad action")
	}
	lock(from, token, amount, arr)
}

// lock escrows `amount` of the calling token under `hashlock` with a block-height timelock.
func lock(sender, token interop.Hash160, amount int, arr []any) {
	hashlock := arr[1].([]byte)
	recipient := arr[2].(interop.Hash160)
	timelock := arr[3].(int)

	if amount <= 0 {
		panic("lock: amount must be positive")
	}
	if len(hashlock) != hashLen {
		panic("lock: hashlock must be 32 bytes")
	}
	if len(recipient) != 20 {
		panic("lock: recipient must be a 20-byte address")
	}
	if timelock <= ledger.CurrentIndex()+minLockBlocks {
		panic("lock: timelock must be in the future")
	}

	ctx := storage.GetContext()
	sk := hkey(prefixState, hashlock)
	if getIntCtx(ctx, sk) != stateNone {
		panic("lock: hashlock already used")
	}

	// Effects (no interaction on this path: the asset is already escrowed by the transfer).
	storage.Put(ctx, hkey(prefixSender, hashlock), sender)
	storage.Put(ctx, hkey(prefixRecip, hashlock), recipient)
	storage.Put(ctx, hkey(prefixToken, hashlock), token)
	storage.Put(ctx, hkey(prefixAmount, hashlock), amount)
	storage.Put(ctx, hkey(prefixLock, hashlock), timelock)
	storage.Put(ctx, sk, stateLocked)

	runtime.Notify("Locked", hashlock, sender, recipient, token, amount, timelock)
}

// Claim pays the escrow to the fixed recipient on a correct preimage. Permissionless:
// revealing the preimage is the authorization, and funds only ever go to `recipient`, so the
// caller's identity does not matter. The preimage is emitted for the counterparty's watcher.
func Claim(hashlock, preimage []byte) {
	ctx := storage.GetContext()
	sk := hkey(prefixState, hashlock)
	if getIntCtx(ctx, sk) != stateLocked {
		panic("claim: not locked")
	}
	if len(preimage) != preimageLen {
		panic("claim: preimage must be 32 bytes")
	}
	if string(crypto.Sha256(preimage)) != string(hashlock) {
		panic("claim: preimage mismatch")
	}

	recipient := storage.Get(ctx, hkey(prefixRecip, hashlock)).(interop.Hash160)
	token := storage.Get(ctx, hkey(prefixToken, hashlock)).(interop.Hash160)
	amount := getIntCtx(ctx, hkey(prefixAmount, hashlock))

	// Effects before interaction.
	storage.Put(ctx, sk, stateClaimed)
	send(token, recipient, amount)

	runtime.Notify("Claimed", hashlock, preimage)
}

// Refund returns the escrow to the fixed sender at or after the timelock. Permissionless for
// the same reason as Claim: funds only ever go to `sender`.
func Refund(hashlock []byte) {
	ctx := storage.GetContext()
	sk := hkey(prefixState, hashlock)
	if getIntCtx(ctx, sk) != stateLocked {
		panic("refund: not locked")
	}
	if ledger.CurrentIndex() < getIntCtx(ctx, hkey(prefixLock, hashlock)) {
		panic("refund: timelock not reached")
	}

	sender := storage.Get(ctx, hkey(prefixSender, hashlock)).(interop.Hash160)
	token := storage.Get(ctx, hkey(prefixToken, hashlock)).(interop.Hash160)
	amount := getIntCtx(ctx, hkey(prefixAmount, hashlock))

	// Effects before interaction.
	storage.Put(ctx, sk, stateRefunded)
	send(token, sender, amount)

	runtime.Notify("Refunded", hashlock)
}

// ---- views ----

// Swap returns the full record: sender, recipient, token, amount, timelock, state.
func Swap(hashlock []byte) []any {
	ctx := storage.GetContext()
	return []any{
		storage.Get(ctx, hkey(prefixSender, hashlock)),
		storage.Get(ctx, hkey(prefixRecip, hashlock)),
		storage.Get(ctx, hkey(prefixToken, hashlock)),
		getIntCtx(ctx, hkey(prefixAmount, hashlock)),
		getIntCtx(ctx, hkey(prefixLock, hashlock)),
		getIntCtx(ctx, hkey(prefixState, hashlock)),
	}
}

func StateOf(hashlock []byte) int { return getInt(hkey(prefixState, hashlock)) }

func Exists(hashlock []byte) bool { return getInt(hkey(prefixState, hashlock)) != stateNone }

func Version() int { return getInt(keyVersion) }

// ---- helpers ----

// send performs a NEP-17 transfer out and checks the boolean result (FTB-15): if a token ever
// returns false instead of faulting, our state was already updated and the tokens must
// provably have moved.
func send(token, to interop.Hash160, amount int) {
	if !contract.Call(token, "transfer", contract.All, runtime.GetExecutingScriptHash(), to, amount, nil).(bool) {
		panic("send: token transfer returned false")
	}
}

func eq(a, b interop.Hash160) bool { return string(a) == string(b) }

// hkey keys a swap field by prefix + the 32-byte hashlock. The prefix slices are len 1 cap 1,
// so append allocates a fresh backing array and never aliases the prefix.
func hkey(prefix, hashlock []byte) []byte { return append(prefix, hashlock...) }

func getInt(key []byte) int { return getIntCtx(storage.GetContext(), key) }

func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
