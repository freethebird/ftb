// Package ftbtoken is the FTB (Free the Bird) token: a capped NEP-17 for Neo N3,
// written for the neo-go compiler.
//
// Invariants (the whole point of this contract):
//   - Supply never exceeds maxSupply.
//   - The ONLY way FTB is created is MintForMigration, callable ONLY by the Migrator,
//     ONLY while migration is open. After CloseMigration, supply can never grow again.
//   - No reward or staking logic lives here. Rewards come from PROVIDING liquidity in
//     other modules (swap LPs, lend suppliers). Idle FTB earns nothing. It never prints.
//
// MO: every power is explicit and emits an event; every input is validated; the
// irreversible acts (CloseMigration, RenounceOwnership) are one-way doors.
//
// Reference implementation. Internal audit lineage only - no external audit was
// performed (DECISIONS.md, Decision 7). Verify before trusting value.
package ftbtoken

import (
	"github.com/nspcc-dev/neo-go/pkg/interop"
	"github.com/nspcc-dev/neo-go/pkg/interop/contract"
	"github.com/nspcc-dev/neo-go/pkg/interop/native/management"
	"github.com/nspcc-dev/neo-go/pkg/interop/runtime"
	"github.com/nspcc-dev/neo-go/pkg/interop/storage"
)

const (
	symbol   = "FTB"
	decimals = 8
	one      = 100_000_000 // 1 FTB in base units
	// Audit item FTB-01: exact sum of all entitlements plus the dev-team allocation.
	// Tree: cmd/merkleroot -snapshot ftb_claim_list.csv -r1 1 -r2 1
	// (4,830 wallets, root 0x9b4be5c9...630cc8) printed 30,035,929,794,660,360;
	// dev allocation 7,336,189,899,000,000 (68,330,973.24 FLM-equiv + 5,030,925.75
	// FLOCKS at r1 = r2 = 1) is added on top — the committed cap, DECISIONS.md 16 Oct.
	// The dev wallet is still accumulating, so this is reset once more at MainNet launch.
	maxSupply = 37372119693660360
)

// Single-byte prefixes keep storage tight (MO: weight).
var (
	prefixBalance = []byte{0x01} // prefixBalance + account -> balance
	keySupply     = []byte{0x10} // total supply
	keyOwner      = []byte{0x11} // admin (multisig); renounceable
	keyMigrator   = []byte{0x12} // the only minter
	keyOpen       = []byte{0x13} // 1 while migration open, 0 once sealed forever
)

// _deploy sets the owner explicitly from deploy data (MO: visible, no hidden constant).
// On update, state is preserved.
//
//lint:ignore U1000 VM entry point, invoked by the chain via the manifest, not from Go
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
	storage.Put(ctx, keyOpen, 1)
	storage.Put(ctx, keySupply, 0)
}

// ---- NEP-17 ----

func Symbol() string { return symbol }
func Decimals() int  { return decimals }

// MaxSupply is the hard cap, exposed so anyone can verify on-chain that it equals the
// published snapshot total (the sum of all migration entitlements). Audit item FTB-01.
func MaxSupply() int { return maxSupply }

func TotalSupply() int {
	return getInt(keySupply)
}

func BalanceOf(account interop.Hash160) int {
	if len(account) != 20 {
		panic("balanceOf: bad account")
	}
	return getInt(balanceKey(account))
}

// Transfer moves FTB. A contract moving its own FTB passes CheckWitness on its own hash,
// which is how the lend and swap modules move pooled FTB.
func Transfer(from, to interop.Hash160, amount int, data any) bool {
	if len(from) != 20 || len(to) != 20 {
		panic("transfer: bad address")
	}
	if amount < 0 {
		panic("transfer: negative amount")
	}
	if !runtime.CheckWitness(from) {
		return false
	}
	ctx := storage.GetContext()
	fromKey := balanceKey(from)
	fromBal := getIntCtx(ctx, fromKey)
	if fromBal < amount {
		return false
	}
	if amount != 0 {
		if fromBal == amount {
			storage.Delete(ctx, fromKey)
		} else {
			storage.Put(ctx, fromKey, fromBal-amount)
		}
		toKey := balanceKey(to)
		storage.Put(ctx, toKey, getIntCtx(ctx, toKey)+amount)
	}
	runtime.Notify("Transfer", from, to, amount)
	if management.GetContract(to) != nil {
		contract.Call(to, "onNEP17Payment", contract.All, from, amount, data)
	}
	return true
}

