// Package ftbgov is FTB's keyless on-chain governance executor. A proposal commits a SUBJECT
// hash -- sha256 of the exact action and parameters an executor will carry out -- opens a
// voting window, and is decided by FTB stakers: vote weight is the voter's staked FTB in the
// dividends module, one vote per address per proposal. A proposal PASSES when, after its
// window closes, turnout reaches the quorum floor (1/6 of the staked supply snapshotted at
// proposal time) AND the YES weight is a 2/3 supermajority of the weight cast.
//
// Nothing here moves funds or mints. The executors (treasury, bondvault) read Passed(id) and
// Subject(id) and act only on a proposal whose subject they can reproduce from their own
// parameters -- so no committee key is needed to authorise spending: a passed vote IS the
// authorisation, and anyone may execute it. Because every executor binds this ONE contract as
// its governance source, a look-alike contract cannot forge a "passed" result.
//
// Owner binds the dividends module once (write-once Configure) and is then renounced: after
// that the contract is keyless -- Propose and Vote are permissionless, the tallies are chain
// state, and Passed is pure arithmetic anyone can verify.
//
// Reference implementation. Internal audit lineage only. Verify before trusting value.
package ftbgov

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/ledger"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const (
	minVotingPeriod = 5760       // ~1 day @ 15s: the shortest a window may stay open
	maxVotingPeriod = 2102400    // ~1 year: the longest (matches the registry maxDelay ceiling)
	quorumDivisor   = 6          // turnout must reach 1/6 of the staked snapshot (the quorum floor)
	maxID           = 4294967296 // 2^32: idKey packs ids into 4 big-endian bytes (literal, not 1<<32)
)

