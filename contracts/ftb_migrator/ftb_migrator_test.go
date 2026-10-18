// Reference neotest suite for the Migrator. Critically, it builds the Merkle tree with
// pkg/merkle (the same code the CLI uses) and then claims on-chain, so a PASS proves the
// off-chain leaf format and the on-chain Claim agree byte for byte.
//
// If TestClaimFlow fails with "bad proof", the account byte order is the single thing to
// fix: flip BytesBE to BytesLE in pkg/merkle.accountBytes (one place, everything follows).
//
// Run: go test ./contracts/ftb_migrator/
package ftbmigrator_test

import (
	"math/big"
	"testing"

	"github.com/freethebird/ftb/pkg/merkle"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func TestClaimFlow(t *testing.T) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)

	// deploy token and migrator (owner passed explicitly as deploy data)
	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	tok := e.NewInvoker(tokC.Hash, owner)

	migC := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_migrator.go", "ftb_migrator.yml")
	e.DeployContract(t, migC, owner.ScriptHash().BytesBE())
	mig := e.NewInvoker(migC.Hash, owner)

	// token trusts the migrator as its only minter
	tok.Invoke(t, stackitem.Null{}, "setMigrator", migC.Hash.BytesBE())

	// snapshot of two holders, built with the SAME pkg/merkle the CLI uses
	a1 := e.NewAccount(t)
	a2 := e.NewAccount(t)
	amt1 := big.NewInt(1000)
	amt2 := big.NewInt(2500)
	leaves := [][]byte{
		merkle.Leaf(a1.ScriptHash(), amt1),
		merkle.Leaf(a2.ScriptHash(), amt2),
	}
	root, proofs := merkle.Build(leaves)
	// the invoker marshaler takes []any for nested arrays, not [][]byte
	proofArg := func(p [][]byte) []any {
		a := make([]any, len(p))
		for i, s := range p {
			a[i] = s
		}
		return a
	}

	mig.Invoke(t, stackitem.Null{}, "configure", tokC.Hash.BytesBE(), root)

	// the committed ratio is visible on-chain (FTB-01): r = 100M FTB ÷ 526,782,179.10 FLM
	mig.Invoke(t, stackitem.Make(10000000000000000), "ratioNum")
	mig.Invoke(t, stackitem.Make(52678217910000000), "ratioDen")
	// quote(1000) = floor(1000 × r) = 189; quote(2500) = floor(2500 × r) = 474
	mig.Invoke(t, stackitem.Make(189), "quote", 1000)
	mig.Invoke(t, stackitem.Make(474), "quote", 2500)
	mig.Invoke(t, stackitem.Make(0), "quote", 0)

	// a1 claims its exact entitlement; FTB minted is floor(amount × r), not the face value
	mig1 := e.NewInvoker(migC.Hash, a1)
	mig1.Invoke(t, stackitem.Null{}, "claim", a1.ScriptHash(), 1000, proofArg(proofs[0]))
	tok.Invoke(t, stackitem.Make(189), "balanceOf", a1.ScriptHash().BytesBE())
	mig.Invoke(t, stackitem.Make(true), "claimed", a1.ScriptHash())

	// no double claim
	mig1.InvokeFail(t, "already claimed", "claim", a1.ScriptHash(), 1000, proofArg(proofs[0]))

	// a wrong amount does not verify
	mig2 := e.NewInvoker(migC.Hash, a2)
	mig2.InvokeFail(t, "bad proof", "claim", a2.ScriptHash(), 9999, proofArg(proofs[1]))

	// a2 claims correctly; supply grows by the converted amount only
	mig2.Invoke(t, stackitem.Null{}, "claim", a2.ScriptHash(), 2500, proofArg(proofs[1]))
	tok.Invoke(t, stackitem.Make(189+474), "totalSupply")

	// someone not in the snapshot cannot claim
	a3 := e.NewAccount(t)
	mig3 := e.NewInvoker(migC.Hash, a3)
	mig3.InvokeFail(t, "bad proof", "claim", a3.ScriptHash(), 1, proofArg(proofs[0]))

	// closing seals the token's mint forever
	mig.Invoke(t, stackitem.Null{}, "close")
	tok.Invoke(t, stackitem.Make(false), "migrationOpen")
	mig2b := e.NewInvoker(migC.Hash, a2)
	_ = mig2b
}

// Configure binds the published root once; a second call must fault (audit item FTB-02).
func TestConfigureIsWriteOnce(t *testing.T) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)

	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	migC := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_migrator.go", "ftb_migrator.yml")
	e.DeployContract(t, migC, owner.ScriptHash().BytesBE())
	mig := e.NewInvoker(migC.Hash, owner)

	root := make([]byte, 32) // any 32-byte value; content is irrelevant to the write-once check
	mig.Invoke(t, stackitem.Null{}, "configure", tokC.Hash.BytesBE(), root)
	mig.InvokeFail(t, "already configured", "configure", tokC.Hash.BytesBE(), root)
}
