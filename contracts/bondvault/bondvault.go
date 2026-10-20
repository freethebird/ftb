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
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package bondvault

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

// listing states. BV-01/BV-02 remediation added stateRegistered: a listing must be
// authorized by the committee (which binds its refund target and slash sink up-front)
// BEFORE it can take a bond. The old flow let the first payer open a listing and become
// its refund target, which is the hijack this closes.
const (
	stateNone       = 0
	stateRegistered = 1 // committee-authorized, refund + compensation bound, awaiting its bond
	stateBonded     = 2
	stateSlashed    = 3
	stateDelisted   = 4
)

var (
	keyOwner      = []byte{0x01} // deploy owner; may Configure once
	keyFTB        = []byte{0x02} // the FTB token hash (bonds are paid in FTB)
	keyCommittee  = []byte{0x03} // operator multisig executing passed ballots
	prefixState   = []byte{0x10} // + listed token -> state
	prefixVault   = []byte{0x11} // + listed token -> remaining vault half
	prefixLiq     = []byte{0x12} // + listed token -> remaining liquidity half
	prefixProject = []byte{0x13} // + listed token -> project address (committee-registered refund target)
	prefixComp    = []byte{0x14} // + listed token -> compensation sink a slash pays out to (committee-registered)
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

// RegisterListing authorizes a listing after its ballot passes. The committee binds, in the
// open and up-front, BOTH the refund target (the project address a delist returns to) and the
// compensation sink (the address a slash pays out to). Only a registered listing can take a
// bond, and neither address can be set or changed at bond or slash time:
//
//   - BV-01 (refund hijack): the refund target is the committee-registered project, never
//     "whoever paid the first FTB", so a front-runner can no longer capture the refund.
//   - BV-02 (slash-to-arbitrary): the slash destination is fixed here, declared publicly at
//     registration, so a committee key can trigger a slash but cannot redirect its proceeds
//     to itself at slash time. The sink may be neither the committee nor the project.
//
// Committee only, one-time per token (a slashed or delisted token is finished — a new
// listing is a new proposal with a fresh token record).
func RegisterListing(token, project, compensation interop.Hash160) {
	requireCommittee()
	if len(token) != 20 || len(project) != 20 || len(compensation) != 20 {
		panic("register: bad address")
	}
	ctx := storage.GetContext()
	committee := storage.Get(ctx, keyCommittee).(interop.Hash160)
	if eq(compensation, committee) {
		panic("register: compensation must not be the committee") // BV-02: no self-drain
	}
	if eq(compensation, project) {
		panic("register: compensation must not be the project") // a slash must not refund the rugger
	}
	sk := append(prefixState, token...)
	if getIntCtx(ctx, sk) != stateNone {
		panic("register: listing already exists")
	}
	storage.Put(ctx, sk, stateRegistered)
	storage.Put(ctx, append(prefixProject, token...), project)
	storage.Put(ctx, append(prefixComp, token...), compensation)
	runtime.Notify("ListingRegistered", token, project, compensation)
}

// OnNEP17Payment receives a bond. Only FTB is accepted; data is the listed token's contract
// hash, which MUST already be a committee-registered listing (RegisterListing). The first
// payment opens the registered listing (bonded); further payments top it up. The payer is
// not recorded as anything — the refund target was fixed at registration (BV-01). A slashed
// or delisted token can never bond again.
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
	case stateRegistered:
		storage.Put(ctx, sk, stateBonded) // first bond opens the committee-registered listing
	case stateBonded:
		// top-up below
	default:
		// stateNone: not registered (BV-01 — no permissionless listing creation);
		// stateSlashed / stateDelisted: the listing is over.
		panic("bond: listing not open (register it first; closed listings are final)")
	}
	liq := amount / 2
	vault := amount - liq
	storage.Put(ctx, append(prefixLiq, token...), getIntCtx(ctx, append(prefixLiq, token...))+liq)
	storage.Put(ctx, append(prefixVault, token...), getIntCtx(ctx, append(prefixVault, token...))+vault)
	runtime.Notify("Bonded", from, token, amount, vault, liq)
}

// Slash sends the vault half to the listing's PRE-REGISTERED compensation sink after a passed
// slash ballot. BV-02 remediation: the destination is the address bound at RegisterListing,
// in the open and up-front — it is not a parameter here, so a committee key can trigger a
// slash but cannot choose or redirect where the money goes at slash time. The liquidity half
// stays locked pending the delisting resolution. Committee only.
func Slash(token interop.Hash160) {
	requireCommittee()
	ctx := storage.GetContext()
	sk := append(prefixState, token...)
	if getIntCtx(ctx, sk) != stateBonded {
		panic("slash: not a bonded listing")
	}
	cv := storage.Get(ctx, append(prefixComp, token...))
	if cv == nil {
		panic("slash: no compensation sink registered") // unreachable for a registered listing; fail-closed
	}
	to := cv.(interop.Hash160)
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

// State returns the listing state: 0 none, 1 registered, 2 bonded, 3 slashed, 4 delisted.
func State(token interop.Hash160) int { return getInt(append(prefixState, token...)) }

// CompensationOf returns the committee-registered slash destination for a listed token
// (nil if the token was never registered). The slash sink is public from registration on.
func CompensationOf(token interop.Hash160) interop.Hash160 {
	v := storage.Get(storage.GetContext(), append(prefixComp, token...))
	if v == nil {
		return nil
	}
	return v.(interop.Hash160)
}

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
