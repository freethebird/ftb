// Package registry is the seam board: a small contract that holds the current hash of
// each swappable module (oracle, lend, router, ...), so modules resolve each other by
// name and any one module can be replaced without rewriting the others.
//
// Governance (MO: visible cost over invisible): after a one-time bootstrap, every change
// is a two-step, timelocked, event-emitting act. You Propose a new hash, the timelock
// elapses in the open, then anyone can Apply it. The delay can only be increased, never
// shortened, and the token (the supply invariant) is deliberately NOT governed here; it
// seals itself and is immutable.
//
// v2 adds the proposals board (GOVERNANCE.md): anyone may put a governance or listing
// proposal on the public board; the cost is an exact bond in FTB, paid by transfer with
// the proposal as the payment data. The board is only the read model — votes live in
// per-proposal ballot contracts, listing money in the bond vault. No new authority is
// created: closing proposals belongs to the same owner multisig, and every act emits an
// event. Bonds sit in escrow and are refunded on a legitimate close or kept by the
// committee against spam — the disposition is explicit in the closing call.
//
// Reference implementation pending audit.
package registry

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/std"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	keyOwner       = []byte{0x01}
	keyDelay       = []byte{0x02} // timelock, in blocks
	keyFinished    = []byte{0x03} // bootstrap finished flag
	keyBondAmt     = []byte{0x04} // required proposal bond, in FTB base units
	keyEscrow      = []byte{0x05} // FTB currently held as proposal bonds
	keyPropCount   = []byte{0x06} // monotonic proposal counter
	prefixValue    = []byte{0x10} // prefixValue + name -> current module hash
	prefixPendVal  = []byte{0x11} // prefixPendVal + name -> proposed hash
	prefixPendEta  = []byte{0x12} // prefixPendEta + name -> earliest apply height
	prefixPropKind = []byte{0x20} // + decimal id -> "ballot" | "listing"
	prefixPropRef  = []byte{0x21} // + id -> ballot contract hash, or token hash for listings
	prefixPropMeta = []byte{0x22} // + id -> human-readable terms (question, bond %, ...)
	prefixPropBy   = []byte{0x23} // + id -> proposer
	prefixPropBond = []byte{0x24} // + id -> bond paid
	prefixPropOpen = []byte{0x25} // + id -> 1 while open (deleted on close)
)

// maxMetaLen bounds the free-text field (Annex B class 5: every input has a length).
const maxMetaLen = 1024

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
	storage.Put(ctx, keyFinished, 0)
	storage.Put(ctx, keyDelay, 0)
}

// ---- reads (safe) ----

func Get(name string) interop.Hash160 {
	v := storage.Get(storage.GetContext(), append(prefixValue, name...))
	if v == nil {
		return nil
	}
	return v.(interop.Hash160)
}
func Delay() int     { return getInt(keyDelay) }
func Finished() bool { return getInt(keyFinished) != 0 }

// ---- one-time bootstrap ----

func SetInitialDelay(blocks int) {
	requireOwner()
	requireNotFinished()
	if blocks < 0 {
		panic("delay must be >= 0")
	}
	storage.Put(storage.GetContext(), keyDelay, blocks)
	runtime.Notify("DelaySet", blocks)
}

func Bootstrap(name string, value interop.Hash160) {
	requireOwner()
	requireNotFinished()
	if len(value) != 20 {
		panic("bootstrap: bad value")
	}
	storage.Put(storage.GetContext(), append(prefixValue, name...), value)
	runtime.Notify("Bootstrapped", name, value)
}

// FinishBootstrap is a one-way door: after it, only timelocked Propose/Apply works, and
// only with a non-zero delay (you cannot lock in a zero timelock).
func FinishBootstrap() {
	requireOwner()
	requireNotFinished()
	if getInt(keyDelay) <= 0 {
		panic("set a non-zero timelock before finishing bootstrap")
	}
	storage.Put(storage.GetContext(), keyFinished, 1)
	runtime.Notify("BootstrapFinished")
}

