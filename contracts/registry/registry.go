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
// Reference implementation pending audit.
package registry

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	keyOwner      = []byte{0x01}
	keyDelay      = []byte{0x02} // timelock, in blocks
	keyFinished   = []byte{0x03} // bootstrap finished flag
	prefixValue   = []byte{0x10} // prefixValue + name -> current module hash
	prefixPendVal = []byte{0x11} // prefixPendVal + name -> proposed hash
	prefixPendEta = []byte{0x12} // prefixPendEta + name -> earliest apply height
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

// ---- helpers ----

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
