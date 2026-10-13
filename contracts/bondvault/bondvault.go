// Package bondvault holds the FTB bonds that projects post when governance lists their
// token (GOVERNANCE.md, listing track). The bond is bought on the open market in FTB and
// split in two at the door:
//
//   - liquidity half — earmarked for the FTB/<token> pool. Until the AMM exists it stays
//     in this vault; a later version moves it into the locked LP position.
//   - vault half — the rug-pull bond. Locked for as long as the project is listed.
//
// Three states per listing: bonded -> slashed -> delisted. Slash and delist are executed
// by the operator committee (multisig) ONLY after a passed ballot — the chain decides,
// this contract holds the money, the committee turns the key. There is no vesting clock
// and no good-behavior unlock: listed = bonded, delisted = released, nothing in between.
//
// Reference implementation pending audit.
package bondvault

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

// listing states
const (
	stateNone     = 0
	stateBonded   = 1
	stateSlashed  = 2
	stateDelisted = 3
)

var (
	keyOwner      = []byte{0x01} // deploy owner; may Configure once
	keyFTB        = []byte{0x02} // the FTB token hash (bonds are paid in FTB)
	keyCommittee  = []byte{0x03} // operator multisig executing passed ballots
	prefixState   = []byte{0x10} // + listed token -> state
	prefixVault   = []byte{0x11} // + listed token -> remaining vault half
	prefixLiq     = []byte{0x12} // + listed token -> remaining liquidity half
	prefixProject = []byte{0x13} // + listed token -> project address (first payer)
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

// Configure binds the FTB token and the operator committee. Owner only, write-once:
// who holds the bonds and who may execute votes must never change after publication.
func Configure(ftb, committee interop.Hash160) {
	requireOwner()
	if len(ftb) != 20 || len(committee) != 20 {
		panic("configure: bad address")
	}
	ctx := storage.GetContext()
	if storage.Get(ctx, keyFTB) != nil {
		panic("configure: already configured (write-once)")
	}
	storage.Put(ctx, keyFTB, ftb)
	storage.Put(ctx, keyCommittee, committee)
	runtime.Notify("Configured", ftb, committee)
}

// OnNEP17Payment receives a bond. Only FTB is accepted; data is the listed token's
// contract hash. A first payment opens the listing (bonded); further payments top it up.
// A delisted token can never bond again — a new listing is a new proposal.
func OnNEP17Payment(from interop.Hash160, amount int, data any) {
	if amount <= 0 {
		panic("bond: amount must be positive")
	}
	caller := runtime.GetCallingScriptHash()
	if !eq(caller, ftbHash()) {
		panic("bond: only FTB accepted")
	}
	token := data.(interop.Hash160)
	if len(token) != 20 {
		panic("bond: data must be the listed token hash")
	}
	ctx := storage.GetContext()
	sk := append(prefixState, token...)
	switch getIntCtx(ctx, sk) {
	case stateNone:
		storage.Put(ctx, sk, stateBonded)
		storage.Put(ctx, append(prefixProject, token...), from)
	case stateBonded:
		// top-up below
	default:
		panic("bond: listing closed") // slashed or delisted listings are over
	}
	liq := amount / 2
	vault := amount - liq
	storage.Put(ctx, append(prefixLiq, token...), getIntCtx(ctx, append(prefixLiq, token...))+liq)
	storage.Put(ctx, append(prefixVault, token...), getIntCtx(ctx, append(prefixVault, token...))+vault)
	runtime.Notify("Bonded", from, token, amount, vault, liq)
}

// Slash sends the vault half to the compensation address after a passed slash ballot.
// The liquidity half stays locked pending the delisting resolution. Committee only.
func Slash(token, to interop.Hash160) {
	requireCommittee()
	if len(to) != 20 {
		panic("slash: bad compensation address")
	}
	ctx := storage.GetContext()
	sk := append(prefixState, token...)
	if getIntCtx(ctx, sk) != stateBonded {
		panic("slash: not a bonded listing")
	}
	vk := append(prefixVault, token...)
	amount := getIntCtx(ctx, vk)
	storage.Put(ctx, vk, 0)
	storage.Put(ctx, sk, stateSlashed)
	send(ftbHash(), to, amount)
	runtime.Notify("Slashed", token, to, amount)
}

// Delist ends a listing after a passed delisting ballot and returns whatever remains
// (both halves, or only the liquidity half after a slash) to the project. Committee only.
func Delist(token interop.Hash160) {
	requireCommittee()
	ctx := storage.GetContext()
	sk := append(prefixState, token...)
	st := getIntCtx(ctx, sk)
	if st != stateBonded && st != stateSlashed {
		panic("delist: no live listing")
	}
	vk := append(prefixVault, token...)
	lk := append(prefixLiq, token...)
	amount := getIntCtx(ctx, vk) + getIntCtx(ctx, lk)
	project := storage.Get(ctx, append(prefixProject, token...)).(interop.Hash160)
	storage.Put(ctx, vk, 0)
	storage.Put(ctx, lk, 0)
	storage.Put(ctx, sk, stateDelisted)
	send(ftbHash(), project, amount)
	runtime.Notify("Delisted", token, project, amount)
}

// ---- views (safe) ----

// State returns the listing state: 0 none, 1 bonded, 2 slashed, 3 delisted.
func State(token interop.Hash160) int { return getInt(append(prefixState, token...)) }

// VaultOf returns the remaining rug-pull bond for a listed token.
func VaultOf(token interop.Hash160) int { return getInt(append(prefixVault, token...)) }

// LiquidityOf returns the remaining liquidity half for a listed token.
func LiquidityOf(token interop.Hash160) int { return getInt(append(prefixLiq, token...)) }

// Project returns the address that posted the bond (the delisting refund target).
func Project(token interop.Hash160) interop.Hash160 {
	v := storage.Get(storage.GetContext(), append(prefixProject, token...))
	if v == nil {
		return nil
	}
	return v.(interop.Hash160)
}

func Committee() interop.Hash160 {
	return storage.Get(storage.GetContext(), keyCommittee).(interop.Hash160)
}
func Token() interop.Hash160 { return ftbHash() }

// ---- helpers ----

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

func requireCommittee() {
	c := storage.Get(storage.GetContext(), keyCommittee)
	if c == nil {
		panic("not configured")
	}
	if !runtime.CheckWitness(c.(interop.Hash160)) {
		panic("only committee")
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