// ---- governed changes (after bootstrap) ----

func Propose(name string, value interop.Hash160) {
	requireOwner()
	requireFinished()
	if len(value) != 20 {
		panic("propose: bad value")
	}
	eta := ledger.CurrentIndex() + getInt(keyDelay)
	ctx := storage.GetContext()
	storage.Put(ctx, append(prefixPendVal, name...), value)
	storage.Put(ctx, append(prefixPendEta, name...), eta)
	runtime.Notify("Proposed", name, value, eta)
}

// Apply is callable by anyone once the timelock has elapsed: the delay is the protection,
// not who pushes the button.
func Apply(name string) {
	requireFinished()
	ctx := storage.GetContext()
	etaKey := append(prefixPendEta, name...)
	eta := getIntCtx(ctx, etaKey)
	if eta == 0 {
		panic("apply: nothing pending")
	}
	if ledger.CurrentIndex() < eta {
		panic("apply: timelock not elapsed")
	}
	value := storage.Get(ctx, append(prefixPendVal, name...)).(interop.Hash160)
	storage.Put(ctx, append(prefixValue, name...), value)
	storage.Delete(ctx, append(prefixPendVal, name...))
	storage.Delete(ctx, etaKey)
	runtime.Notify("Applied", name, value)
}

func Cancel(name string) {
	requireOwner()
	ctx := storage.GetContext()
	storage.Delete(ctx, append(prefixPendVal, name...))
	storage.Delete(ctx, append(prefixPendEta, name...))
	runtime.Notify("Cancelled", name)
}

// IncreaseDelay can only lengthen the timelock (MO: one-way toward more safety).
func IncreaseDelay(blocks int) {
	requireOwner()
	if blocks <= getInt(keyDelay) {
		panic("delay can only increase")
	}
	storage.Put(storage.GetContext(), keyDelay, blocks)
	runtime.Notify("DelayIncreased", blocks)
}

// ---- proposals board (v2) ----

// SetProposalBond fixes the exact FTB amount a proposal costs. Owner only, evented;
// until it is set the board stays closed (fail-closed, never a silent zero).
func SetProposalBond(amount int) {
	requireOwner()
	if amount <= 0 {
		panic("bond must be positive")
	}
	storage.Put(storage.GetContext(), keyBondAmt, amount)
	runtime.Notify("ProposalBondSet", amount)
}

// OnNEP17Payment submits a proposal. Only FTB is accepted (the module bound as "token",
// itself behind the timelock); the amount must be exactly the configured bond; data is
// [kind, ref, meta] where kind is "ballot" or "listing", ref is the ballot contract hash
// or the listed token hash, and meta is the human-readable terms. No caller, no bond,
// no proposal: submission is permissionless but never free.
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	if !eqHash(runtime.GetCallingScriptHash(), tokenHash()) {
		panic("proposals: only FTB bonds")
	}
	bond := getInt(keyBondAmt)
	if bond == 0 {
		panic("proposals: bond not configured")
	}
	if amount != bond {
		panic("proposals: exact bond required")
	}
	arr := data.([]any) // a malformed bond transfer faults here, fail-closed
	if len(arr) != 3 {
		panic("proposals: data = [kind, ref, meta]")
	}
	kind := arr[0].(string)
	if kind != "ballot" && kind != "listing" {
		panic("proposals: kind must be ballot or listing")
	}
	ref := arr[1].(interop.Hash160)
	if len(ref) != 20 {
		panic("proposals: bad ref")
	}
	meta := arr[2].(string)
	if len(meta) == 0 || len(meta) > maxMetaLen {
		panic("proposals: meta must be 1..1024 bytes")
	}

	ctx := storage.GetContext()
	id := getIntCtx(ctx, keyPropCount) + 1
	idb := []byte(std.Itoa(id, 10))
	storage.Put(ctx, keyPropCount, id)
	storage.Put(ctx, append(prefixPropKind, idb...), kind)
	storage.Put(ctx, append(prefixPropRef, idb...), ref)
	storage.Put(ctx, append(prefixPropMeta, idb...), meta)
	storage.Put(ctx, append(prefixPropBy, idb...), from)
	storage.Put(ctx, append(prefixPropBond, idb...), amount)
	storage.Put(ctx, append(prefixPropOpen, idb...), 1)
	storage.Put(ctx, keyEscrow, getIntCtx(ctx, keyEscrow)+amount)
	runtime.Notify("ProposalSubmitted", id, kind, ref, from, amount)
}

