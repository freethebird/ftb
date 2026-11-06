// Reference neotest suite for the FTB token. Covers every invariant and every guarded
// path (MO: audit every step). Requires neo-go. Run:
//
//	go test ./contracts/ftb_token/
//
// Note: exact result expectations (stackitem shapes for void methods) can vary slightly
// by neo-go version; if a void Invoke mismatches, switch its expected value between
// stackitem.Null{} and stackitem.Make(true) to match your neo-go.
package ftbtoken_test

import (
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

// must equal the maxSupply constant in ftb_token.go (FTB-01: the committed 100M hard cap)
const maxSupply = 10000000000000000

// testWindowMs is a comfortably-future claim window (ms) for tests that keep migration open.
const testWindowMs = 15768000000

func deploy(t *testing.T) (*neotest.Executor, *neotest.ContractInvoker, neotest.Signer, neotest.Signer) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)

	owner := e.NewAccount(t) // the admin multisig stand-in
	other := e.NewAccount(t) // an unprivileged account

	// deploy the token; _deploy data = owner script hash (explicit owner, MO #1)
	c := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_token.go", "ftb_token.yml")
	e.DeployContract(t, c, owner.ScriptHash().BytesBE())

	tok := e.NewInvoker(c.Hash, owner)
	// stash `other` on the invoker chain by returning it
	_ = other
	return e, tok, owner, other
}

func TestMetadataAndInitialState(t *testing.T) {
	_, tok, _, _ := deploy(t)
	tok.Invoke(t, stackitem.Make("FTB"), "symbol")
	tok.Invoke(t, stackitem.Make(8), "decimals")
	tok.Invoke(t, stackitem.Make(0), "totalSupply")
	tok.Invoke(t, stackitem.Make(true), "migrationOpen")
}

// MaxSupply is exposed so the cap is verifiable on-chain (audit item FTB-01).
func TestMaxSupplyView(t *testing.T) {
	_, tok, _, _ := deploy(t)
	tok.Invoke(t, stackitem.Make(maxSupply), "maxSupply")
}

func TestOnlyOwnerCanSetMigrator(t *testing.T) {
	e, tok, _, other := deploy(t)
	otherInv := e.NewInvoker(tok.Hash, other)
	// unprivileged caller is rejected
	otherInv.InvokeFail(t, "only owner", "setMigrator", other.ScriptHash().BytesBE())
	// owner succeeds
	tok.Invoke(t, stackitem.Null{}, "setMigrator", other.ScriptHash().BytesBE())
}

func TestMintOnlyByMigrator(t *testing.T) {
	e, tok, owner, _ := deploy(t)

	// deploy the tiny caller and make IT the migrator
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())

	to := owner.ScriptHash()

	// a direct (non-migrator) mint attempt faults
	tok.InvokeFail(t, "only migrator", "mintForMigration", to.BytesBE(), 1000)

	// minting through the caller (the migrator) succeeds
	callerInv := e.NewInvoker(cc.Hash, owner)
	callerInv.Invoke(t, stackitem.Null{}, "mint", tok.Hash.BytesBE(), to.BytesBE(), 5_000)

	tok.Invoke(t, stackitem.Make(5_000), "totalSupply")
	tok.Invoke(t, stackitem.Make(5_000), "balanceOf", to.BytesBE())
}

func TestCapIsEnforced(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())

	callerInv := e.NewInvoker(cc.Hash, owner)
	// minting one unit over the cap must fault
	callerInv.InvokeFail(t, "exceeds cap", "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), maxSupply+1)
}

func TestCloseMigrationSealsMint(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())

	// close migration (owner), then any further mint must fault forever
	tok.Invoke(t, stackitem.Null{}, "closeMigration")
	tok.Invoke(t, stackitem.Make(false), "migrationOpen")

	callerInv := e.NewInvoker(cc.Hash, owner)
	callerInv.InvokeFail(t, "migration closed", "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), 1)
}