var (
	keyOwner    = []byte{0x01} // binds dividends once, then renounced -> keyless
	keyDiv      = []byte{0x02} // the dividends module: staked-FTB vote-weight source
	keyCount    = []byte{0x03} // monotonic proposal counter (also the highest id)
	prefixSubj  = []byte{0x10} // + id -> 32-byte subject hash
	prefixClose = []byte{0x11} // + id -> voting-close block height
	prefixBase  = []byte{0x12} // + id -> quorum base: total staked snapshot at propose
	prefixYes   = []byte{0x13} // + id -> accumulated YES weight
	prefixNo    = []byte{0x14} // + id -> accumulated NO weight
	prefixVoted = []byte{0x15} // + id + voter -> 1 (one vote per address per proposal)
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

// Configure binds the dividends module (the staked-FTB electorate). Owner only, write-once.
func Configure(dividends interop.Hash160) {
	requireOwner()
	if len(dividends) != 20 {
		panic("configure: bad address")
	}
	ctx := storage.GetContext()
	if storage.Get(ctx, keyDiv) != nil {
		panic("configure: already configured (write-once)")
	}
	storage.Put(ctx, keyDiv, dividends)
	runtime.Notify("Configured", dividends)
}

// Propose opens a proposal to carry out the action committed by subject (a 32-byte hash the
// executor will reproduce from its own parameters). Permissionless: anyone may propose, but a
// proposal only does anything if it later PASSES. The quorum base is the staked supply snapshot
// taken now, so turnout is measured against the electorate that existed at proposal time.
func Propose(subject []byte, votingPeriod int) int {
	if len(subject) != 32 {
		panic("propose: subject must be a 32-byte hash")
	}
	if votingPeriod < minVotingPeriod || votingPeriod > maxVotingPeriod {
		panic("propose: voting period out of range")
	}
	base := totalStaked()
	if base <= 0 {
		panic("propose: no staked supply to form a quorum")
	}
	ctx := storage.GetContext()
	id := getInt(keyCount) + 1
	if id < 1 || id >= maxID {
		panic("propose: id space exhausted")
	}
	closeAt := int(ledger.CurrentIndex()) + votingPeriod
	storage.Put(ctx, idKey(prefixSubj, id), subject)
	storage.Put(ctx, idKey(prefixClose, id), closeAt)
	storage.Put(ctx, idKey(prefixBase, id), base)
	storage.Put(ctx, keyCount, id)
	runtime.Notify("Proposed", id, closeAt, base)
	return id
}

// Vote casts the caller's staked-FTB weight for or against a proposal while its window is
// open. One vote per address per proposal; weight is the voter's stake at the moment of voting
// (committed capital -- the dividends module locks it for minStakeBlocks).
func Vote(id int, voter interop.Hash160, support bool) {
	if len(voter) != 20 {
		panic("vote: bad voter")
	}
	if !runtime.CheckWitness(voter) {
		panic("vote: no witness for voter")
	}
	ctx := storage.GetContext()
	closeAt := getIntCtx(ctx, idKey(prefixClose, id))
	if closeAt == 0 {
		panic("vote: no such proposal")
	}
	if int(ledger.CurrentIndex()) >= closeAt {
		panic("vote: window closed")
	}
	vk := append(idKey(prefixVoted, id), voter...)
	if storage.Get(ctx, vk) != nil {
		panic("vote: already voted")
	}
	w := stakedOf(voter)
	if w <= 0 {
		panic("vote: no staked weight")
	}
	storage.Put(ctx, vk, 1)
	if support {
		yk := idKey(prefixYes, id)
		storage.Put(ctx, yk, getIntCtx(ctx, yk)+w)
	} else {
		nk := idKey(prefixNo, id)
		storage.Put(ctx, nk, getIntCtx(ctx, nk)+w)
	}
	runtime.Notify("Voted", id, voter, support, w)
}

// Passed reports whether a proposal has been decided in favour: window closed, turnout at or
// above the 1/6 quorum of the snapshot, and YES at or above a 2/3 supermajority of the weight
// cast. Pure arithmetic over chain state -- any node computes the same answer. False for an
// unknown proposal or one whose window is still open.
func Passed(id int) bool {
	ctx := storage.GetContext()
	closeAt := getIntCtx(ctx, idKey(prefixClose, id))
	if closeAt == 0 || int(ledger.CurrentIndex()) < closeAt {
		return false
	}
	yes := getIntCtx(ctx, idKey(prefixYes, id))
	no := getIntCtx(ctx, idKey(prefixNo, id))
	cast := yes + no
	if cast <= 0 {
		return false
	}
	if cast*quorumDivisor < getIntCtx(ctx, idKey(prefixBase, id)) {
		return false // quorum floor: turnout < base/6
	}
	if yes*3 < cast*2 {
		return false // supermajority: yes < 2/3 of cast
	}
	return true
}

// ---- views (safe) ----

func Subject(id int) []byte {
	v := storage.Get(storage.GetContext(), idKey(prefixSubj, id))
	if v == nil {
		return nil
	}
	return v.([]byte)
}
func Close(id int) int      { return getInt(idKey(prefixClose, id)) }
func Yes(id int) int        { return getInt(idKey(prefixYes, id)) }
func No(id int) int         { return getInt(idKey(prefixNo, id)) }
func QuorumBase(id int) int { return getInt(idKey(prefixBase, id)) }
func Count() int            { return getInt(keyCount) }

// Voted reports whether an address has already voted on a proposal.
func Voted(id int, voter interop.Hash160) bool {
	return storage.Get(storage.GetContext(), append(idKey(prefixVoted, id), voter...)) != nil
}

// Dividends exposes the bound staked-FTB electorate source.
func Dividends() interop.Hash160 {
	h := storage.Get(storage.GetContext(), keyDiv)
	if h == nil {
		return nil
	}
	return h.(interop.Hash160)
}

// RenounceOwnership drops the owner: Configure can never be called again, the dividends binding
// is frozen, and the contract is fully keyless. One-way door.
func RenounceOwnership() {
	requireOwner()
	storage.Put(storage.GetContext(), keyOwner, []byte{})
	runtime.Notify("OwnershipRenounced")
}

// ---- helpers ----

func idKey(prefix []byte, id int) []byte {
	if id < 1 || id >= maxID {
		panic("gov: id out of range")
	}
	k := append(prefix, byte((id>>24)&0xFF)) // #nosec G115 -- masked to one byte
	k = append(k, byte((id>>16)&0xFF))       // #nosec G115 -- masked to one byte
	k = append(k, byte((id>>8)&0xFF))        // #nosec G115 -- masked to one byte
	k = append(k, byte(id&0xFF))             // #nosec G115 -- masked to one byte
	return k
}

func divHash() interop.Hash160 {
	h := storage.Get(storage.GetContext(), keyDiv)
	if h == nil {
		panic("gov: not configured")
	}
	return h.(interop.Hash160)
}

func stakedOf(u interop.Hash160) int {
	return contract.Call(divHash(), "stakedOf", contract.ReadOnly, u).(int)
}

func totalStaked() int {
	return contract.Call(divHash(), "totalStaked", contract.ReadOnly).(int)
}

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil {
		panic("owner not set")
	}
	o := owner.(interop.Hash160)
	if len(o) != 20 {
		panic("only owner") // renounced: owner cleared, no witness possible
	}
	if !runtime.CheckWitness(o) {
		panic("only owner")
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
