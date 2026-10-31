// Reference neotest suite for FtbMigratorR6 — the v4 claim tree. Mirrors the base
// migrator flow but asserts the v4 ratio: the enlarged entitlement base
// (57,745,832,920,225,836) means the same 100M cap converts at a lower r, so a given
// entitlement mints fewer FTB than under v3/R5 (quote(1000) is 173 here vs 185 under v3/R5).
// A PASS proves R6's leaf format and on-chain Claim agree byte-for-byte with pkg/merkle,
// that Configure is write-once, and that the whole base quotes to EXACTLY the cap.
//
// Run: go test ./contracts/ftb_migrator_r6/
package ftbmigratorr6_test

import (
	"math/big"
	"testing"

	"github.com/freethebird/ftb/pkg/merkle"
	"github.com/nspcc-dev/neo-go/pkg/neotest"
	"github.com/nspcc-dev/neo-go/pkg/neotest/chain"
	"github.com/nspcc-dev/neo-go/pkg/vm/stackitem"
)

func TestClaimFlowR6(t *testing.T) {
	bc, validator := chain.NewSingle(t)
	e := neotest.NewExecutor(t, bc, validator, validator)
	owner := e.NewAccount(t)

	// deploy token and the R5 migrator (owner passed explicitly as deploy data)
	tokC := neotest.CompileFile(t, e.Validator.ScriptHash(), "../ftb_token/ftb_token.go", "../ftb_token/ftb_token.yml")
	e.DeployContract(t, tokC, owner.ScriptHash().BytesBE())
	tok := e.NewInvoker(tokC.Hash, owner)

	migC := neotest.CompileFile(t, e.Validator.ScriptHash(), "ftb_migrator_r6.go", "ftb_migrator_r6.yml")
	e.DeployContract(t, migC, owner.ScriptHash().BytesBE())
	mig := e.NewInvoker(migC.Hash, owner)

	// token trusts the R5 migrator as its only minter
	tok.Invoke(t, stackitem.Null{}, "setMigrator", migC.Hash.BytesBE())

	// snapshot of two holders, built with the SAME pkg/merkle the CLI uses
	a1 := e.NewAccount(t)
	a2 := e.NewAccount(t)
	leaves := [][]byte{
		merkle.Leaf(a1.ScriptHash(), big.NewInt(1000)),
		merkle.Leaf(a2.ScriptHash(), big.NewInt(2500)),
	}
	root, proofs := merkle.Build(leaves)
	proofArg := func(p [][]byte) []any {
		a := make([]any, len(p))
		for i, s := range p {
			a[i] = s
		}
		return a
	}

	mig.Invoke(t, stackitem.Null{}, "configure", tokC.Hash.BytesBE(), root)
	// Configure is write-once (audit item FTB-02): a second call must fault on R5 too.
	mig.InvokeFail(t, "already configured", "configure", tokC.Hash.BytesBE(), root)

	// the v3/R5 ratio is visible on-chain: r = 100M FTB ÷ 540,271,734.82587996 FLM-equiv
	mig.Invoke(t, stackitem.Make(10000000000000000), "ratioNum")
	mig.Invoke(t, stackitem.Make(57745832920225836), "ratioDen")
	// quote at the v4 ratio: floor(1000×r)=173, floor(2500×r)=432 (v3/R5 gave 185 / 462)
	mig.Invoke(t, stackitem.Make(173), "quote", 1000)
	mig.Invoke(t, stackitem.Make(432), "quote", 2500)
	mig.Invoke(t, stackitem.Make(0), "quote", 0)
	// cap boundary: the ENTIRE entitlement base quotes to EXACTLY the 100M cap (10^16), never above.
	mig.Invoke(t, stackitem.Make(10000000000000000), "quote", 57745832920225836)

	// a1 claims its exact entitlement; FTB minted is floor(amount × r), not the face value
	a1Mig := e.NewInvoker(migC.Hash, a1)
	a1Mig.Invoke(t, stackitem.Null{}, "claim", a1.ScriptHash(), 1000, proofArg(proofs[0]))
	tok.Invoke(t, stackitem.Make(173), "balanceOf", a1.ScriptHash().BytesBE())
	mig.Invoke(t, stackitem.Make(true), "claimed", a1.ScriptHash())

	// no double claim (the token's canonical ledger faults the replay)
	a1Mig.InvokeFail(t, "already claimed", "claim", a1.ScriptHash(), 1000, proofArg(proofs[0]))

	// a wrong amount does not verify
	a2Mig := e.NewInvoker(migC.Hash, a2)
	a2Mig.InvokeFail(t, "bad proof", "claim", a2.ScriptHash(), 9999, proofArg(proofs[1]))

	// a2 claims correctly; supply grows by the converted amount only
	a2Mig.Invoke(t, stackitem.Null{}, "claim", a2.ScriptHash(), 2500, proofArg(proofs[1]))
	tok.Invoke(t, stackitem.Make(173+432), "totalSupply")

	// someone not in the snapshot cannot claim
	a3 := e.NewAccount(t)
	e.NewInvoker(migC.Hash, a3).InvokeFail(t, "bad proof", "claim", a3.ScriptHash(), 1, proofArg(proofs[0]))

	// closing seals the token's mint forever
	mig.Invoke(t, stackitem.Null{}, "close")
	tok.Invoke(t, stackitem.Make(false), "migrationOpen")
}