func TestTransferRequiresWitnessAndMovesBalance(t *testing.T) {
	e, tok, owner, other := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())

	// seed owner with FTB via the migrator
	callerInv := e.NewInvoker(cc.Hash, owner)
	callerInv.Invoke(t, stackitem.Null{}, "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), 1_000)

	// owner transfers to `other`
	tok.Invoke(t, stackitem.Make(true), "transfer", owner.ScriptHash().BytesBE(), other.ScriptHash().BytesBE(), 400, nil)
	tok.Invoke(t, stackitem.Make(600), "balanceOf", owner.ScriptHash().BytesBE())
	tok.Invoke(t, stackitem.Make(400), "balanceOf", other.ScriptHash().BytesBE())

	// a transfer FROM owner signed by `other` (no owner witness) returns false, moves nothing
	otherInv := e.NewInvoker(tok.Hash, other)
	otherInv.Invoke(t, stackitem.Make(false), "transfer", owner.ScriptHash().BytesBE(), other.ScriptHash().BytesBE(), 100, nil)
	tok.Invoke(t, stackitem.Make(600), "balanceOf", owner.ScriptHash().BytesBE())
}

func TestRenounceRequiresCloseThenIsFinal(t *testing.T) {
	e, tok, _, _ := deploy(t)
	// one-way doors in order: renounce is blocked until the mint is sealed (audit item FTB-03)
	tok.InvokeFail(t, "close migration first", "renounceOwnership")
	// seal the mint, then renounce succeeds
	tok.Invoke(t, stackitem.Null{}, "closeMigration")
	tok.Invoke(t, stackitem.Null{}, "renounceOwnership")
	// after renounce, no admin action works, ever
	tok.InvokeFail(t, "only owner", "setMigrator", tok.Hash.BytesBE())
	_ = e
}

// The claimed ledger lives in the TOKEN (v2): once an account has minted, it can never
// mint again — not through the same migrator, and not through a replacement migrator.
// This is the exact R2 + R3 double-claim path that inflated TestNet supply by ~20.16M.
func TestClaimedLedgerBlocksDoubleMint(t *testing.T) {
	e, tok, owner, other := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())

	callerInv := e.NewInvoker(cc.Hash, owner)
	to := other.ScriptHash()

	// before any mint: not claimed, zero amount
	tok.Invoke(t, stackitem.Make(false), "claimed", to.BytesBE())
	tok.Invoke(t, stackitem.Make(0), "claimedAmount", to.BytesBE())

	// first mint marks the ledger with the minted amount
	callerInv.Invoke(t, stackitem.Null{}, "mint", tok.Hash.BytesBE(), to.BytesBE(), 5_000)
	tok.Invoke(t, stackitem.Make(true), "claimed", to.BytesBE())
	tok.Invoke(t, stackitem.Make(5_000), "claimedAmount", to.BytesBE())

	// a second mint to the SAME account faults, even from the same trusted migrator
	callerInv.InvokeFail(t, "already claimed", "mint", tok.Hash.BytesBE(), to.BytesBE(), 5_000)
	// the ledger blocked it BEFORE the cap check: supply did not move
	tok.Invoke(t, stackitem.Make(5_000), "totalSupply")

	// ...while a DIFFERENT account still mints fine (the ledger is per-account)
	callerInv.Invoke(t, stackitem.Null{}, "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), 7_000)
	tok.Invoke(t, stackitem.Make(12_000), "totalSupply")
}

// H1 remediation: once minting has begun, the migrator is frozen. Re-pointing it while
// supply exists is the rogue-swap-under-a-new-root headroom vector (the per-account
// claimed ledger only blocks a re-mint to the SAME address, not fresh addresses under a
// new root). The token now refuses setMigrator after the first mint. The per-account
// double-mint protection remains independently covered by TestClaimedLedgerBlocksDoubleMint.
func TestSetMigratorFrozenAfterFirstMint(t *testing.T) {
	e, tok, owner, other := deploy(t)
	cc1 := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc1, nil)
	cc2 := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller2/caller2.go", "testdata/caller2/caller2.yml")
	e.DeployContract(t, cc2, nil)

	to := other.ScriptHash()
	// migrator A is set before any mint (allowed), then mints -> supply becomes > 0
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc1.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())
	e.NewInvoker(cc1.Hash, owner).Invoke(t, stackitem.Null{}, "mint", tok.Hash.BytesBE(), to.BytesBE(), 5_000)

	// the owner can no longer swap in a different migrator: frozen after first mint
	tok.InvokeFail(t, "locked after first mint", "setMigrator", cc2.Hash.BytesBE())
	// supply unchanged; the rogue-root headroom path is unreachable
	tok.Invoke(t, stackitem.Make(5_000), "totalSupply")
}