// CloseProposal resolves a proposal after its outcome is known. Owner only. refund=true
// returns the bond to the proposer (the proposal ran its course); refund=false pays it
// to the committee (spam never comes back). The record stays on-chain either way —
// only the open flag is cleared. Closing twice panics.
func CloseProposal(id int, refund bool) {
	requireOwner()
	ctx := storage.GetContext()
	idb := []byte(std.Itoa(id, 10))
	openKey := append(prefixPropOpen, idb...)
	if getIntCtx(ctx, openKey) == 0 {
		panic("close: not open")
	}
	bond := getIntCtx(ctx, append(prefixPropBond, idb...))
	proposer := storage.Get(ctx, append(prefixPropBy, idb...)).(interop.Hash160)
	to := proposer
	if !refund {
		to = storage.Get(ctx, keyOwner).(interop.Hash160)
	}
	storage.Delete(ctx, openKey)
	storage.Put(ctx, keyEscrow, getIntCtx(ctx, keyEscrow)-bond)
	sendFTB(to, bond)
	runtime.Notify("ProposalClosed", id, refund, to, bond)
}

// ---- proposals board views (safe) ----

func ProposalCount() int { return getInt(keyPropCount) }
func ProposalBond() int  { return getInt(keyBondAmt) }

// Escrow exposes the FTB held as proposal bonds, so anyone can check solvency against
// the contract's actual balance.
func Escrow() int { return getInt(keyEscrow) }

// Proposal returns [kind, ref, meta, proposer, bond, open] for an id, or panics.
func Proposal(id int) []any {
	ctx := storage.GetContext()
	idb := []byte(std.Itoa(id, 10))
	kind := storage.Get(ctx, append(prefixPropKind, idb...))
	if kind == nil {
		panic("proposal: no such id")
	}
	return []any{
		kind.(string),
		storage.Get(ctx, append(prefixPropRef, idb...)).(interop.Hash160),
		storage.Get(ctx, append(prefixPropMeta, idb...)).(string),
		storage.Get(ctx, append(prefixPropBy, idb...)).(interop.Hash160),
		getIntCtx(ctx, append(prefixPropBond, idb...)),
		getIntCtx(ctx, append(prefixPropOpen, idb...)) != 0,
	}
}

// ---- helpers ----

// tokenHash resolves the FTB module hash. Fail-closed: if no token is bound, every
// bond path panics rather than accepting the wrong asset.
func tokenHash() interop.Hash160 {
	v := storage.Get(storage.GetContext(), append(prefixValue, "token"...))
	if v == nil {
		panic("token module not bound")
	}
	return v.(interop.Hash160)
}

func sendFTB(to interop.Hash160, amount int) {
	if amount == 0 {
		return
	}
	ok := contract.Call(tokenHash(), "transfer", contract.All,
		runtime.GetExecutingScriptHash(), to, amount, nil).(bool)
	if !ok {
		panic("transfer failed")
	}
}

func eqHash(a, b interop.Hash160) bool { return string(a) == string(b) }

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil || !runtime.CheckWitness(owner.(interop.Hash160)) {
		panic("only owner")
	}
}
func requireFinished() {
	if getInt(keyFinished) == 0 {
		panic("bootstrap not finished")
	}
}
func requireNotFinished() {
	if getInt(keyFinished) != 0 {
		panic("bootstrap already finished")
	}
}
func getInt(key []byte) int { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
