// Package ftbtoken is the FTB (Free the Bird) token: a capped NEP-17 for Neo N3,
// written for the neo-go compiler.
//
// Invariants (the whole point of this contract):
//   - Supply never exceeds maxSupply.
//   - The ONLY way FTB is created is MintForMigration, callable ONLY by the Migrator,
//     ONLY while migration is open. After CloseMigration, supply can never grow again.
//   - The claimed ledger lives HERE, not in the migrator (v2, after the R2+R3 live
//     double-claim): MintForMigration records every claimant and panics on a repeat,
//     so replacing or re-pointing the migrator can NEVER reopen a claim. The migrator
//     is a stateless proof-checker; the token alone decides who has claimed, forever.
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
	// Audit item FTB-01: the committed hard cap, DECISIONS.md 18 Oct — 100,000,000 FTB
	// flat, the governance ratio r = 100,000,000 ÷ 577,458,329.20225836 = 0.173173 FTB per
	// FLM/FLOCKS applied at claim time by the migrator (the tree stays in FLM units,
	// so 1 FLM of snapshot = 1 vote). Claims draw from the v4 snapshot tree (5,233
	// wallets) at r; the rest of the cap is unclaimable headroom, never printed.
	maxSupply = 10000000000000000
)

// Single-byte prefixes keep storage tight (MO: weight).
var (
	prefixBalance = []byte{0x01} // prefixBalance + account -> balance
	keySupply     = []byte{0x10} // total supply
	keyOwner      = []byte{0x11} // admin (multisig); renounceable
	keyMigrator   = []byte{0x12} // the only minter
	keyOpen       = []byte{0x13} // 1 while migration open, 0 once sealed forever
	prefixClaimed = []byte{0x14} // prefixClaimed + account -> FTB amount claimed (once, ever)
	keyDeadline   = []byte{0x15} // unix ms; minting forbidden at/after this instant (0 = unset)
	keyClaimProp  = []byte{0x16} // authorizing claim-window proposal hash (governance provenance)
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
// under the cap, and ONLY ONCE per recipient — ever. The claimed ledger is written
// here, in the token, so no migrator replacement (R2 -> R3 taught us why) can ever
// mint twice to the same account. There is no other way to create FTB.
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
	// Claim window: a wall-clock deadline fixed once at SetClaimWindow and never moved.
	// Minting ends by itself when the deadline passes - no owner action, no transaction.
	deadline := getIntCtx(ctx, keyDeadline)
	if deadline == 0 {
		panic("mint: claim window not set")
	}
	if runtime.GetTime() >= deadline {
		panic("mint: claim window closed")
	}
	claimedKey := append(prefixClaimed, to...)
	if storage.Get(ctx, claimedKey) != nil {
		panic("mint: already claimed") // once per account, across all migrators, forever
	}
	supply := getIntCtx(ctx, keySupply)
	if supply+amount > maxSupply {
		panic("mint: exceeds cap") // the hard cap, enforced
	}
	storage.Put(ctx, claimedKey, amount)
	storage.Put(ctx, keySupply, supply+amount)
	toKey := balanceKey(to)
	storage.Put(ctx, toKey, getIntCtx(ctx, toKey)+amount)
	// from == null signals a mint. The Hash160 conversion keeps the compiler's event
	// inference aligned with the manifest; at runtime a nil slice still emits as null.
	runtime.Notify("Transfer", interop.Hash160(nil), to, amount)
}

// Claimed reports whether an account has ever received a migration mint, from ANY
// migrator. This is the canonical claim status — the migrator's view is a passthrough.
func Claimed(account interop.Hash160) bool {
	if len(account) != 20 {
		panic("claimed: bad account")
	}
	return storage.Get(storage.GetContext(), append(prefixClaimed, account...)) != nil
}

// ClaimedAmount returns how much FTB an account's migration claim minted (0 if none),
// so the claim ledger is auditable on-chain, not just a flag.
func ClaimedAmount(account interop.Hash160) int {
	if len(account) != 20 {
		panic("claimedAmount: bad account")
	}
	return getInt(append(prefixClaimed, account...))
}

