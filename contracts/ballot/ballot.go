// Package ballot is FTB governance: a snapshot-weighted, one-vote-per-address ballot that
// reuses the MIGRATION Merkle tree. A voter proves their pre-exploit entitlement with the
// exact same proof they use to claim FTB (same leaf format, same root), so voting weight is
// the published snapshot — not current balances, which the attacker diluted.
//
// The tally is chain state: every full node replicates and verifies it, so "counting the
// vote" is not a service anyone runs — it is something every node can read. That is what
// makes this governance between nodes instead of governance by whoever runs the tally.
//
// No funds move here. No mint. One ballot per proposal: a new question is a new deployment,
// and the published contract hash IS the proposal's identity.
//
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package ballot

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/crypto"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/std"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	keyOwner    = []byte{0x01}
	keyRoot     = []byte{0x02} // the migration snapshot root (same as the migrator's)
	keyNumOpts  = []byte{0x03} // number of options
	prefixTally = []byte{0x10} // prefixTally + option byte -> accumulated weight
	prefixVoted = []byte{0x11} // prefixVoted + account -> option chosen (one vote, ever)
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

// Configure binds the migration snapshot root and the option count. Owner only, write-once:
// a ballot's question and electorate must never change after publication.
func Configure(merkleRoot []byte, numOptions int) {
	requireOwner()
	ctx := storage.GetContext()
	if storage.Get(ctx, keyRoot) != nil {
		panic("configure: already configured (write-once)")
	}
	if len(merkleRoot) != 32 {
		panic("configure: root must be 32 bytes")
	}
	if numOptions < 2 || numOptions > 16 {
		panic("configure: 2 to 16 options")
	}
	storage.Put(ctx, keyRoot, merkleRoot)
	storage.Put(ctx, keyNumOpts, numOptions)
	runtime.Notify("Configured", merkleRoot, numOptions)
}

// Vote casts the caller's snapshot weight behind one option. The proof is the voter's
// migration proof: leaf = sha256(0x00 || account || ":" || decimal(amount)), nodes are
// sha256(0x01 || min || max), sorted-pair, identical to ftb_migrator.Claim.
func Vote(account interop.Hash160, amount int, option int, proof [][]byte) {
	if len(account) != 20 {
		panic("vote: bad account")
	}
	if amount <= 0 {
		panic("vote: weight must be positive")
	}
	ctx := storage.GetContext()
	n := getIntCtx(ctx, keyNumOpts)
	if n == 0 {
		panic("vote: not configured")
	}
	if option < 1 || option > n {
		panic("vote: no such option")
	}
	if !runtime.CheckWitness(account) {
		panic("vote: no witness for account")
	}
	votedKey := append(prefixVoted, account...)
	if storage.Get(ctx, votedKey) != nil {
		panic("vote: already voted")
	}

	leafInput := []byte{0x00}
	leafInput = append(leafInput, []byte(account)...)
	leafInput = append(leafInput, ':')
	leafInput = append(leafInput, []byte(std.Itoa(amount, 10))...)
	node := []byte(crypto.Sha256(leafInput))

	for i := 0; i < len(proof); i++ {
		sib := proof[i]
		if len(sib) != 32 {
			panic("vote: bad proof element")
		}
		if lessBytes(node, sib) {
			node = []byte(crypto.Sha256(append([]byte{0x01}, append(node, sib...)...)))
		} else {
			node = []byte(crypto.Sha256(append([]byte{0x01}, append(sib, node...)...)))
		}
	}

	root := storage.Get(ctx, keyRoot).([]byte)
	if string(node) != string(root) {
		panic("vote: bad proof")
	}

	storage.Put(ctx, votedKey, option)
	tk := append(prefixTally, byte(option)) // #nosec G115 -- option is bounded to 1..numOptions above, and Configure caps numOptions at 16
	storage.Put(ctx, tk, getIntCtx(ctx, tk)+amount)
	runtime.Notify("Voted", account, option, amount)
}

// ---- views (safe) ----

// Tally returns the total snapshot weight behind an option (1-based). Out-of-range ids are
// rejected rather than silently aliased: the tally key is one byte, so an unbounded option
// would read another option's tally (found by gosec G115, FTB-31).
func Tally(option int) int {
	if option < 1 || option > 16 { // the Configure ceiling; tally keys are byte-wide
		panic("tally: no such option")
	}
	return getInt(append(prefixTally, byte(option))) // #nosec G115 -- bounded to 1..16 above
}

// Voted returns the option an address chose, or 0 if it has not voted.
func Voted(account interop.Hash160) int {
	return getInt(append(prefixVoted, account...))
}

func NumOptions() int { return getInt(keyNumOpts) }

// Root exposes the bound snapshot root so anyone can check it against the published one.
func Root() []byte {
	r := storage.Get(storage.GetContext(), keyRoot)
	if r == nil {
		return nil
	}
	return r.([]byte)
}

// ---- helpers ----

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil {
		panic("owner not set")
	}
	if !runtime.CheckWitness(owner.(interop.Hash160)) {
		panic("only owner")
	}
}

// lessBytes compares two 32-byte hashes as unsigned big-endian, matching bytes.Compare
// in the off-chain builder (sorted-pair order). Identical to the migrator's.
func lessBytes(a, b []byte) bool {
	for i := 0; i < 32; i++ {
		ai := int(a[i]) & 0xff
		bi := int(b[i]) & 0xff
		if ai != bi {
			return ai < bi
		}
	}
	return false
}

func getInt(key []byte) int { return getIntCtx(storage.GetContext(), key) }
func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
