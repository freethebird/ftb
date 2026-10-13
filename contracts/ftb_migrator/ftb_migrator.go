// Package ftbmigrator is the FTB Migrator: FLM + FLOCKS holders claim FTB once, from a
// published pre-exploit snapshot, proven by a Merkle proof. The attacker is not in the
// snapshot, so it cannot claim. This is the ONLY thing allowed to mint FTB.
//
// Leaf format (MUST match the off-chain builder in pkg/merkle EXACTLY):
//
//	leaf = sha256( 0x00 ++ accountBytes(20) ++ ":" ++ decimal(amount) )
//	node = sha256( 0x01 ++ min ++ max )   sorted-pair, domain-separated (FTB-11)
//
// Sibling hashing is sorted-pair (smaller 32-byte hash first), odd nodes carry up.
//
// Reference implementation pending audit.
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
	keyOwner   = []byte{0x01}
	keyFTB     = []byte{0x02}
	keyRoot    = []byte{0x03}
	prefixDone = []byte{0x10}
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

func Claimed(account interop.Hash160) bool {
	return storage.Get(storage.GetContext(), append(prefixDone, account...)) != nil
}

// Claim verifies the caller's (account, amount) entitlement against the published root,
// marks it claimed once, and mints exactly that amount of FTB to the account.
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
	doneKey := append(prefixDone, account...)
	if storage.Get(ctx, doneKey) != nil {
		panic("claim: already claimed")
	}

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

	storage.Put(ctx, doneKey, 1)
	ftb := storage.Get(ctx, keyFTB).(interop.Hash160)
	contract.Call(ftb, "mintForMigration", contract.All, account, amount)
	runtime.Notify("Claimed", account, amount)
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