// CloseMigration permanently ends minting. One-way door. After this, no FTB can ever be
// created again, regardless of who holds any key.
//
// Before the deadline, only the owner may seal early. At or after the deadline, anyone may
// call it: minting is already dead by then, so this only flips the visible flag and emits
// the event - there is no owner left to do it after the day-one renounce.
func CloseMigration() {
	ctx := storage.GetContext()
	deadline := getIntCtx(ctx, keyDeadline)
	if deadline == 0 || runtime.GetTime() < deadline {
		requireOwner()
	}
	storage.Put(ctx, keyOpen, 0)
	runtime.Notify("MigrationClosed")
}

// MigrationOpen reports whether a claim can still be minted right now: the flag is up and,
// once a deadline is set, it has not passed. It flips to false at the deadline on its own,
// before anyone calls CloseMigration.
func MigrationOpen() bool {
	ctx := storage.GetContext()
	if getIntCtx(ctx, keyOpen) == 0 {
		return false
	}
	deadline := getIntCtx(ctx, keyDeadline)
	return deadline == 0 || runtime.GetTime() < deadline
}

// Deadline is the wall-clock instant (unix ms) at which the claim window seals, or 0 if not
// yet set. At or after this instant MintForMigration refuses and MigrationOpen is false.
func Deadline() int { return getInt(keyDeadline) }

// ClaimWindowProposal is the governance proposal that authorized the claim window (its
// 20-byte hash), so the authority behind the deadline is checkable on-chain. nil if unset.
func ClaimWindowProposal() interop.Hash160 {
	p := storage.Get(storage.GetContext(), keyClaimProp)
	if p == nil {
		return nil
	}
	return p.(interop.Hash160)
}

// ---- admin (all renounceable) ----

func SetMigrator(migrator interop.Hash160) {
	requireOwner()
	if len(migrator) != 20 {
		panic("setMigrator: bad address")
	}
	ctx := storage.GetContext()
	// H1 remediation: once minting has begun, the migrator is frozen here. Re-pointing it
	// while supply exists would let a new merkle root mint the cap headroom to fresh
	// addresses (the once-per-account ledger only blocks a re-mint to the SAME address).
	// Set or fix the migrator before the first claim; after that it is immutable.
	if getIntCtx(ctx, keySupply) > 0 {
		panic("setMigrator: locked after first mint")
	}
	storage.Put(ctx, keyMigrator, migrator)
	runtime.Notify("MigratorSet", migrator)
}

// SetClaimWindow fixes the migration wall-clock deadline and records the governance
// proposal that authorized it. Owner only, write-once: it is set during wiring, before the
// first claim, and can never be moved afterwards. The deadline is now + windowMs, stamped
// relative to this call block time.
func SetClaimWindow(windowMs int, proposal interop.Hash160) {
	requireOwner()
	ctx := storage.GetContext()
	if getIntCtx(ctx, keyDeadline) != 0 {
		panic("claimWindow: already set (write-once)")
	}
	if windowMs <= 0 {
		panic("claimWindow: window must be positive")
	}
	if len(proposal) != 20 {
		panic("claimWindow: proposal must be a 20-byte hash")
	}
	deadline := runtime.GetTime() + windowMs
	storage.Put(ctx, keyDeadline, deadline)
	storage.Put(ctx, keyClaimProp, proposal)
	runtime.Notify("ClaimWindowSet", deadline, proposal)
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
	// L1 remediation: the most dangerous power emits a named, observable event
	// (the native ContractManagement notification alone is easy to miss).
	runtime.Notify("Updated")
	management.UpdateWithData(nef, manifest, data)
}

// RenounceOwnership makes the token fully immutable: no more SetMigrator, no more Update,
// no more owner CloseMigration. One-way door, by design - the keys are destroyed.
func RenounceOwnership() {
	requireOwner()
	// FTB-03 revisited: the original rule required CloseMigration BEFORE renounce so the
	// owner could not vanish while minting stayed reachable. A claim-window deadline makes
	// that automatic - minting ends by itself at the deadline with no owner - so once a
	// deadline is set the owner may step down at once (day-one renounce) and the chain seals
	// on its own. Refuse only if neither the deadline is set NOR migration is closed, which
	// would leave minting reachable with no way to ever stop it.
	if getInt(keyOpen) != 0 && getInt(keyDeadline) == 0 {
		panic("renounce: set the claim window or close migration first")
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