// ---- the only mint path ----

// MintForMigration is callable ONLY by the registered Migrator, ONLY while open, ONLY
// under the cap. There is no other way to create FTB.
func MintForMigration(to interop.Hash160, amount int) {
	if len(to) != 20 {
		panic("mint: bad recipient")
	}
	if amount <= 0 {
		panic("mint: amount must be positive")
	}
	ctx := storage.GetContext()
	mig := storage.Get(ctx, keyMigrator)
	if mig == nil {
		panic("mint: migrator not set")
	}
	if string(runtime.GetCallingScriptHash()) != string(mig.(interop.Hash160)) {
		panic("mint: only migrator")
	}
	if getIntCtx(ctx, keyOpen) == 0 {
		panic("mint: migration closed")
	}
	supply := getIntCtx(ctx, keySupply)
	if supply+amount > maxSupply {
		panic("mint: exceeds cap") // the hard cap, enforced
	}
	storage.Put(ctx, keySupply, supply+amount)
	toKey := balanceKey(to)
	storage.Put(ctx, toKey, getIntCtx(ctx, toKey)+amount)
	// from == null signals a mint. The Hash160 conversion keeps the compiler's event
	// inference aligned with the manifest; at runtime a nil slice still emits as null.
	runtime.Notify("Transfer", interop.Hash160(nil), to, amount)
}

// CloseMigration permanently ends minting. One-way door. After this, no FTB can ever be
// created again, regardless of who holds any key.
func CloseMigration() {
	requireOwner()
	storage.Put(storage.GetContext(), keyOpen, 0)
	runtime.Notify("MigrationClosed")
}

func MigrationOpen() bool { return getInt(keyOpen) != 0 }

// ---- admin (all renounceable) ----

func SetMigrator(migrator interop.Hash160) {
	requireOwner()
	if len(migrator) != 20 {
		panic("setMigrator: bad address")
	}
	storage.Put(storage.GetContext(), keyMigrator, migrator)
	runtime.Notify("MigratorSet", migrator)
}

func GetMigrator() interop.Hash160 {
	mig := storage.Get(storage.GetContext(), keyMigrator)
	if mig == nil {
		return nil
	}
	return mig.(interop.Hash160)
}

// Update exists for pre-launch fixes only and MUST be renounced before value flows.
func Update(nef []byte, manifest []byte, data any) {
	requireOwner()
	management.UpdateWithData(nef, manifest, data)
}

// RenounceOwnership makes the token fully immutable: no more SetMigrator, no more Update,
// no more CloseMigration (migration should already be closed). One-way door, by design.
func RenounceOwnership() {
	requireOwner()
	// One-way doors in order: the mint must be sealed BEFORE the owner disappears, or
	// CloseMigration becomes permanently unreachable with minting still open. Audit item FTB-03.
	if getInt(keyOpen) != 0 {
		panic("renounce: close migration first")
	}
	storage.Put(storage.GetContext(), keyOwner, []byte{}) // empty owner -> requireOwner always fails
	runtime.Notify("OwnershipRenounced")
}

// ---- helpers ----

func requireOwner() {
	owner := storage.Get(storage.GetContext(), keyOwner)
	if owner == nil {
		panic("owner not set")
	}
	o := owner.(interop.Hash160)
	if len(o) != 20 || !runtime.CheckWitness(o) {
		panic("only owner")
	}
}

func balanceKey(account interop.Hash160) []byte {
	return append(prefixBalance, account...)
}

func getInt(key []byte) int {
	return getIntCtx(storage.GetContext(), key)
}

func getIntCtx(ctx storage.Context, key []byte) int {
	v := storage.Get(ctx, key)
	if v == nil {
		return 0
	}
	return v.(int)
}
