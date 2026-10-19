// Package ftbmigrator is the FTB Migrator: FLM + FLOCKS holders claim FTB once, from a
// published pre-exploit snapshot, proven by a Merkle proof. The attacker is not in the
// snapshot, so it cannot claim. This is the ONLY thing allowed to mint FTB.
//
// R4: this contract is now a STATELESS proof-checker. It holds no claim ledger of its
// own — the claimed ledger lives inside the token (the R2+R3 double-claim proved that a
// ledger the migrator owns dies with the migrator: deploy a new one and every claim
// reopens). Here we verify the proof and ask the token to mint; the token alone decides
// who has claimed, across all migrators, forever. Church and state, separated.
//
// Leaves are denominated in FLM-equivalent base units (8 decimals). The governance
// ratio r (DECISIONS.md 18 Oct: 1 FLM/FLOCKS = 0.189832 FTB, i.e. the 100,000,000 FTB
// hard cap ÷ 526,782,179.10 total FLM-equiv supply) is applied AT MINT TIME:
// minted = floor(amount × ratioNum / ratioDen). Keeping the tree in FLM units means
// the same proof also carries the governance vote at exactly 1 FLM = 1 vote.
//
// Leaf format (MUST match the off-chain builder in pkg/merkle EXACTLY):
//
//	leaf = sha256( 0x00 ++ accountBytes(20) ++ ":" ++ decimal(amount) )
//	node = sha256( 0x01 ++ min ++ max )   sorted-pair, domain-separated (FTB-11)
//
// Sibling hashing is sorted-pair (smaller 32-byte hash first), odd nodes carry up.
//
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package ftbmigrator

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/crypto"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/std"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

var (
	keyOwner = []byte{0x01}
	keyFTB   = []byte{0x02}
	keyRoot  = []byte{0x03}
	// no claim ledger here (R4): claim status lives in the token - see Claimed below.
)

const (
	// The governance ratio r in exact integer form (DECISIONS.md 18 Oct):
	// 100,000,000 FTB (in base units) over 526,782,179.10 FLM-equiv (in base units).
	// floor semantics: the last base unit always rounds down, so the cap can never
	// be exceeded by rounding.
	ratioNum = 10_000_000_000_000_000 // 100M FTB × 10^8
	ratioDen = 52_678_217_910_000_000 // 526,782,179.10 × 10^8
)

// RatioNum and RatioDen expose the committed conversion ratio so anyone can verify
// on-chain that claims mint at the published r = 0.189832 (audit item FTB-01).
func RatioNum() int { return ratioNum }
func RatioDen() int { return ratioDen }

// Quote returns how much FTB a snapshot entitlement of `amount` FLM-equivalent base
// units mints: floor(amount × r). The front-end previews claims with exactly this.
func Quote(amount int) int {
	if amount <= 0 {
		return 0
	}
	return amount * ratioNum / ratioDen
}

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

// Configure wires the FTB token and publishes the snapshot Merkle root. Owner only.
func Configure(ftb interop.Hash160, merkleRoot []byte) {
	requireOwner()
	if len(ftb) != 20 {
		panic("configure: bad ftb hash")
	}
	if len(merkleRoot) != 32 {
		panic("configure: root must be 32 bytes")
	}
	ctx := storage.GetContext()
	// Write-once: the published snapshot root is bound at configure time and cannot be
	// silently swapped afterward. To change it before launch, redeploy and re-point the
	// token's migrator. Audit item FTB-02.
	if storage.Get(ctx, keyRoot) != nil {
		panic("configure: already configured (write-once)")
	}
	storage.Put(ctx, keyFTB, ftb)
	storage.Put(ctx, keyRoot, merkleRoot)
	runtime.Notify("Configured", ftb, merkleRoot)
}

// Claimed is a read-only passthrough to the token's canonical claim ledger. Before
// Configure there is no token, so nothing has been claimed.
func Claimed(account interop.Hash160) bool {
	if len(account) != 20 {
		panic("claimed: bad account")
	}
	ftb := storage.Get(storage.GetContext(), keyFTB)
	if ftb == nil {
		return false
	}
	return contract.Call(ftb.(interop.Hash160), "claimed", contract.ReadOnly, account).(bool)
}

// Claim verifies the caller's (account, amount) entitlement against the published root
// and mints floor(amount × r) FTB to the account — `amount` is the snapshot entitlement
// in FLM-equivalent base units, r the committed ratio. The token's claimed ledger makes
// the mint once-per-account across all migrators; a repeat claim faults there.
func Claim(account interop.Hash160, amount int, proof [][]byte) {
	if len(account) != 20 {
		panic("claim: bad account")
	}
	if amount <= 0 {
		panic("claim: amount must be positive")
	}
	if !runtime.CheckWitness(account) {
		panic("claim: no witness for account")
	}
	ctx := storage.GetContext()

	// leaf = sha256( 0x00 ++ account ++ ":" ++ itoa(amount) ); domain-separated (FTB-11)
	leafInput := []byte{0x00}
	leafInput = append(leafInput, []byte(account)...)
	leafInput = append(leafInput, ':')
	leafInput = append(leafInput, []byte(std.Itoa(amount, 10))...)
	node := []byte(crypto.Sha256(leafInput))

	for i := 0; i < len(proof); i++ {
		sib := proof[i]
		if len(sib) != 32 {
			panic("claim: bad proof element")
		}
		// internal nodes are tagged 0x01 so a leaf can never be reinterpreted as a node
		if lessBytes(node, sib) {
			node = []byte(crypto.Sha256(append([]byte{0x01}, append(node, sib...)...)))
		} else {
			node = []byte(crypto.Sha256(append([]byte{0x01}, append(sib, node...)...)))
		}
	}

	root := storage.Get(ctx, keyRoot).([]byte)
	if string(node) != string(root) {
		panic("claim: bad proof")
	}

	mint := Quote(amount)
	if mint <= 0 {
		panic("claim: entitlement converts to zero FTB")
	}
	ftb := storage.Get(ctx, keyFTB).(interop.Hash160)
	contract.Call(ftb, "mintForMigration", contract.All, account, mint)
	runtime.Notify("Claimed", account, amount, mint)
}

// Close seals the token's mint forever, ending the migration. Owner only.
func Close() {
	requireOwner()
	ftb := storage.Get(storage.GetContext(), keyFTB).(interop.Hash160)
	contract.Call(ftb, "closeMigration", contract.All)
	runtime.Notify("Closed")
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

// lessBytes compares two 32-byte hashes as unsigned big-endian, matching bytes.Compare
// in the off-chain builder (sorted-pair order).
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