// --- claim-window deadline: self-sealing migration + day-one renounce ---
// The FTB claim-window vote ratified 6 months. The deadline is wall-clock (SetClaimWindow),
// enforced in the token; the migrator stays byte-for-byte. These prove: mint is fail-closed
// until the window is set; the window is owner-only + write-once; the deadline seals minting
// with no transaction; the owner may renounce on day one with claims still open; and after
// the deadline anyone may flip the closed flag.

func TestMintRefusedUntilClaimWindowSet(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	callerInv := e.NewInvoker(cc.Hash, owner)
	// no setClaimWindow yet -> the mint is fail-closed
	callerInv.InvokeFail(t, "claim window not set", "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), 1_000)
}

func TestSetClaimWindowOwnerOnlyAndWriteOnce(t *testing.T) {
	e, tok, _, other := deploy(t)
	otherInv := e.NewInvoker(tok.Hash, other)
	otherInv.InvokeFail(t, "only owner", "setClaimWindow", testWindowMs, other.ScriptHash().BytesBE())
	tok.InvokeFail(t, "window must be positive", "setClaimWindow", 0, other.ScriptHash().BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, other.ScriptHash().BytesBE())
	tok.InvokeFail(t, "already set", "setClaimWindow", testWindowMs, other.ScriptHash().BytesBE())
	tok.Invoke(t, stackitem.NewBuffer(other.ScriptHash().BytesBE()), "claimWindowProposal")
}

func TestClaimOpenBeforeDeadline(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())
	tok.Invoke(t, stackitem.Make(true), "migrationOpen")
	callerInv := e.NewInvoker(cc.Hash, owner)
	callerInv.Invoke(t, stackitem.Null{}, "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), 5_000)
	tok.Invoke(t, stackitem.Make(5_000), "totalSupply")
}

func TestClaimWindowSealsMintAtDeadline(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", 1, owner.ScriptHash().BytesBE())
	e.GenerateNewBlocks(t, 2) // move wall-clock past the 1 ms deadline, no close call
	tok.Invoke(t, stackitem.Make(false), "migrationOpen")
	callerInv := e.NewInvoker(cc.Hash, owner)
	callerInv.InvokeFail(t, "claim window closed", "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), 1_000)
}

func TestDayOneRenounceWithDeadlineSet(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())
	// migration is OPEN (deadline in the future) yet renounce still succeeds: the deadline
	// seals the mint on its own, so no key need be held to the end of the window.
	tok.Invoke(t, stackitem.Make(true), "migrationOpen")
	tok.Invoke(t, stackitem.Null{}, "renounceOwnership")
	tok.InvokeFail(t, "only owner", "setClaimWindow", testWindowMs, owner.ScriptHash().BytesBE())
}

func TestCloseAfterDeadlineIsPermissionless(t *testing.T) {
	e, tok, owner, other := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", 1, owner.ScriptHash().BytesBE())
	e.GenerateNewBlocks(t, 2) // past the deadline
	otherInv := e.NewInvoker(tok.Hash, other)
	otherInv.Invoke(t, stackitem.Null{}, "closeMigration")
	tok.Invoke(t, stackitem.Make(false), "migrationOpen")
}

func TestCloseBeforeDeadlineIsOwnerOnly(t *testing.T) {
	e, tok, _, other := deploy(t)
	tok.Invoke(t, stackitem.Null{}, "setClaimWindow", testWindowMs, other.ScriptHash().BytesBE())
	otherInv := e.NewInvoker(tok.Hash, other)
	otherInv.InvokeFail(t, "only owner", "closeMigration")
}
