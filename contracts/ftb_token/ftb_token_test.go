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

// must equal the maxSupply constant in ftb_token.go (FTB-01: merkleroot total + dev allocation)
const maxSupply = 37372119693660360

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

	callerInv := e.NewInvoker(cc.Hash, owner)
	// minting one unit over the cap must fault
	callerInv.InvokeFail(t, "exceeds cap", "mint", tok.Hash.BytesBE(), owner.ScriptHash().BytesBE(), maxSupply+1)
}

func TestCloseMigrationSealsMint(t *testing.T) {
	e, tok, owner, _ := deploy(t)
	cc := neotest.CompileFile(t, e.Validator.ScriptHash(), "testdata/caller/caller.go", "testdata/caller/caller.yml")
	e.DeployContract(t, cc, nil)
	tok.Invoke(t, stackitem.Null{}, "setMigrator", cc.Hash.BytesBE())

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
